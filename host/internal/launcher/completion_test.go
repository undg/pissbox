package launcher

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCLIWordsAreAlwaysWorkspaceNames(t *testing.T) {
	words := []string{"help", "completion", "list", "delete", "__complete", "__completeNoDesc"}
	o, err := parse(append([]string{"--name", "dev"}, words...))
	if err != nil || !reflect.DeepEqual(o.workspaces, words) {
		t.Fatalf("workspaces = %q, err = %v", o.workspaces, err)
	}
	f := newFixture(t)
	out, status, calls := f.launch("", nil, words...)
	if status != 0 || len(calls) != 1 {
		t.Fatalf("status = %d, calls = %v, output = %s", status, calls, out)
	}
	for _, word := range words {
		if !contains(calls[0], word) {
			t.Fatalf("workspace %q missing from %v", word, calls)
		}
	}
	// Even the internal completion flag is a path after --.
	o, err = parse([]string{"--", "--complete", "--completion"})
	if err != nil || !reflect.DeepEqual(o.workspaces, []string{"--complete", "--completion"}) {
		t.Fatal(o, err)
	}
}

func TestCompletionRequests(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"flags", []string{"--pissbox-"}, "--pissbox-pi-dir\t"},
		{"flags after workspace", []string{"project", "--na"}, "--name\t"},
		{"workspace directories", []string{""}, ":16\n"},
		{"more workspace directories", []string{"help", "__complete", ""}, ":16\n"},
		{"directory flag", []string{"--pissbox-pi-dir", ""}, ":16\n"},
		{"directory equals", []string{"--pissbox-state-dir=par"}, ":16\n"},
		{"delete directories", []string{"-d", ""}, ":16\n"},
		{"after separator", []string{"--", "--na"}, ":16\n"},
		{"sandbox name", []string{"--name", ""}, ":4\n"},
		{"shell names", []string{"--completion", ""}, "bash\nzsh\nfish\npowershell\n:4\n"},
		{"shell prefix", []string{"--completion=z"}, "zsh\n:4\n"},
		{"secret env", []string{"--secret", ""}, ":4\n"},
		{"secret host", []string{"--secret", "API", ""}, ":4\n"},
		{"secret command", []string{"--secret", "API", "example.com", ""}, ":4\n"},
		{"after secret", []string{"--secret", "API", "example.com", "get_token --name help", "--na"}, "--name\t"},
		{"workspace after secret", []string{"--secret", "API", "example.com", "get_token", ""}, ":16\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := requestCompletion(tc.args, &out); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Fatalf("completion = %q, want %q", out.String(), tc.want)
			}
			for _, command := range []string{"__complete", "help\t", "completion\t"} {
				if strings.Contains(out.String(), command) {
					t.Fatalf("suggested a subcommand: %q", out.String())
				}
			}
		})
	}
}

func TestGeneratedCompletionUsesFlagsOnly(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			var out bytes.Buffer
			if err := writeCompletion(shell, &out); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "--complete") || strings.Contains(out.String(), "__complete") {
				t.Fatal("script does not use the flag-only completion protocol")
			}
			if shell == "bash" {
				cmd := exec.Command("bash", "-n")
				cmd.Stdin = strings.NewReader(out.String())
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("invalid Bash completion: %v: %s", err, output)
				}
			}
		})
	}
	var out bytes.Buffer
	if err := writeCompletion("unknown", &out); err == nil || out.Len() != 0 {
		t.Fatal("accepted unknown shell or emitted a partial script")
	}
	if err := requestCompletion(nil, &out); err == nil {
		t.Fatal("accepted a request without a completion word")
	}
}

func TestGeneratedBashCompletionCallsFlagProtocol(t *testing.T) {
	f := newFixture(t)
	var script bytes.Buffer
	if err := writeCompletion("bash", &script); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(f.root, "completion.bash")
	f.write(path, script.String())
	// Exercise the generated script's subprocess request, not just its text.
	// The result-processing UI is owned by bash-completion, not the launcher.
	cmd := exec.Command("bash", "-c", `
source "$1"
words=("$2" --na)
cur=--na
__pissbox_get_completion_results
printf '%s\n:%s\n' "$out" "$directive"
`, "completion-test", path, f.exe)
	cmd.Dir = f.project
	cmd.Env = f.command("", nil).Env
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "--name\t") || !strings.Contains(string(output), ":4") {
		t.Fatalf("Bash completion: %v: %s", err, output)
	}
	if calls := f.calls(); len(calls) != 0 {
		t.Fatalf("completion called runtime tools: %v", calls)
	}
}

func TestHelpAndCompletionHaveNoLaunchSideEffects(t *testing.T) {
	for _, args := range [][]string{
		{"--help"}, {"--completion", "bash"}, {"--completion=zsh"},
		{"--completion", "fish"}, {"--completion", "powershell"},
		{"--complete", "--na"}, {"--complete", "--secret", "API", ""},
		{"--completion", "invalid"}, {"--complete"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			f := newFixture(t)
			f.write(filepath.Join(f.project, ".pissbox.toml"), "not valid TOML = [")
			marker := filepath.Join(f.root, "secret-command-ran")
			f.write(f.userConfig, "[secrets.API]\nhost = 'example.com'\ncommand = 'touch "+marker+"'\n")
			out, status, calls := f.launch("", map[string]string{"PISSBOX_PI_DIR": "missing"}, args...)
			wantFailure := args[len(args)-1] == "invalid" || len(args) == 1 && args[0] == "--complete"
			if (status != 0) != wantFailure || len(calls) != 0 {
				t.Fatalf("status = %d, calls = %v, output = %s", status, calls, out)
			}
			for _, path := range []string{marker, f.state, filepath.Join(f.pi, "settings.json")} {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatalf("unexpected side effect at %s: %v", path, err)
				}
			}
			if args[0] == "--help" && (strings.Contains(out, "Available Commands:") || !strings.Contains(out, "--completion SHELL")) {
				t.Fatalf("unexpected help: %s", out)
			}
		})
	}
}

func TestSecretParserErrorsDoNotPrintCommands(t *testing.T) {
	f := newFixture(t)
	command := "private-command --account confidential"
	out, status, _ := f.launch("", nil, "--secret", "API", "invalid/url", command)
	if status == 0 || strings.Contains(out, command) {
		t.Fatalf("status = %d, output = %s", status, out)
	}
	out, _, calls := f.launch("", nil, "--complete", "--secret", "API", "example.com", command, "--unknown", "")
	if strings.Contains(out, command) || len(calls) != 0 {
		t.Fatalf("completion leaked a command or called runtime tools: %v: %s", calls, out)
	}
}
