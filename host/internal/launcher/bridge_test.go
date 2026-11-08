package launcher

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBridgeWatchdogRestoresReturningDaemonWithoutUndoingExplicitStop(t *testing.T) {
	f := newFixture(t)
	t.Setenv("PISSBOX_LAUNCHER_TEST_PROCESS", "bridge-daemon")
	t.Setenv("PISSBOX_BRIDGE_RUNTIME_DIR", f.root)
	t.Setenv("PISSBOX_CONFIG", f.userConfig)
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	cli := func(op string) []byte {
		t.Helper()
		out, err := exec.Command(f.exe, op).CombinedOutput()
		if err != nil {
			t.Fatalf("bridge %s: %v: %s", op, err, out)
		}
		return out
	}
	wait := func(check func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if check() {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("bridge recovery timed out")
	}
	cli("ensure")
	t.Cleanup(func() {
		cli("stop")
		wait(func() bool { _, err := os.Stat(filepath.Join(f.root, "control.sock")); return os.IsNotExist(err) })
	})
	ctx, cancel := context.WithCancel(context.Background())
	r := registration{binary: f.exe, processes: &processes{}, cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(r.done)
		r.maintain(ctx, []string{"reconnect", "--mailbox", f.state, "--owner-pid", fmt.Sprint(os.Getpid())}, 20*time.Millisecond)
	}()
	defer r.stop()
	if err := os.MkdirAll(f.state, 0700); err != nil {
		t.Fatal(err)
	}
	registered := func() bool {
		var status struct {
			Registrations int `json:"registrations"`
		}
		if err := json.Unmarshal(cli("status"), &status); err != nil {
			t.Fatal(err)
		}
		return status.Registrations == 1
	}
	wait(registered)
	first, err := os.ReadFile(filepath.Join(f.state, "generation"))
	if err != nil {
		t.Fatal(err)
	}
	cli("stop")
	wait(func() bool { _, err := os.Stat(filepath.Join(f.root, "control.sock")); return os.IsNotExist(err) })
	time.Sleep(100 * time.Millisecond)
	if out, err := exec.Command(f.exe, "status").CombinedOutput(); err == nil {
		t.Fatalf("watchdog restarted an explicitly stopped daemon: %s", out)
	}
	cli("ensure")
	wait(registered)
	second, err := os.ReadFile(filepath.Join(f.state, "generation"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) == string(second) {
		t.Fatal("restart reused a generation")
	}
	r.stop()
	if r.id != "" {
		t.Fatal("stop retained the recovered registration")
	}
	var status struct {
		Registrations int `json:"registrations"`
	}
	json.Unmarshal(cli("status"), &status)
	if status.Registrations != 0 {
		t.Fatal("stop did not unregister the recovered ID")
	}
}

func TestBridgeHerdBindingAndResourceRefusal(t *testing.T) {
	for _, tc := range []struct {
		env     map[string]string
		binding bool
		warning string
	}{
		{map[string]string{"HERDR_ENV": "1", "HERDR_SOCKET_PATH": "host-only.sock", "HERDR_PANE_ID": "w1:p7"}, true, ""},
		{map[string]string{"HERDR_ENV": "1", "HERDR_SOCKET_PATH": "host-only.sock"}, false, "Herd reporting disabled"},
		{map[string]string{"HERDR_ENV": "1", "HERDR_PANE_ID": "w1:p7"}, false, "Herd reporting disabled"},
		{map[string]string{"HERDR_SOCKET_PATH": "host-only.sock", "HERDR_PANE_ID": "w1:p7"}, false, ""},
	} {
		t.Run(fmt.Sprint(tc.env), func(t *testing.T) {
			f := newFixture(t)
			f.bridge()
			out, status, calls := f.launch("", tc.env)
			if status != 0 || kinds(calls) != "bridge-register,run,bridge-unregister" {
				t.Fatalf("%d %v %s", status, calls, out)
			}
			binding := strings.Contains(strings.Join(calls[0], " "), "--herdr-socket host-only.sock --herdr-pane w1:p7")
			if binding != tc.binding {
				t.Fatal(calls[0])
			}
			if strings.Contains(strings.Join(calls[1], " "), "host-only.sock") || strings.Contains(strings.Join(calls[1], " "), "w1:p7") {
				t.Fatal("host binding exposed to sandbox", calls[1])
			}
			if tc.warning != "" && !strings.Contains(out, tc.warning) {
				t.Fatal(out)
			}
		})
	}
	for _, resource := range []string{"extensions/pissbox-bridge.js", "extensions/lib/agent-messaging.js", "extensions/lib/agent-lifetime.js", "extensions/lib/vault-tools.js", "extensions/lib/agent-identity.js", "extensions/agent-names.txt"} {
		t.Run(resource, func(t *testing.T) {
			f := newFixture(t)
			f.bridge()
			os.Remove(filepath.Join(f.pi, resource))
			out, status, calls := f.launch("", nil)
			if status != 0 || kinds(calls) != "run" || !strings.Contains(out, "extension missing") {
				t.Fatalf("%d %v %s", status, calls, out)
			}
		})
	}
}
func TestBridgeStopBypassesConfigSettingsAndSandbox(t *testing.T) {
	f := newFixture(t)
	f.bridge()
	f.config("invalid toml")
	out, status, calls := f.launch("", nil, "--bridge-stop")
	if status != 0 || kinds(calls) != "bridge-stop" {
		t.Fatalf("%d %v %s", status, calls, out)
	}
	if _, err := os.Stat(f.state); !os.IsNotExist(err) {
		t.Fatal("bridge stop prepared state")
	}
	os.Remove(bridgeBinary(f.root))
	out, status, calls = f.launch("", nil, "--bridge-stop")
	if status == 0 || len(calls) != 0 || !strings.Contains(out, "binary is missing") {
		t.Fatalf("%d %v %s", status, calls, out)
	}
}
func TestBridgeOnlyStartsAfterIsolationFinalization(t *testing.T) {
	for _, finalize := range []string{"0", "23"} {
		t.Run(finalize, func(t *testing.T) {
			f := newFixture(t)
			f.bridge()
			f.config("isolated_directories = [\"deps\"]")
			out, status, calls := f.launch("", map[string]string{"FINALIZE_STATUS": finalize})
			want := "create,mount,exec,exec,bridge-register,run,bridge-unregister"
			if finalize == "23" {
				want = "create,mount,exec,exec"
			}
			if kinds(calls) != want || (finalize == "23" && status != 23) || (finalize == "0" && status != 0) {
				t.Fatalf("%d %v %s", status, calls, out)
			}
		})
	}
}

func TestBridgeReloadBypassesConfigSettingsAndSandbox(t *testing.T) {
	f := newFixture(t)
	f.bridge()
	f.config("invalid toml")
	out, status, calls := f.launch("", nil, "--bridge-reload")
	if status != 0 || kinds(calls) != "bridge-reload" {
		t.Fatalf("%d %v %s", status, calls, out)
	}
	if _, err := os.Stat(f.state); !os.IsNotExist(err) {
		t.Fatal("bridge reload prepared state")
	}
	os.Remove(bridgeBinary(f.root))
	out, status, calls = f.launch("", nil, "--bridge-reload")
	if status == 0 || len(calls) != 0 || !strings.Contains(out, "binary is missing") {
		t.Fatalf("%d %v %s", status, calls, out)
	}
}

func TestBridgeUnregisterWritesLauncherRetirementMarker(t *testing.T) {
	f := newFixture(t)
	f.bridge()
	out, status, calls := f.launch("", nil)
	if status != 0 || kinds(calls) != "bridge-register,run,bridge-unregister" {
		t.Fatalf("%d %v %s", status, calls, out)
	}
	mailbox := flagValue(calls[0], "--mailbox")
	id := flagValue(calls[2], "--id")
	if mailbox == "" || id == "" {
		t.Fatalf("missing mailbox or registration ID: %v", calls)
	}
	data, err := os.ReadFile(filepath.Join(mailbox, "retired"))
	if err != nil {
		t.Fatal(err)
	}
	var marker struct {
		Version int    `json:"version"`
		ID      string `json:"id"`
	}
	if err := json.Unmarshal(data, &marker); err != nil || marker.Version != 1 || marker.ID != id {
		t.Fatalf("invalid launcher retirement marker: %+v, %v", marker, err)
	}
}
