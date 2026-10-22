package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func fixture(t *testing.T) (string, *os.Root) {
	t.Helper()
	base := os.Getenv("PISSBOX_BRIDGE_TEST_FIXTURE_DIR")
	if base == "" {
		base = filepath.Join("..", ".cache", "test-fixtures")
	}
	if err := os.MkdirAll(base, 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(base, "bridge-")
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	return dir, root
}

func TestFixtureRootOverride(t *testing.T) {
	base, _ := fixture(t)
	t.Setenv("PISSBOX_BRIDGE_TEST_FIXTURE_DIR", base)
	dir, _ := fixture(t)
	if filepath.Dir(dir) != base {
		t.Fatalf("fixture root override ignored: %s is not under %s", dir, base)
	}
}

func queue(t *testing.T, dir string, seq int, body string) string {
	t.Helper()
	name := fmt.Sprintf("%020d-0000000000000000.json", seq)
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return name
}

type request struct {
	ID     string         `json:"id"`
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

func receive(t *testing.T, requests <-chan request) request {
	t.Helper()
	select {
	case r := <-requests:
		return r
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Herd request")
		return request{}
	}
}

func TestMailboxForwardsOrderedTransitionsAndReleasesOnDisconnect(t *testing.T) {
	dir, root := fixture(t)
	socket := filepath.Join(dir, "herdr.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	requests := make(chan request, 10)
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
	// Deliberately create files out of order. All transitions happen before one poll.
	queue(t, dir, 3, `{"version":1,"kind":"state","state":"idle"}`)
	queue(t, dir, 1, `{"version":1,"kind":"state","state":"working"}`)
	queue(t, dir, 2, `{"version":1,"kind":"state","state":"blocked"}`)
	queue(t, dir, 4, `{"version":1,"kind":"state","state":"working","pane_id":"other","method":"pane.close"}`)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := bridge{root: root, target: &herdr{socket: socket, pane: "w1:p7"}, warn: func(string) {}}
	done := make(chan error, 1)
	go func() { done <- b.run(ctx, 0) }()
	previousSeq := float64(0)
	for _, state := range []string{"working", "blocked", "idle"} {
		r := receive(t, requests)
		if r.Method != "pane.report_agent" || r.Params["state"] != state ||
			r.Params["pane_id"] != "w1:p7" || r.Params["agent"] != "pissbox-pi" || r.Params["source"] != "pissbox:pi" {
			t.Fatalf("unexpected report: %+v", r)
		}
		seq := r.Params["seq"].(float64)
		if seq <= previousSeq {
			t.Fatal("report sequences did not increase")
		}
		previousSeq = seq
	}
	cancel()
	r := receive(t, requests)
	if r.Method != "pane.release_agent" || r.Params["pane_id"] != "w1:p7" {
		t.Fatalf("missing release: %+v", r)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("bridge did not stop")
	}
	if _, err := root.Stat("ready"); !os.IsNotExist(err) {
		t.Fatal("ready marker survived shutdown")
	}
	names, err := eventNames(root)
	if err != nil || len(names) != 0 {
		t.Fatalf("events not consumed: %v, %v", names, err)
	}
}

func TestMailboxRejectsUnsafeEvents(t *testing.T) {
	dir, root := fixture(t)
	for _, body := range []string{
		`{"version":2,"kind":"state","state":"idle"}`,
		`{"version":1,"kind":"command","state":"idle"}`,
		`{"version":1,"kind":"release","state":"idle"}`,
		`{"version":1,"kind":"state","state":"done"}`,
		`{"version":1,"kind":"state","state":"idle"} {}`,
		`{"version":1,"kind":"state","state":"idle","resume_argv":["sh"]}`,
		`{"version":1,"kind":"state","state":"blocked","message":"\u001b[2J"}`,
		strings.Repeat("x", maxEventBytes+1),
	} {
		name := queue(t, dir, 1, body)
		if _, err := readEvent(root, name); err == nil {
			t.Fatalf("accepted invalid event: %.80s", body)
		}
	}
	outside := dir + ".outside.json"
	if err := os.WriteFile(outside, []byte(`{"version":1,"kind":"state","state":"idle"}`), 0600); err != nil {
		t.Fatal(err)
	}
	link := "00000000000000000002-0000000000000000.json"
	if err := os.Symlink("../"+filepath.Base(outside), filepath.Join(dir, link)); err != nil {
		t.Fatal(err)
	}
	if _, err := readEvent(root, link); err == nil {
		t.Fatal("followed a sandbox-supplied symlink")
	}
	fifo := "00000000000000000003-0000000000000000.json"
	if err := syscall.Mkfifo(filepath.Join(dir, fifo), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readEvent(root, fifo); err == nil {
		t.Fatal("accepted a FIFO as an event")
	}
}

func TestHerdRejectionKeepsQueuedEventForRetry(t *testing.T) {
	dir, root := fixture(t)
	socket := filepath.Join(dir, "herdr.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for i := 0; i < 2; i++ {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			var r request
			json.NewDecoder(bufio.NewReader(conn)).Decode(&r)
			if i == 0 {
				json.NewEncoder(conn).Encode(map[string]any{"id": r.ID, "error": map[string]string{"code": "pane_not_found"}})
			} else {
				json.NewEncoder(conn).Encode(map[string]any{"id": r.ID, "result": map[string]string{"type": "ok"}})
			}
			conn.Close()
		}
	}()
	name := queue(t, dir, 1, `{"version":1,"kind":"state","state":"working"}`)
	warnings := 0
	b := bridge{root: root, target: &herdr{socket: socket, pane: "w1:p1"}, warn: func(string) { warnings++ }}
	if err := b.poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := root.Stat(name); err != nil || warnings != 1 {
		t.Fatal("failed report was not retained with a warning")
	}
	if err := b.poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := root.Stat(name); !os.IsNotExist(err) {
		t.Fatal("successful retry was not consumed")
	}
}
