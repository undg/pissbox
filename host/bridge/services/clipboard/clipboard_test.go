package clipboard

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func clipboardDirectory(t *testing.T) string {
	t.Helper()
	base := os.Getenv("PISSBOX_BRIDGE_TEST_FIXTURE_DIR")
	if base == "" {
		base = filepath.Join("..", "..", "..", "..", ".cache", "test-fixtures")
	}
	base, err := filepath.Abs(base)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(base, 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(base, "clipboard-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestClipboardBackendSelection(t *testing.T) {
	for _, tc := range []struct {
		name, platform string
		desktop        DesktopEnvironment
		want           [][]string
	}{
		{"macOS", "darwin", DesktopEnvironment{}, [][]string{{"pbcopy"}}},
		{"Wayland and X11", "linux", DesktopEnvironment{Display: ":1", WaylandDisplay: "wayland-1"}, [][]string{{"wl-copy", "--type", "text/plain;charset=utf-8"}, {"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}}},
		{"X11", "linux", DesktopEnvironment{Display: ":1"}, [][]string{{"xclip", "-selection", "clipboard"}, {"xsel", "--clipboard", "--input"}}},
		{"Wayland", "linux", DesktopEnvironment{WaylandDisplay: "wayland-1"}, [][]string{{"wl-copy", "--type", "text/plain;charset=utf-8"}}},
		{"headless", "linux", DesktopEnvironment{}, nil},
		{"unsupported", "windows", DesktopEnvironment{}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := clipboardCommands(tc.platform, &tc.desktop); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("commands = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestClipboardCopiesExactTextAndDesktopEnvironmentThroughStdin(t *testing.T) {
	dir := clipboardDirectory(t)
	t.Setenv("PATH", dir)
	t.Setenv("DISPLAY", "stale-daemon-display")
	t.Setenv("XAUTHORITY", "stale-daemon-auth")
	for _, tc := range []struct{ backend, platform, args string }{
		{"pbcopy", "darwin", ""},
		{"wl-copy", "linux", "--type\ntext/plain;charset=utf-8\n"},
		{"xclip", "linux", "-selection\nclipboard\n"},
		{"xsel", "linux", "--clipboard\n--input\n"},
	} {
		t.Run(tc.backend, func(t *testing.T) {
			// Only this executable exists: earlier candidates must be skipped.
			command := filepath.Join(dir, tc.backend)
			script := "#!/bin/sh\n/bin/cat > \"$CLIP_TEST_OUT\"\nfor arg do printf '%s\\n' \"$arg\"; done > \"$CLIP_TEST_ARGS\"\nprintf '%s|%s|%s|%s' \"$DISPLAY\" \"$WAYLAND_DISPLAY\" \"$XDG_RUNTIME_DIR\" \"$XAUTHORITY\" > \"$CLIP_TEST_ENV\"\n"
			if err := os.WriteFile(command, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			defer os.Remove(command)
			out, args, env := filepath.Join(dir, "out"), filepath.Join(dir, "args"), filepath.Join(dir, "env")
			t.Setenv("CLIP_TEST_OUT", out)
			t.Setenv("CLIP_TEST_ARGS", args)
			t.Setenv("CLIP_TEST_ENV", env)
			desktop := &DesktopEnvironment{Display: ":9", WaylandDisplay: "wayland-9", RuntimeDir: "/run/user/test", Xauthority: ""}
			for _, text := range []string{"hello 世界\n' $(touch should-not-exist)\n\t", "no trailing newline", ""} {
				backend, err := copyClipboard(context.Background(), tc.platform, desktop, text)
				if err != nil || backend != tc.backend {
					t.Fatalf("backend=%q err=%v", backend, err)
				}
				for file, want := range map[string]string{out: text, args: tc.args, env: ":9|wayland-9|/run/user/test|"} {
					got, err := os.ReadFile(file)
					if err != nil || string(got) != want {
						t.Fatalf("%s = %q, %v; want %q", file, got, err, want)
					}
				}
			}
		})
	}
}

func TestClipboardFallsBackOnFailureAndTimeoutWithoutExposingText(t *testing.T) {
	dir := clipboardDirectory(t)
	t.Setenv("PATH", dir)
	for name, script := range map[string]string{
		"wl-copy": "#!/bin/sh\nexec /bin/sleep 30\n",
		"xclip":   "#!/bin/sh\n/bin/cat >&2\nexit 1\n",
		"xsel":    "#!/bin/sh\n/bin/cat > /dev/null\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	desktop := &DesktopEnvironment{Display: ":1", WaylandDisplay: "wayland-1"}
	backend, err := copyClipboard(context.Background(), "linux", desktop, "private clipboard text")
	if err != nil || backend != "xsel" {
		t.Fatalf("backend=%q err=%v", backend, err)
	}
	if err := os.Remove(filepath.Join(dir, "xsel")); err != nil {
		t.Fatal(err)
	}
	desktop.WaylandDisplay = ""
	_, err = copyClipboard(context.Background(), "linux", desktop, "private clipboard text")
	if err == nil || !strings.Contains(err.Error(), "host PATH") || strings.Contains(err.Error(), "private clipboard text") {
		t.Fatal(err)
	}
}

func TestClipboardRejectsInvalidTextAndExpiredRequestsBeforeExecution(t *testing.T) {
	dir := clipboardDirectory(t)
	t.Setenv("PATH", dir)
	marker := filepath.Join(dir, "called")
	t.Setenv("CLIP_TEST_MARKER", marker)
	if err := os.WriteFile(filepath.Join(dir, "pbcopy"), []byte("#!/bin/sh\nprintf called > \"$CLIP_TEST_MARKER\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{strings.Repeat("a", maxTextBytes+1), string([]byte{0xff})} {
		if _, err := copyClipboard(context.Background(), "darwin", nil, text); err == nil {
			t.Fatal("accepted invalid text")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := copyClipboard(ctx, "darwin", nil, "text"); err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("invalid request executed a command", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "xclip"), []byte("#!/bin/sh\nexec /bin/sleep 30\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "xsel"), []byte("#!/bin/sh\nprintf called > \"$CLIP_TEST_MARKER\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	deadline, stop := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer stop()
	if _, err := copyClipboard(deadline, "linux", &DesktopEnvironment{Display: ":1"}, "text"); err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("cancelled request tried another backend", err)
	}
}

func TestCopyUsesCurrentHostDesktopAndValidatesBoundary(t *testing.T) {
	dir := clipboardDirectory(t)
	t.Setenv("PATH", dir)
	t.Setenv("DISPLAY", ":7")
	t.Setenv("WAYLAND_DISPLAY", "wayland-7")
	t.Setenv("XDG_RUNTIME_DIR", "/run/test-7")
	t.Setenv("XAUTHORITY", "auth-7")
	want := &DesktopEnvironment{Display: ":7", WaylandDisplay: "wayland-7", RuntimeDir: "/run/test-7", Xauthority: "auth-7"}
	if got := CurrentDesktop(); *got != *want {
		t.Fatalf("desktop = %+v, want %+v", got, want)
	}
	out := filepath.Join(dir, "copied")
	t.Setenv("CLIP_TEST_OUT", out)
	for _, name := range []string{"pbcopy", "wl-copy", "xclip", "xsel"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n/bin/cat > \"$CLIP_TEST_OUT\"\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	text := strings.Repeat("x", maxTextBytes)
	backend, err := Copy(context.Background(), nil, text)
	wantBackend := "wl-copy"
	if runtime.GOOS == "darwin" {
		wantBackend = "pbcopy"
	}
	if err != nil || backend != wantBackend {
		t.Fatalf("backend=%q err=%v", backend, err)
	}
	if got, err := os.ReadFile(out); err != nil || string(got) != text {
		t.Fatalf("copied text differs: bytes=%d, err=%v", len(got), err)
	}
}
