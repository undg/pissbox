package clipboard

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestCommandCopyPreparesWithoutExecutingAndCapturesOutputOnPaste(t *testing.T) {
	t.Setenv("BASH_ENV", "")
	t.Setenv("ENV", "")
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		backend, platform string
		args              []string
	}{
		{"pbcopy", "darwin", nil},
		{"wl-copy", "linux", []string{"--type", "text/plain;charset=utf-8"}},
		{"xclip", "linux", []string{"-selection", "clipboard"}},
		{"xsel", "linux", []string{"--clipboard", "--input"}},
	} {
		t.Run(tc.backend, func(t *testing.T) {
			dir := clipboardDirectory(t)
			t.Setenv("PATH", dir)
			t.Setenv("DISPLAY", ":1")
			t.Setenv("WAYLAND_DISPLAY", "wayland-1")
			copied, visible, arguments, marker := filepath.Join(dir, "copied"), filepath.Join(dir, "visible"), filepath.Join(dir, "args"), filepath.Join(dir, "executed")
			t.Setenv("CLIP_TEST_OUT", copied)
			t.Setenv("CLIP_TEST_VISIBLE", visible)
			t.Setenv("CLIP_TEST_ARGS", arguments)
			t.Setenv("CLIP_TEST_MARKER", marker)
			// Substitute a file for /dev/tty, but use real tee for the pipeline.
			tee := "#!/bin/sh\n[ \"$#\" -eq 1 ] && [ \"$1\" = /dev/tty ] || exit 99\nexec /usr/bin/tee \"$CLIP_TEST_VISIBLE\"\n"
			backend := "#!/bin/sh\n: > \"$CLIP_TEST_ARGS\"\nfor arg do printf '%s\\n' \"$arg\" >> \"$CLIP_TEST_ARGS\"; done\n/bin/cat > \"$CLIP_TEST_OUT\"\n"
			for name, script := range map[string]string{tc.backend: backend, "tee": tee} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
					t.Fatal(err)
				}
			}
			command := "printf 'out 世界\\n'; printf 'err\\n' >&2; printf ran > \"$CLIP_TEST_MARKER\"; exit 7 # trailing comment"
			var result CommandResult
			if tc.platform == runtime.GOOS {
				result, err = CopyCommand(context.Background(), nil, command)
			} else {
				result, err = copyText(context.Background(), tc.platform, nil, command, true)
			}
			if err != nil || result.Backend != tc.backend {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			payload, err := os.ReadFile(copied)
			if err != nil || string(payload) != result.CopiedText {
				t.Fatalf("wrong clipboard payload: %q, %v", payload, err)
			}
			if !strings.Contains(result.CopiedText, "\n"+command+"\n") {
				t.Fatal("original command was altered", result.CopiedText)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("copy executed the user command", err)
			}

			// Simulate the user pasting it. pipefail must not leak into their shell.
			run := exec.Command(bash, "-c", "set +o pipefail\n"+result.CopiedText+"\nstatus=$?\ncase :$SHELLOPTS: in *:pipefail:*) exit 99;; esac\nexit \"$status\"")
			if output, err := run.CombinedOutput(); err == nil || run.ProcessState == nil || run.ProcessState.ExitCode() != 7 {
				t.Fatalf("exit status not preserved: %v, output=%q", err, output)
			}
			for _, file := range []string{copied, visible} {
				if output, err := os.ReadFile(file); err != nil || string(output) != "out 世界\nerr\n" {
					t.Fatalf("%s: %q, %v", file, output, err)
				}
			}
			if data, err := os.ReadFile(marker); err != nil || string(data) != "ran" {
				t.Fatalf("pasted command did not execute: %q, %v", data, err)
			}
			data, err := os.ReadFile(arguments)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			if len(data) != 0 {
				got = strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
			}
			if !reflect.DeepEqual(got, tc.args) {
				t.Fatalf("clipboard arguments=%q, want %q", got, tc.args)
			}
		})
	}
}

func TestCaptureCommandSupportsCompoundCommandsPipelinesAndHeredocs(t *testing.T) {
	t.Setenv("BASH_ENV", "")
	t.Setenv("ENV", "")
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatal(err)
	}
	dir := clipboardDirectory(t)
	t.Setenv("PATH", dir)
	t.Setenv("CLIP_TEST_VISIBLE", filepath.Join(dir, "visible"))
	t.Setenv("CLIP_TEST_OUT", filepath.Join(dir, "copied"))
	for name, script := range map[string]string{
		"tee":    "#!/bin/sh\n/usr/bin/tee \"$CLIP_TEST_VISIBLE\"\nexit \"${CLIP_TEST_TEE_STATUS:-0}\"\n",
		"pbcopy": "#!/bin/sh\n/bin/cat > \"$CLIP_TEST_OUT\"\nexit \"${CLIP_TEST_BACKEND_STATUS:-0}\"\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		command, output          string
		status                   int
		backendStatus, teeStatus string
	}{
		{"printf 'one\\n' && printf 'two\\n'", "one\ntwo\n", 0, "", ""},
		{"printf output | /bin/cat", "output", 0, "", ""},
		{"false | /bin/cat", "", 1, "", ""},
		{"/bin/cat <<'EOF'\nhere doc\nEOF", "here doc\n", 0, "", ""},
		{"printf 'ok\\n' # final comment", "ok\n", 0, "", ""},
		{"true", "", 0, "", ""},
		{"printf 'clipboard failed\\n'", "clipboard failed\n", 23, "23", ""},
		{"printf 'tee failed\\n'", "tee failed\n", 31, "", "31"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			t.Setenv("CLIP_TEST_BACKEND_STATUS", tc.backendStatus)
			t.Setenv("CLIP_TEST_TEE_STATUS", tc.teeStatus)
			cmd := exec.Command(bash, "-c", captureCommand(tc.command, []string{"pbcopy"}))
			output, err := cmd.CombinedOutput()
			if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != tc.status {
				t.Fatalf("status=%v err=%v output=%q", cmd.ProcessState, err, output)
			}
			for _, name := range []string{"visible", "copied"} {
				if got, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(got) != tc.output {
					t.Fatalf("%s: %q, %v", name, got, err)
				}
			}
		})
	}
}

func TestCommandCopyFallbackUsesTheBackendThatActuallySucceeded(t *testing.T) {
	dir := clipboardDirectory(t)
	t.Setenv("PATH", dir)
	t.Setenv("CLIP_TEST_OUT", filepath.Join(dir, "copied"))
	for name, script := range map[string]string{
		"wl-copy": "#!/bin/sh\nexit 1\n",
		"xclip":   "#!/bin/sh\n/bin/cat > \"$CLIP_TEST_OUT\"\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	result, err := copyText(context.Background(), "linux", &DesktopEnvironment{Display: ":1", WaylandDisplay: "wayland-1"}, "printf test", true)
	if err != nil || result.Backend != "xclip" || !strings.HasSuffix(result.CopiedText, "| xclip '-selection' 'clipboard')") {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "copied")); err != nil || string(got) != result.CopiedText {
		t.Fatalf("wrong fallback payload: %q, %v", got, err)
	}
}

func TestCommandCopyRejectsInvalidOrOversizedPreparedTextBeforeWriting(t *testing.T) {
	dir := clipboardDirectory(t)
	t.Setenv("PATH", dir)
	t.Setenv("CLIP_TEST_MARKER", filepath.Join(dir, "called"))
	if err := os.WriteFile(filepath.Join(dir, "pbcopy"), []byte("#!/bin/sh\nprintf called > \"$CLIP_TEST_MARKER\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"", " \n\t", "printf\x00bad", strings.Repeat("x", maxTextBytes)} {
		if _, err := copyText(context.Background(), "darwin", nil, command, true); err == nil {
			t.Fatalf("accepted invalid command of %d bytes", len(command))
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "called")); !os.IsNotExist(err) {
		t.Fatal("invalid command touched clipboard", err)
	}
}
