package launcher

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSecretParserPreservesCommandsAndWorkspaceOrder(t *testing.T) {
	o, err := parse([]string{"one", "--secret", "API_ONE", "api.one.com", "security find-generic-password -s 'api one' -w", "two", "--secret", "API_TWO", "*.example.com", "/host/bin/get-token --account two", "-c", "--", "--secret"})
	if err != nil {
		t.Fatal(err)
	}
	want := []secret{
		{Env: "API_ONE", Host: "api.one.com", Command: "security find-generic-password -s 'api one' -w"},
		{Env: "API_TWO", Host: "*.example.com", Command: "/host/bin/get-token --account two"},
	}
	if !reflect.DeepEqual(o.secrets, want) || !reflect.DeepEqual(o.workspaces, []string{"one", "two", "--secret"}) || !o.continued {
		t.Fatalf("%+v", o)
	}
}

func TestSecretParserRejectsMissingUnsafeAndDuplicateArguments(t *testing.T) {
	for _, args := range [][]string{
		{"--secret"}, {"--secret", "API"}, {"--secret", "API", "example.com"},
		{"--secret", "", "example.com", "get_token"},
		{"--secret", "1API", "example.com", "get_token"},
		{"--secret", "API-KEY", "example.com", "get_token"},
		{"--secret", "PISSBOX_PI_DIR", "example.com", "get_token"},
		{"--secret", "SANDBOX_NAME", "example.com", "get_token"},
		{"--secret", "API", "", "get_token"},
		{"--secret", "API", "--name", "get_token"},
		{"--secret", "API", "https://example.com", "get_token"},
		{"--secret", "API", "example.com:443", "get_token"},
		{"--secret", "API", "example.com/path", "get_token"},
		{"--secret", "API", "example.com?key", "get_token"},
		{"--secret", "API", "example.com#fragment", "get_token"},
		{"--secret", "API", "user@example.com", "get_token"},
		{"--secret", "API", "example.com\\path", "get_token"},
		{"--secret", "API", "example.com\n", "get_token"},
		{"--secret", "API", "example.com", ""},
		{"--secret", "API", "example.com", " \t"},
		{"--secret", "API", "example.com", "--yes"},
		{"--secret", "API", "example.com", "get_token", "--secret", "API", "other.com", "get_other"},
	} {
		if _, err := parse(args); err == nil {
			t.Fatalf("accepted %q", args)
		}
	}
	for _, host := range []string{"example.com", "*.example.com", "**.example.com", "127.0.0.1", "::1"} {
		if _, err := parse([]string{"--secret", "_API_1", host, "get_token"}); err != nil {
			t.Fatal(host, err)
		}
	}
}

func TestSecretCreationScopesToReportedIdentityBeforeSetupAndPi(t *testing.T) {
	for _, isolated := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "isolated"}[isolated], func(t *testing.T) {
			f := newFixture(t)
			if isolated {
				f.config("isolated_directories = [\"deps\"]")
			}
			marker := filepath.Join(f.project, "must-not-execute-on-host")
			command := "touch '" + marker + "'"
			out, status, calls := f.launch("", nil, "--secret", "API", "*.example.com", command, "--secret", "SECOND", "other.com", "/host/bin/get-token --account two", "-c")
			want := "secret,create,secret,secret,exec,run"
			if isolated {
				want = "secret,create,mount,exec,secret,secret,exec,run"
			}
			if status != 0 || kinds(calls) != want {
				t.Fatalf("%d %v: %s", status, calls, out)
			}
			firstSecret := len(calls) - 4
			if !reflect.DeepEqual(calls[firstSecret], []string{"secret", "set-custom", "--sandbox", "reported", "--env", "API", "--host", "*.example.com", "--command", command}) {
				t.Fatal(calls[firstSecret])
			}
			if !reflect.DeepEqual(calls[firstSecret+1], []string{"secret", "set-custom", "--sandbox", "reported", "--env", "SECOND", "--host", "other.com", "--command", "/host/bin/get-token --account two"}) {
				t.Fatal(calls[firstSecret+1])
			}
			setup := calls[len(calls)-2]
			if setup[0] != "exec" || setup[1] != "--user" || setup[2] != "1000" || setup[3] != "reported" || !strings.Contains(setup[len(setup)-1], "--finalize-isolation") {
				t.Fatal(setup)
			}
			if !reflect.DeepEqual(calls[len(calls)-1], []string{"run", "--name", "reported", "--", "--continue"}) {
				t.Fatal(calls)
			}
			if strings.Contains(strings.Join(calls[1], " "), command) || strings.Contains(out, command) {
				t.Fatal("command escaped registration boundary", calls, out)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("launcher executed secret command")
			}
		})
	}
}

func TestSecretFailuresNeverStartPiOrReplaySetup(t *testing.T) {
	for _, tc := range []struct {
		name   string
		env    map[string]string
		want   string
		status int
	}{
		{"unsupported", map[string]string{"SECRET_UNSUPPORTED": "1"}, "secret", 1},
		{"missing command", map[string]string{"SECRET_MISSING_FLAG": "--command"}, "secret", 1},
		{"missing scope", map[string]string{"SECRET_MISSING_FLAG": "--sandbox"}, "secret", 1},
		{"missing env", map[string]string{"SECRET_MISSING_FLAG": "--env"}, "secret", 1},
		{"creation", map[string]string{"CREATE_FAIL": "7"}, "secret,create", 7},
		{"missing identity", map[string]string{"NO_IDENTITY": "1"}, "secret,create", 1},
		{"invalid identity", map[string]string{"CREATED_NAME": "-unsafe"}, "secret,create", 1},
		{"registration", map[string]string{"SECRET_STATUS": "19"}, "secret,create,secret", 19},
		{"setup", map[string]string{"FINALIZE_STATUS": "23"}, "secret,create,secret,exec", 23},
		{"attach", map[string]string{"RUN_STATUS": "42"}, "secret,create,secret,exec,run", 42},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			out, status, calls := f.launch("", tc.env, "--secret", "API", "example.com", "get_token")
			if status != tc.status || kinds(calls) != tc.want {
				t.Fatalf("%d %v: %s", status, calls, out)
			}
			if tc.want == "secret" {
				if _, err := os.Stat(f.state); !os.IsNotExist(err) {
					t.Fatal("created state before capability check")
				}
			}
		})
	}
}

func TestSecretFlagsNeverRegisterOnExistingSandboxActions(t *testing.T) {
	for _, tc := range []struct {
		input string
		extra []string
		want  string
	}{
		{"p", nil, "secret,create,exec,run"},
		{"c", nil, "secret,create,exec,run"},
		{"s", nil, "secret,create,exec,exec"},
		{"q", nil, "secret,create"},
		{"dy\n", nil, "secret,create,rm"},
		{"", []string{"-y"}, "secret,create,exec,run"},
	} {
		t.Run(tc.input+strings.Join(tc.extra, ""), func(t *testing.T) {
			f := newFixture(t)
			args := append([]string{"--secret", "API", "example.com", "get_token"}, tc.extra...)
			out, status, calls := f.launch(tc.input, map[string]string{"CONFLICT": "1"}, args...)
			if status != 0 || kinds(calls) != tc.want {
				t.Fatalf("%d %v: %s", status, calls, out)
			}
			for _, call := range calls[1:] {
				if call[0] == "secret" || strings.Contains(strings.Join(call, " "), "get_token") {
					t.Fatal("registered or forwarded secret on attach", call)
				}
			}
		})
	}
}

func TestSecretManagementConflictsAndHelpHaveNoEffects(t *testing.T) {
	for _, extra := range [][]string{{"--ls"}, {"--delete", "."}, {"--bridge-stop"}, {"--help"}} {
		f := newFixture(t)
		args := append([]string{"--secret", "API", "example.com", "get_token"}, extra...)
		out, status, calls := f.launch("", nil, args...)
		if len(calls) != 0 {
			t.Fatal(calls)
		}
		if extra[0] == "--help" {
			if status != 0 || !strings.Contains(out, "--secret ENV_NAME HOST COMMAND") {
				t.Fatal(status, out)
			}
		} else if status == 0 || !strings.Contains(out, "cannot be combined") {
			t.Fatal(status, out)
		}
	}
}
