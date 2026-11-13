package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"pissbox/host/config"
)

func TestGlobalSecretsRegisterOnCreationAndNotOnAttach(t *testing.T) {
	f := newFixture(t)
	global := `[secrets.ZED]
host = "z.example.com"
command = "printf zed"
[secrets.ALPHA]
host = "*.example.com"
command = "test -n \"$HOME\""
`
	f.write(f.userConfig, global)
	out, status, calls := f.launch("", nil, "-c")
	if status != 0 || kinds(calls) != "secret,create,secret,secret,exec,run" {
		t.Fatalf("%d %v: %s", status, calls, out)
	}
	want := [][]string{
		{"secret", "set-custom", "--sandbox", "reported", "--env", "ALPHA", "--host", "*.example.com", "--placeholder", argValue(calls[2], "--placeholder"), "--command", "test -n \"$HOME\""},
		{"secret", "set-custom", "--sandbox", "reported", "--env", "ZED", "--host", "z.example.com", "--placeholder", argValue(calls[3], "--placeholder"), "--command", "printf zed"},
	}
	if !reflect.DeepEqual(calls[2:4], want) {
		t.Fatal(calls)
	}
	if !contains(calls[1], "ALPHA="+argValue(calls[2], "--placeholder")) || !contains(calls[1], "ZED="+argValue(calls[3], "--placeholder")) {
		t.Fatal("creation did not receive global secret placeholders", calls[1])
	}
	if calls[4][0] != "exec" || !strings.Contains(calls[4][len(calls[4])-1], "--finalize-isolation") {
		t.Fatal(calls)
	}

	out, status, calls = f.launch("", map[string]string{"CONFLICT": "1"}, "-y")
	if status != 0 || kinds(calls) != "secret,create,exec,run" {
		t.Fatalf("%d %v: %s", status, calls, out)
	}
	for _, call := range calls {
		if call[0] == "secret" && len(call) > 1 && call[1] == "set-custom" && contains(call, "--sandbox") {
			t.Fatal("registered global secret on attach", call)
		}
	}
}

func TestCLIOverridesGlobalAndLeavesConfigReadOnly(t *testing.T) {
	f := newFixture(t)
	marker := filepath.Join(f.project, "overridden-command-ran")
	global := fmt.Sprintf(`[secrets.API]
host = "global.example.com"
command = "touch '%s'"
[secrets.OTHER]
host = "other.example.com"
command = "true"
`, marker)
	f.write(f.userConfig, global)
	out, status, calls := f.launch("", nil, "--secret", "API", "cli.example.com", "cli_command")
	if status != 0 || kinds(calls) != "secret,create,secret,secret,exec,run" {
		t.Fatalf("%d %v: %s", status, calls, out)
	}
	if !reflect.DeepEqual(calls[2], []string{"secret", "set-custom", "--sandbox", "reported", "--env", "API", "--host", "cli.example.com", "--placeholder", argValue(calls[2], "--placeholder"), "--command", "cli_command"}) {
		t.Fatal(calls[2])
	}
	if !reflect.DeepEqual(calls[3], []string{"secret", "set-custom", "--sandbox", "reported", "--env", "OTHER", "--host", "other.example.com", "--placeholder", argValue(calls[3], "--placeholder"), "--command", "true"}) {
		t.Fatal(calls[3])
	}
	if !contains(calls[1], "API="+argValue(calls[2], "--placeholder")) || !contains(calls[1], "OTHER="+argValue(calls[3], "--placeholder")) {
		t.Fatal("creation did not receive merged placeholders", calls[1])
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("launcher probed a CLI-overridden global command")
	}
	if data, err := os.ReadFile(f.userConfig); err != nil || string(data) != global {
		t.Fatal("CLI changed global config", string(data), err)
	}

	out, status, calls = f.launch("", nil, "--secret", "API", "one.example.com", "one", "--secret", "API", "two.example.com", "two")
	if status == 0 || len(calls) != 0 {
		t.Fatalf("%d %v: %s", status, calls, out)
	}
	if data, _ := os.ReadFile(f.userConfig); string(data) != global {
		t.Fatal("duplicate CLI entries changed global config")
	}
}

func TestInaccessibleGlobalSecretsAreSkipped(t *testing.T) {
	f := newFixture(t)
	global := `[secrets.GOOD]
host = "good.example.com"
command = "true"
[secrets.BAD]
host = "bad.example.com"
command = "exit 3"
[secrets.MISSING]
host = "missing.example.com"
command = "/definitely/not/a/real/command"
`
	f.write(f.userConfig, global)
	out, status, calls := f.launch("", nil)
	if status != 0 || kinds(calls) != "secret,create,secret,exec,run" {
		t.Fatalf("%d %v: %s", status, calls, out)
	}
	for _, name := range []string{"BAD", "MISSING"} {
		if strings.Count(out, "Global secret "+name+" was not added") != 1 {
			t.Fatalf("missing warning for %s: %s", name, out)
		}
	}
	if strings.Contains(out, "Global secret GOOD") || !strings.Contains(out, "Global secret BAD was not added") || !strings.Contains(out, "Global secret MISSING was not added") {
		t.Fatal(out)
	}
	if !reflect.DeepEqual(calls[2][4:6], []string{"--env", "GOOD"}) {
		t.Fatal(calls[2])
	}
	created := strings.Join(calls[1], " ")
	if strings.Contains(created, "BAD=") || strings.Contains(created, "MISSING=") {
		t.Fatal("skipped secret passed to creation", calls[1])
	}
}

func TestDefaultGlobalStarterCreationAndCreationFailureWarning(t *testing.T) {
	f := newFixture(t)
	xdg := filepath.Join(f.configDir, "xdg")
	// The fixture lives under the checkout, which may itself be under HOME.
	// A separate HOME keeps the expected schema path independent of that.
	home := t.TempDir()
	out, status, calls := f.launch("", map[string]string{"PISSBOX_CONFIG": "", "XDG_CONFIG_HOME": xdg, "HOME": home})
	if status != 0 || kinds(calls) != "run" {
		t.Fatalf("%d %v: %s", status, calls, out)
	}
	path := filepath.Join(xdg, "pissbox", "config.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := config.Starter(f.root)
	if string(data) != want {
		t.Fatalf("got %q want %q", data, want)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal(info, err)
	}

	notDirectory := filepath.Join(f.configDir, "not-a-directory")
	f.write(notDirectory, "x")
	out, status, calls = f.launch("", map[string]string{"PISSBOX_CONFIG": "", "XDG_CONFIG_HOME": notDirectory, "HOME": home})
	if status != 0 || kinds(calls) != "run" {
		t.Fatalf("%d %v: %s", status, calls, out)
	}
	if !strings.Contains(out, "Warning: Could not create global configuration") {
		t.Fatal(out)
	}
}

func TestInvalidGlobalConfigPrecedesSideEffects(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(fixture) map[string]string
	}{
		{"malformed", func(f fixture) map[string]string {
			f.write(f.userConfig, "[secrets.API\nhost = \"example.com\"\n")
			return nil
		}},
		{"unknown key", func(f fixture) map[string]string {
			f.write(f.userConfig, "[secrets.API]\nhost = \"example.com\"\ncommand = \"get\"\nvalue = \"literal\"\n")
			return nil
		}},
		{"invalid network", func(f fixture) map[string]string {
			f.write(f.userConfig, "[network]\nblock = ['']\n")
			return nil
		}},
		{"missing override", func(f fixture) map[string]string {
			return map[string]string{"PISSBOX_CONFIG": filepath.Join(f.configDir, "missing.toml")}
		}},
		{"inside workspace", func(f fixture) map[string]string {
			path := filepath.Join(f.project, "config.toml")
			f.write(path, "[secrets.API]\nhost = \"example.com\"\ncommand = \"get\"\n")
			return map[string]string{"PISSBOX_CONFIG": path}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			env := tc.setup(f)
			out, status, calls := f.launch("", env)
			if status == 0 || len(calls) != 0 {
				t.Fatalf("%d %v: %s", status, calls, out)
			}
			if tc.name == "inside workspace" && !strings.Contains(out, "must not be inside") {
				t.Fatal(out)
			}
			if tc.name == "missing override" && !strings.Contains(out, "does not exist") {
				t.Fatal(out)
			}
			for _, path := range []string{f.state, filepath.Join(f.pi, "settings.json")} {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatalf("unexpected effect: %s", path)
				}
			}
		})
	}
}

func TestManagementCommandsBypassGlobalConfig(t *testing.T) {
	f := newFixture(t)
	f.bridge()
	missing := filepath.Join(f.configDir, "missing.toml")
	for _, args := range [][]string{{"--help"}, {"--ls"}, {"--delete", "."}, {"--bridge-stop"}, {"--bridge-reload"}} {
		out, status, _ := f.launch("", map[string]string{"PISSBOX_CONFIG": missing}, args...)
		if strings.Contains(out, "Global configuration") {
			t.Fatalf("%v read global config: %d %s", args, status, out)
		}
		if _, err := os.Stat(missing); !os.IsNotExist(err) {
			t.Fatalf("%v created global config", args)
		}
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
