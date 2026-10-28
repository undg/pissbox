package launcher

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Re-execute the instrumented test executable, never an ordinary go-build
// candidate: coverage and mutation must reach production orchestration.
func TestMain(m *testing.M) {
	if name := filepath.Base(os.Args[0]); name == "sbx" || strings.HasPrefix(name, "go-bridge-") {
		os.Exit(testTool())
	}
	switch os.Getenv("PISSBOX_LAUNCHER_TEST_PROCESS") {
	case "launcher":
		os.Exit(run(os.Getenv("LAUNCHER_ROOT"), os.Getenv("LAUNCHER_HOST"), os.Args[1:]))
	case "tool":
		os.Exit(testTool())
	}
	os.Exit(m.Run())
}
func testTool() int {
	args := os.Args[1:]
	log, _ := os.OpenFile(os.Getenv("LAUNCHER_LOG"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	defer log.Close()
	kind := filepath.Base(os.Args[0])
	if kind != "sbx" {
		if args[0] == "register" {
			if os.Getenv("BRIDGE_FAIL") == "1" {
				return 6
			}
			for i, arg := range args {
				if arg == "--owner-pid" {
					pid, _ := strconv.Atoi(args[i+1])
					if syscall.Kill(pid, 0) != nil {
						return 8
					}
				}
			}
			if os.Getenv("BRIDGE_EMPTY") != "1" {
				fmt.Fprintln(os.Stdout, "fixture-registration")
			}
		}
		json.NewEncoder(log).Encode(append([]string{"bridge"}, args...))
		if args[0] == "unregister" && os.Getenv("UNREGISTER_FAIL") == "1" {
			return 9
		}
		return 0
	}
	json.NewEncoder(log).Encode(args)
	if os.Getenv("REQUIRE_TTY") == "1" && (!isTerminal(os.Stdin) || !isTerminal(os.Stdout)) {
		fmt.Fprintln(os.Stderr, "lost terminal identity")
		return 20
	}
	switch args[0] {
	case "secret":
		if args[len(args)-1] == "--help" {
			if os.Getenv("SECRET_UNSUPPORTED") == "1" {
				return 1
			}
			help := "--command string --sandbox string --env string "
			if missing := os.Getenv("SECRET_MISSING_FLAG"); missing != "" {
				help = strings.ReplaceAll(help, missing+" ", "")
			}
			fmt.Fprint(os.Stdout, help)
			return 0
		}
		n, _ := strconv.Atoi(os.Getenv("SECRET_STATUS"))
		return n
	case "ls":
		fmt.Fprint(os.Stdout, os.Getenv("LISTING"))
	case "create":
		if os.Getenv("CREATE_FAIL") != "" {
			n, _ := strconv.Atoi(os.Getenv("CREATE_FAIL"))
			return n
		}
		if os.Getenv("CONFLICT") != "" {
			fmt.Fprintln(os.Stderr, `sandbox "reported" already exists for this workspace; pass --name to create another, or start the existing sandbox`)
			return 8
		}
		for _, arg := range args {
			if strings.HasPrefix(arg, "PISSBOX_ISOLATION_DIR=") {
				mailbox := strings.TrimPrefix(arg, "PISSBOX_ISOLATION_DIR=")
				if os.Getenv("NO_IDENTITY") != "1" {
					os.WriteFile(filepath.Join(mailbox, "name"), []byte(fallback(os.Getenv("CREATED_NAME"), "reported")+"\n"), 0600)
				}
			}
		}
		if os.Getenv("NEW_SYMLINK") != "" {
			os.Symlink("elsewhere", os.Getenv("NEW_SYMLINK"))
		}
		if os.Getenv("NEW_FILE") != "" {
			os.WriteFile(os.Getenv("NEW_FILE"), nil, 0600)
		}
		if os.Getenv("DENY_MKDIR") != "" {
			os.Chmod(os.Getenv("DENY_MKDIR"), 0500)
		}
	case "run":
		initial := false
		for _, arg := range args {
			if strings.HasPrefix(arg, "PISSBOX_WORKSPACE_DIR=") {
				initial = true
			}
		}
		if os.Getenv("WAIT_RUN") == "1" && (!initial || os.Getenv("CONFLICT") == "") {
			fmt.Fprintln(os.Stderr, "child-waiting")
			for {
				time.Sleep(time.Second)
			}
		}
		if initial && os.Getenv("CONFLICT") != "" {
			fmt.Fprintln(os.Stderr, "sandbox 'reported' already exists and can't be given new workspaces")
			return 8
		}
		n, _ := strconv.Atoi(os.Getenv("RUN_STATUS"))
		return n
	case "mount":
		n, _ := strconv.Atoi(os.Getenv("MOUNT_STATUS"))
		return n
	case "exec":
		if args[len(args)-1] == "pissbox-readiness" {
			n, _ := strconv.Atoi(os.Getenv("READINESS_STATUS"))
			return n
		}
		if args[len(args)-1] == "bash" {
			n, _ := strconv.Atoi(os.Getenv("SHELL_STATUS"))
			return n
		}
		for _, arg := range args {
			if arg == "isolation-mount" {
				n, _ := strconv.Atoi(os.Getenv("PROBE_STATUS"))
				return n
			}
		}
		n, _ := strconv.Atoi(os.Getenv("FINALIZE_STATUS"))
		return n
	case "rm":
		n, _ := strconv.Atoi(os.Getenv("REMOVE_STATUS"))
		return n
	}
	return 0
}

type fixture struct {
	t                                         *testing.T
	root, project, pi, state, log, tools, exe string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	base := os.Getenv("PISSBOX_BRIDGE_TEST_FIXTURE_DIR")
	if base == "" {
		base = filepath.Join("..", "..", "..", ".cache", "test-fixtures")
	}
	if err := os.MkdirAll(base, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(base, "launcher-")
	if err != nil {
		t.Fatal(err)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	f := fixture{t: t, root: root, project: filepath.Join(root, "project"), pi: filepath.Join(root, "pi"), state: filepath.Join(root, "state"), log: filepath.Join(root, "calls.jsonl"), tools: filepath.Join(root, "tools"), exe: exe}
	for _, dir := range []string{f.project, f.pi, f.tools} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, profile := range []string{"settings-co.json", "settings-go.json"} {
		f.write(filepath.Join(f.pi, profile), "{}")
	}
	// Keep argv[0] distinguishable in the subprocess fixture.
	if err := os.Symlink(exe, filepath.Join(f.tools, "sbx")); err != nil {
		t.Fatal(err)
	}
	return f
}
func (f fixture) write(path, text string) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		f.t.Fatal(err)
	}
}
func (f fixture) config(text string) {
	f.t.Helper()
	resources := os.Getenv("PISSBOX_LAUNCHER_TEST_RESOURCE_DIR")
	if resources == "" {
		_, source, _, _ := runtime.Caller(0)
		resources = filepath.Join(filepath.Dir(source), "..", "..", "..")
	}
	source := filepath.Join(resources, "sbx-kit/files/home/.pissbox/scripts/project-setup.sh")
	data, err := os.ReadFile(source)
	if err != nil {
		f.t.Fatalf("validator fixture resource: %v; set PISSBOX_LAUNCHER_TEST_RESOURCE_DIR for copied modules", err)
	}
	f.write(filepath.Join(f.root, "sbx-kit/files/home/.pissbox/scripts/project-setup.sh"), string(data))
	f.write(filepath.Join(f.project, ".pissbox.toml"), text)
}
func (f fixture) bridge() {
	f.t.Helper()
	binary := bridgeBinary(f.root)
	if err := os.MkdirAll(filepath.Dir(binary), 0700); err != nil {
		f.t.Fatal(err)
	}
	if err := os.Symlink(f.exe, binary); err != nil {
		f.t.Fatal(err)
	}
	for _, path := range []string{"extensions/pissbox-bridge.js", "extensions/lib/agent-messaging.js", "extensions/lib/agent-identity.js", "extensions/agent-names.txt"} {
		f.write(filepath.Join(f.pi, path), "")
	}
}
func (f fixture) command(input string, env map[string]string, args ...string) *exec.Cmd {
	f.t.Helper()
	cmd := exec.Command(f.exe, args...)
	cmd.Dir = f.project
	cmd.Stdin = strings.NewReader(input)
	values := map[string]string{}
	for _, pair := range os.Environ() {
		key, value, _ := strings.Cut(pair, "=")
		if !strings.HasPrefix(key, "PISSBOX_") && !strings.HasPrefix(key, "HERDR_") {
			values[key] = value
		}
	}
	values["PISSBOX_LAUNCHER_TEST_PROCESS"] = "launcher"
	values["LAUNCHER_ROOT"] = f.root
	values["LAUNCHER_HOST"] = "other"
	values["LAUNCHER_LOG"] = f.log
	values["PATH"] = f.tools + string(os.PathListSeparator) + os.Getenv("PATH")
	values["PISSBOX_PI_DIR"] = f.pi
	values["PISSBOX_STATE_DIR"] = f.state
	values["GORACE"] = "atexit_sleep_ms=0"
	// Subprocesses dispatch by argv[0] in TestMain; launcher mode is replaced
	// there when the executable is invoked through a tool symlink.
	for key, value := range env {
		values[key] = value
	}
	for key, value := range values {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	return cmd
}
func (f fixture) launch(input string, env map[string]string, args ...string) (string, int, [][]string) {
	f.t.Helper()
	os.Remove(f.log)
	cmd := f.command(input, env, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.String(), exitStatus(err), f.calls()
}
func (f fixture) calls() [][]string {
	f.t.Helper()
	data, err := os.ReadFile(f.log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		f.t.Fatal(err)
	}
	var calls [][]string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var args []string
		if err := json.Unmarshal([]byte(line), &args); err != nil {
			f.t.Fatal(err)
		}
		calls = append(calls, args)
	}
	return calls
}
func kinds(calls [][]string) string {
	var result []string
	for _, args := range calls {
		if args[0] == "bridge" {
			result = append(result, "bridge-"+args[1])
		} else {
			result = append(result, args[0])
		}
	}
	return strings.Join(result, ",")
}

func TestLaunchPreservesArgumentsAndPhysicalDeduplication(t *testing.T) {
	f := newFixture(t)
	f.config("workspaces = [\".\", \"..\"]\nsetup = [\"touch never-on-host\"]\n")
	out, status, calls := f.launch("", nil, ".", "-c", "--name=first", "--name", "last", ".")
	if status != 0 {
		t.Fatalf("status %d: %s", status, out)
	}
	if kinds(calls) != "run" {
		t.Fatal(calls)
	}
	args := calls[0]
	expected := []string{"run", "--env", "PISSBOX_PI_DIR=" + f.pi, "--env", "PISSBOX_STATE_DIR=" + f.state, "--env", "PISSBOX_WORKSPACE_DIR=" + f.project + "/.", "--name", "last", f.root + "/bin/../sbx-kit", ".", f.root, f.pi, f.state, "--", "--continue"}
	if fmt.Sprint(args) != fmt.Sprint(expected) {
		t.Fatalf("got %q\nwant %q", args, expected)
	}
	if _, err := os.Stat(filepath.Join(f.project, "never-on-host")); !os.IsNotExist(err) {
		t.Fatal("host executed setup")
	}
}

func TestInvalidConfigPrecedesEverySideEffect(t *testing.T) {
	for _, text := range []string{"setup = 5", "workspaces = [1]", "workspaces = [\"missing\"]", "isolated_directories = [\"../bad\"]"} {
		t.Run(text, func(t *testing.T) {
			f := newFixture(t)
			f.config(text)
			out, status, calls := f.launch("", nil)
			if status == 0 || len(calls) != 0 {
				t.Fatalf("status=%d calls=%v: %s", status, calls, out)
			}
			for _, path := range []string{f.state, filepath.Join(f.pi, "settings.json")} {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatalf("unexpected effect: %s", path)
				}
			}
		})
	}
}
func TestIgnoreConfigSkipsOnlyWorkspaceDirectoryChecks(t *testing.T) {
	f := newFixture(t)
	f.config("workspaces = [\"missing\"]\n")
	out, status, calls := f.launch("", nil, "--ignore-config-workspaces")
	if status != 0 || kinds(calls) != "run" {
		t.Fatalf("%d %v %s", status, calls, out)
	}
	f.config("workspaces = 5")
	_, status, calls = f.launch("", nil, "--ignore-config-workspaces")
	if status == 0 || len(calls) != 0 {
		t.Fatal(status, calls)
	}
}
func TestExistingSandboxChoicesAndStatuses(t *testing.T) {
	for _, tc := range []struct {
		input  string
		extra  []string
		env    map[string]string
		want   string
		status int
	}{
		{"\n", nil, nil, "run,exec,run", 0}, {"C", nil, nil, "run,exec,run", 0}, {"s", nil, nil, "run,exec,exec", 0}, {"q", nil, nil, "run", 0}, {"dy\n", nil, nil, "run,rm", 0}, {"dn\n", nil, nil, "run", 0}, {"dy", nil, nil, "run", 1}, {"", nil, nil, "run", 1}, {"xyq", nil, nil, "run", 0},
		{"dyes\n", []string{"-y"}, nil, "run,exec,run", 0},
		{"p", nil, map[string]string{"READINESS_STATUS": "19"}, "run,exec", 19},
		{"s", nil, map[string]string{"SHELL_STATUS": "9"}, "run,exec,exec", 9},
		{"dy\n", nil, map[string]string{"REMOVE_STATUS": "7"}, "run,rm", 7},
	} {
		t.Run(tc.input+tc.want, func(t *testing.T) {
			f := newFixture(t)
			env := map[string]string{"CONFLICT": "1"}
			for k, v := range tc.env {
				env[k] = v
			}
			out, status, calls := f.launch(tc.input, env, tc.extra...)
			if status != tc.status || kinds(calls) != tc.want {
				t.Fatalf("%d %v: %s", status, calls, out)
			}
			if len(calls) > 1 && calls[1][0] == "exec" && calls[1][3] != "reported" {
				t.Fatal(calls)
			}
			if tc.input == "C" {
				last := calls[len(calls)-1]
				if strings.Join(last[len(last)-2:], ",") != "--,--continue" {
					t.Fatal(last)
				}
			}
		})
	}
}
func TestBridgeLifecycleDoesNotReplaceSessionFailure(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(strconv.FormatBool(failed), func(t *testing.T) {
			f := newFixture(t)
			f.bridge()
			env := map[string]string{"CONFLICT": "1", "RUN_STATUS": "42"}
			if failed {
				env["BRIDGE_FAIL"] = "1"
			}
			out, status, calls := f.launch("C", env)
			want := "bridge-register,run,bridge-unregister,exec,bridge-register,run,bridge-unregister"
			if failed {
				want = "run,exec,run"
			}
			if status != 42 || kinds(calls) != want {
				t.Fatalf("%d %v: %s", status, calls, out)
			}
			for _, call := range calls {
				if call[0] == "run" {
					hasMailbox := strings.Contains(strings.Join(call, " "), "PISSBOX_BRIDGE_DIR=")
					if hasMailbox == failed {
						t.Fatal(call)
					}
				}
			}
		})
	}
}
func TestIsolationOrderingAndFailClosed(t *testing.T) {
	for _, tc := range []struct {
		env    map[string]string
		want   string
		status int
	}{
		{nil, "create,mount,exec,exec,run", 0},
		{map[string]string{"CREATE_FAIL": "7"}, "create", 7},
		{map[string]string{"NO_IDENTITY": "1"}, "create", 1},
		{map[string]string{"MOUNT_STATUS": "9"}, "create,mount", 9},
		{map[string]string{"PROBE_STATUS": "13"}, "create,mount,exec", 13},
		{map[string]string{"FINALIZE_STATUS": "23"}, "create,mount,exec,exec", 23},
		{map[string]string{"RUN_STATUS": "42"}, "create,mount,exec,exec,run", 42},
		{map[string]string{"CONFLICT": "1", "READINESS_STATUS": "17"}, "create,exec", 17},
	} {
		t.Run(tc.want+strconv.Itoa(tc.status), func(t *testing.T) {
			f := newFixture(t)
			f.config("isolated_directories = [\"node_modules\"]\n")
			out, status, calls := f.launch("", tc.env, "-y", "-c")
			if status != tc.status || kinds(calls) != tc.want {
				t.Fatalf("%d %v: %s", status, calls, out)
			}
			if calls[0][0] == "create" && strings.Contains(strings.Join(calls[0], " "), "--continue") {
				t.Fatal(calls)
			}
			if status == 0 {
				last := calls[len(calls)-1]
				if fmt.Sprint(last) != fmt.Sprint([]string{"run", "--name", "reported", "--", "--continue"}) {
					t.Fatal(last)
				}
			}
		})
	}
}
func TestIsolationRechecksComponentsAfterCreation(t *testing.T) {
	for _, change := range []string{"NEW_SYMLINK", "NEW_FILE"} {
		t.Run(change, func(t *testing.T) {
			f := newFixture(t)
			f.config("isolated_directories = [\"node_modules\"]")
			out, status, calls := f.launch("", map[string]string{change: filepath.Join(f.project, "node_modules")})
			if status == 0 || kinds(calls) != "create" || !strings.Contains(out, "never files or symlinks") {
				t.Fatalf("%d %v: %s", status, calls, out)
			}
		})
	}
}
func TestMountpointMkdirFailureStopsBeforeMount(t *testing.T) {
	f := newFixture(t)
	f.config("isolated_directories = [\"deps/node_modules\"]")
	if os.Geteuid() == 0 {
		t.Skip("root bypasses filesystem directory permissions")
	}
	t.Cleanup(func() { os.Chmod(f.project, 0700) })
	// A real filesystem failure after creation and path revalidation.
	out, status, calls := f.launch("", map[string]string{"DENY_MKDIR": f.project})
	if status == 0 || kinds(calls) != "create" || !strings.Contains(out, "permission denied") {
		t.Fatalf("%d %v: %s", status, calls, out)
	}
}
