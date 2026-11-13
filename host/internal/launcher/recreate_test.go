package launcher

import (
	"fmt"
	"strings"
	"testing"
)

func TestRecreateConfirmationAndFailures(t *testing.T) {
	for _, tc := range []struct {
		name, input, removeStatus string
		args                      []string
		status                    int
		want                      string
	}{
		{"confirmed", "yes\n", "", nil, 0, "run,rm,run"},
		{"declined", "no\n", "", nil, 0, "run"},
		{"default no", "\n", "", nil, 0, "run"},
		{"eof", "", "", nil, 1, "run"},
		{"continue still confirms", "no\n", "", []string{"-c"}, 0, "run"},
		{"skip confirmation", "", "", []string{"-y", "-c"}, 0, "run,rm,run"},
		{"remove failure", "yes\n", "7", nil, 7, "run,rm"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			args := append([]string{"--recreate"}, tc.args...)
			out, status, calls := f.launch(tc.input, map[string]string{"CONFLICT_UNTIL_REMOVED": "1", "REMOVE_STATUS": tc.removeStatus}, args...)
			if status != tc.status || kinds(calls) != tc.want {
				t.Fatalf("status=%d calls=%v: %s", status, calls, out)
			}
			if !strings.Contains(out, "loses sandbox-local data") {
				t.Fatal("missing data-loss warning:", out)
			}
			if len(calls) > 1 && fmt.Sprint(calls[1]) != "[rm --force reported]" {
				t.Fatal("removed wrong sandbox:", calls)
			}
			if len(calls) == 3 && fmt.Sprint(calls[0]) != fmt.Sprint(calls[2]) {
				t.Fatal("creation arguments changed:", calls)
			}
			if tc.name == "skip confirmation" && (!contains(calls[2], "--continue") || strings.Contains(out, "[y/N]")) {
				t.Fatal("continue or yes flag ignored:", calls, out)
			}
		})
	}
}

func TestRecreateCreatesMissingSandbox(t *testing.T) {
	f := newFixture(t)
	out, status, calls := f.launch("", nil, "--recreate")
	if status != 0 || kinds(calls) != "run" || strings.Contains(out, "[y/N]") {
		t.Fatalf("status=%d calls=%v: %s", status, calls, out)
	}
}

func TestRecreateDoesNotRetryFailuresOrAttach(t *testing.T) {
	for _, env := range []map[string]string{
		{"RUN_STATUS": "9"},
		{"CONFLICT": "1"},
	} {
		f := newFixture(t)
		out, status, calls := f.launch("", env, "--recreate", "-y", "-c")
		want := "run"
		if env["CONFLICT"] != "" {
			want = "run,rm,run"
		}
		if status == 0 || kinds(calls) != want {
			t.Fatalf("status=%d calls=%v: %s", status, calls, out)
		}
	}
}

func TestRecreateValidatesConfigBeforeDeletion(t *testing.T) {
	f := newFixture(t)
	f.config("setup = 5")
	out, status, calls := f.launch("", nil, "--recreate", "-y")
	if status == 0 || len(calls) != 0 {
		t.Fatalf("status=%d calls=%v: %s", status, calls, out)
	}
}

func TestRecreateStagedSandbox(t *testing.T) {
	for _, named := range []bool{false, true} {
		t.Run(fmt.Sprint(named), func(t *testing.T) {
			f := newFixture(t)
			f.config("isolated_directories = [\"deps\"]")
			args := []string{"--recreate", "-y", "-c"}
			env := map[string]string{"CONFLICT_UNTIL_REMOVED": "1"}
			name := "reported"
			if named {
				name = "chosen"
				args = append(args, "--name", name)
				env["CREATED_NAME"] = name
			}
			out, status, calls := f.launch("", env, args...)
			if status != 0 || kinds(calls) != "create,rm,create,mount,exec,exec,run" {
				t.Fatalf("status=%d calls=%v: %s", status, calls, out)
			}
			if fmt.Sprint(calls[1]) != fmt.Sprint([]string{"rm", "--force", name}) || flagValue(calls[6], "--name") != name || !contains(calls[6], "--continue") {
				t.Fatal(calls)
			}
		})
	}
}

func TestRecreateRegistersCurrentGlobalSecrets(t *testing.T) {
	f := newFixture(t)
	f.write(f.userConfig, "[secrets.API]\nhost = \"api.example.com\"\ncommand = \"true\"\n")
	out, status, calls := f.launch("", map[string]string{"CONFLICT_UNTIL_REMOVED": "1"}, "--recreate", "-y", "-c")
	if status != 0 || kinds(calls) != "secret,create,rm,create,secret,exec,run" {
		t.Fatalf("status=%d calls=%v: %s", status, calls, out)
	}
	secret := calls[4]
	if flagValue(secret, "--sandbox") != "reported" || flagValue(secret, "--env") != "API" || flagValue(secret, "--host") != "api.example.com" || flagValue(secret, "--command") != "true" {
		t.Fatal("current global secret not registered:", secret)
	}
	if !contains(calls[3], "API="+flagValue(secret, "--placeholder")) || !contains(calls[6], "--continue") {
		t.Fatal("creation placeholder or Pi continuation missing:", calls)
	}
}

func TestRecreateRejectsManagementFlags(t *testing.T) {
	for _, flag := range [][]string{{"-d", "."}, {"-l"}, {"--bridge-stop"}, {"--bridge-reload"}} {
		f := newFixture(t)
		args := append([]string{"--recreate"}, flag...)
		out, status, calls := f.launch("", nil, args...)
		if status == 0 || len(calls) != 0 || !strings.Contains(out, "--recreate cannot be combined") {
			t.Fatalf("status=%d calls=%v: %s", status, calls, out)
		}
	}
}

func TestRecreateRefusesUnexpectedSandboxIdentity(t *testing.T) {
	for _, name := range []string{"-unsafe", "other"} {
		p := newProcesses()
		replaced, status := p.recreate(name, options{name: "requested", yes: true})
		p.close()
		if replaced || status == 0 {
			t.Fatal("accepted unexpected identity:", name)
		}
	}
}
