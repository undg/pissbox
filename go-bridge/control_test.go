package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

// Exercise the actual CLI and detached startup path, not an in-process daemon.
func TestMain(m *testing.M) {
	if os.Getenv("PISSBOX_BRIDGE_TEST_PROCESS") == "1" {
		main()
		return
	}
	os.Exit(m.Run())
}

type processBridge struct {
	t   *testing.T
	dir string
	env []string
}

func processFixture(t *testing.T) processBridge {
	t.Helper()
	dir, _ := fixture(t)
	dir, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	p := processBridge{t: t, dir: dir, env: append(os.Environ(), "PISSBOX_BRIDGE_TEST_PROCESS=1", "GORACE=atexit_sleep_ms=0", "PISSBOX_BRIDGE_RUNTIME_DIR="+dir)}
	t.Cleanup(func() {
		// Always target this isolated directory, never the user's real daemon.
		controlCall(p.dir, controlRequest{Op: "stop"})
		waitFor(t, func() bool {
			_, err := os.Stat(filepath.Join(p.dir, "control.sock"))
			return os.IsNotExist(err)
		})
	})
	return p
}

func (p processBridge) command(args ...string) *exec.Cmd {
	exe, err := os.Executable()
	if err != nil {
		p.t.Fatal(err)
	}
	cmd := exec.Command(exe, args...)
	cmd.Env = append([]string(nil), p.env...)
	return cmd
}

func (p processBridge) cli(args ...string) []byte {
	p.t.Helper()
	out, err := p.command(args...).CombinedOutput()
	if err != nil {
		p.t.Fatalf("%v: %v: %s", args, err, out)
	}
	return out
}

func (p processBridge) status() controlResponse {
	p.t.Helper()
	var status controlResponse
	if err := json.Unmarshal(p.cli("status"), &status); err != nil {
		p.t.Fatal(err)
	}
	return status
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("condition did not become true within five seconds")
}

func TestConcurrentStartupSharesDaemonAcrossStateRootsAndSurvivesStarter(t *testing.T) {
	p := processFixture(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cmd := p.command("ensure")
			cmd.Env = append(cmd.Env, fmt.Sprintf("PISSBOX_STATE_DIR=unused-state-%d", i))
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("concurrent ensure: %v: %s", err, out)
			}
		}(i)
	}
	wg.Wait()
	first := p.status()
	if first.PID <= 1 || first.Registrations != 0 {
		t.Fatalf("bad daemon status: %+v", first)
	}
	p.cli("ensure")
	if p.status().PID != first.PID {
		t.Fatal("ensure replaced the daemon after its starter exited")
	}
	p.cli("stop")
	waitFor(t, func() bool {
		_, err := controlCall(p.dir, controlRequest{Op: "status"})
		return err == errUnavailable
	})
}

func TestStaleSocketAndLockRecoverAfterDaemonCrash(t *testing.T) {
	p := processFixture(t)
	p.cli("ensure")
	pid := p.status().PID
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return staleLock(p.dir) })
	if _, err := os.Stat(filepath.Join(p.dir, "control.sock")); err != nil {
		t.Fatal("crash did not leave the socket fixture behind")
	}
	p.cli("ensure")
	if p.status().PID == pid {
		t.Fatal("crashed daemon was not replaced")
	}
}

func TestIncompatibleLiveDaemonIsNotReplacedOrStopped(t *testing.T) {
	p := processFixture(t)
	listener, err := net.Listen("unix", filepath.Join(p.dir, "control.sock"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			var req controlRequest
			json.NewDecoder(conn).Decode(&req)
			json.NewEncoder(conn).Encode(controlResponse{Version: 99, PID: os.Getpid()})
			conn.Close()
		}
	}()
	for _, op := range []string{"ensure", "stop", "status"} {
		out, err := p.command(op).CombinedOutput()
		if err == nil {
			t.Fatalf("%s accepted incompatible daemon: %s", op, out)
		}
	}
	if response, _ := controlCall(p.dir, controlRequest{Op: "status"}); response.Version != 99 {
		t.Fatal("live incompatible socket was replaced")
	}
}

func herdFixture(t *testing.T, socket string) <-chan request {
	t.Helper()
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	requests := make(chan request, 100)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			var r request
			if json.NewDecoder(conn).Decode(&r) == nil {
				requests <- r
				json.NewEncoder(conn).Encode(map[string]any{"id": r.ID, "result": map[string]string{"type": "ok"}})
			}
			conn.Close()
		}
	}()
	return requests
}

func registerProcess(t *testing.T, p processBridge, mailbox, socket, pane string, owner int) string {
	t.Helper()
	response, err := controlCall(p.dir, controlRequest{Op: "register", Mailbox: mailbox, Socket: socket, Pane: pane, Owner: owner})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.ID) != 32 {
		t.Fatalf("missing opaque registration ID: %+v", response)
	}
	return response.ID
}

func TestIndependentMailboxesDrainReleaseAndDetectOwnerCrash(t *testing.T) {
	p := processFixture(t)
	p.cli("ensure")
	socket := filepath.Join(p.dir, "herd.sock")
	requests := herdFixture(t, socket)
	a, _ := fixture(t)
	b, _ := fixture(t)
	a, _ = filepath.Abs(a)
	b, _ = filepath.Abs(b)
	owner := exec.Command("sleep", "60")
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { owner.Process.Kill(); owner.Wait() })
	id := registerProcess(t, p, a, socket, "pane-a", os.Getpid())
	registerProcess(t, p, b, socket, "pane-b", owner.Process.Pid)
	// A second registration cannot take ownership of, or later release, pane A.
	if _, err := controlCall(p.dir, controlRequest{Op: "register", Mailbox: b, Socket: socket, Pane: "pane-a", Owner: os.Getpid()}); err == nil {
		t.Fatal("accepted duplicate mailbox/pane")
	}
	queue(t, a, 1, `{"version":1,"kind":"state","state":"working"}`)
	queue(t, b, 1, `{"version":1,"kind":"state","state":"blocked"}`)
	seen := map[string]any{}
	for i := 0; i < 2; i++ {
		r := receive(t, requests)
		seen[r.Params["pane_id"].(string)] = r.Params["state"]
	}
	if seen["pane-a"] != "working" || seen["pane-b"] != "blocked" {
		t.Fatalf("wrong routing: %v", seen)
	}
	// Queue immediately before unregister: the final event must be drained.
	queue(t, a, 2, `{"version":1,"kind":"state","state":"idle"}`)
	p.cli("unregister", "--id", id)
	for _, method := range []string{"pane.report_agent", "pane.release_agent"} {
		r := receive(t, requests)
		if r.Method != method || r.Params["pane_id"] != "pane-a" {
			t.Fatalf("wrong final drain/release: %+v", r)
		}
	}
	if p.status().Registrations != 1 {
		t.Fatal("unregister removed another registration")
	}
	queue(t, b, 2, `{"version":1,"kind":"state","state":"working"}`)
	if r := receive(t, requests); r.Params["pane_id"] != "pane-b" || r.Params["state"] != "working" {
		t.Fatalf("surviving pane stopped reporting: %+v", r)
	}
	owner.Process.Kill()
	owner.Wait()
	if r := receive(t, requests); r.Method != "pane.release_agent" || r.Params["pane_id"] != "pane-b" {
		t.Fatalf("owner crash did not release its own pane: %+v", r)
	}
	waitFor(t, func() bool { return p.status().Registrations == 0 })
	for _, dir := range []string{a, b} {
		if _, err := os.Stat(filepath.Join(dir, "ready")); !os.IsNotExist(err) {
			t.Fatal("readiness survived unregistration")
		}
	}
}

func TestFailedHerdConnectionDoesNotBlockOtherMailboxes(t *testing.T) {
	p := processFixture(t)
	p.cli("ensure")
	socket := filepath.Join(p.dir, "herd.sock")
	requests := herdFixture(t, socket)
	// Accept connections but never reply, exercising the socket deadline.
	stuck, err := net.Listen("unix", filepath.Join(p.dir, "stuck.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer stuck.Close()
	go func() {
		for {
			conn, err := stuck.Accept()
			if err != nil {
				return
			}
			go func() { defer conn.Close(); time.Sleep(time.Second) }()
		}
	}()
	bad, _ := fixture(t)
	good, _ := fixture(t)
	bad, _ = filepath.Abs(bad)
	good, _ = filepath.Abs(good)
	registerProcess(t, p, bad, filepath.Join(p.dir, "stuck.sock"), "bad", os.Getpid())
	registerProcess(t, p, good, socket, "good", os.Getpid())
	queue(t, bad, 1, `{"version":1,"kind":"state","state":"working"}`)
	queue(t, good, 1, `{"version":1,"kind":"state","state":"working"}`)
	started := time.Now()
	if r := receive(t, requests); r.Params["pane_id"] != "good" || time.Since(started) > time.Second {
		t.Fatalf("failed connection stalled healthy mailbox: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(bad, "00000000000000000001-0000000000000000.json")); err != nil {
		t.Fatal("failed connection lost queued event")
	}
	// A broken mailbox also expires independently, leaving the daemon alive.
	os.Remove(filepath.Join(good, "ready"))
	os.Mkdir(filepath.Join(good, "ready"), 0700)
	waitFor(t, func() bool { return p.status().Registrations == 1 })
	if _, err := controlCall(p.dir, controlRequest{Op: "status"}); err != nil {
		t.Fatal(err)
	}
}

func TestStopRejectsWrongVersionAndReleasesAllRegistrations(t *testing.T) {
	p := processFixture(t)
	p.cli("ensure")
	pid := p.status().PID
	socket := filepath.Join(p.dir, "herd.sock")
	requests := herdFixture(t, socket)
	var mailboxes []string
	for _, pane := range []string{"a", "b"} {
		mailbox, _ := fixture(t)
		mailbox, _ = filepath.Abs(mailbox)
		mailboxes = append(mailboxes, mailbox)
		registerProcess(t, p, mailbox, socket, pane, os.Getpid())
		queue(t, mailbox, 1, `{"version":1,"kind":"state","state":"working"}`)
		receive(t, requests)
	}
	conn, err := net.Dial("unix", filepath.Join(p.dir, "control.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(controlTimeout))
	json.NewEncoder(conn).Encode(controlRequest{Version: 99, Op: "stop"})
	var response controlResponse
	if err := json.NewDecoder(conn).Decode(&response); err != nil || response.Error == "" {
		t.Fatalf("incompatible stop was not rejected: %+v, %v", response, err)
	}
	if status := p.status(); status.PID != pid || status.Registrations != 2 {
		t.Fatalf("incompatible stop changed live daemon: %+v", status)
	}
	p.cli("stop")
	panes := map[string]bool{}
	for i := 0; i < 2; i++ {
		r := receive(t, requests)
		if r.Method != "pane.release_agent" {
			t.Fatalf("stop did not release: %+v", r)
		}
		panes[r.Params["pane_id"].(string)] = true
	}
	if !panes["a"] || !panes["b"] {
		t.Fatalf("stop released wrong panes: %v", panes)
	}
	waitFor(t, func() bool {
		for _, mailbox := range mailboxes {
			if _, err := os.Stat(filepath.Join(mailbox, "ready")); !os.IsNotExist(err) {
				return false
			}
		}
		return true
	})
}

func TestIdleControlClientExpiresWithoutStallingStatus(t *testing.T) {
	p := processFixture(t)
	p.cli("ensure")
	conn, err := net.Dial("unix", filepath.Join(p.dir, "control.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	p.status()
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Fatal("idle control client was not disconnected")
	} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("server did not enforce its control IO deadline")
	}
}

func TestControlRejectsUnboundedPathsAndUnsafeRuntime(t *testing.T) {
	p := processFixture(t)
	cmd := p.command("ensure")
	cmd.Env = append(cmd.Env, "PISSBOX_BRIDGE_RUNTIME_DIR="+p.dir+"/"+fmt.Sprintf("%0104d", 1))
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("accepted oversized socket path: %s", out)
	}
	if err := os.Chmod(p.dir, 0755); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(p.dir, 0700)
	if out, err := p.command("ensure").CombinedOutput(); err == nil {
		t.Fatalf("daemon unexpectedly started in an unsafe directory: %s", out)
	}
}
