package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	for _, resource := range []string{"extensions/pissbox-bridge.js", "extensions/lib/agent-messaging.js", "extensions/lib/agent-identity.js", "extensions/agent-names.txt"} {
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
