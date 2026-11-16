// Package clipboard provides write-only host clipboard access through fixed
// desktop commands. It has no dependency on Pi, mailbox transport, or launchers.
package clipboard

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"
)

const maxTextBytes = 16 * 1024

// DesktopEnvironment selects a host graphical session. Callers must obtain
// these connection details from the host, never from sandbox requests.
type DesktopEnvironment struct {
	Display        string `json:"display"`
	WaylandDisplay string `json:"waylandDisplay"`
	RuntimeDir     string `json:"runtimeDir"`
	Xauthority     string `json:"xauthority"`
}

// CurrentDesktop captures the current process's graphical-session environment.
func CurrentDesktop() *DesktopEnvironment {
	return &DesktopEnvironment{os.Getenv("DISPLAY"), os.Getenv("WAYLAND_DISPLAY"), os.Getenv("XDG_RUNTIME_DIR"), os.Getenv("XAUTHORITY")}
}

func (e *DesktopEnvironment) environ() []string {
	return append(os.Environ(), "DISPLAY="+e.Display, "WAYLAND_DISPLAY="+e.WaylandDisplay, "XDG_RUNTIME_DIR="+e.RuntimeDir, "XAUTHORITY="+e.Xauthority)
}

func clipboardCommands(platform string, desktop *DesktopEnvironment) [][]string {
	switch platform {
	case "darwin":
		return [][]string{{"pbcopy"}}
	case "linux":
		var commands [][]string
		if desktop.WaylandDisplay != "" {
			commands = append(commands, []string{"wl-copy", "--type", "text/plain;charset=utf-8"})
		}
		if desktop.Display != "" {
			commands = append(commands, []string{"xclip", "-selection", "clipboard"}, []string{"xsel", "--clipboard", "--input"})
		}
		return commands
	default:
		return nil
	}
}

// ValidateText rejects invalid UTF-8 or text larger than 16 KiB. Empty text is valid.
func ValidateText(text string) error {
	if len(text) > maxTextBytes || !utf8.ValidString(text) {
		return errors.New("clipboard text must be valid UTF-8 and at most 16384 bytes")
	}
	return nil
}

// Copy writes text exactly and returns the successful backend's name. Empty text
// clears the clipboard. A nil desktop uses CurrentDesktop. Commands inherit the
// host PATH; each attempt has a two-second timeout within ctx's deadline.
// An interrupted or failed write may already have changed the clipboard.
func Copy(ctx context.Context, desktop *DesktopEnvironment, text string) (string, error) {
	return copyClipboard(ctx, runtime.GOOS, desktop, text)
}

func copyClipboard(ctx context.Context, platform string, desktop *DesktopEnvironment, text string) (string, error) {
	if err := ValidateText(text); err != nil {
		return "", err
	}
	if desktop == nil {
		desktop = CurrentDesktop()
	}
	interrupted := errors.New("clipboard request expired or cancelled; clipboard may have changed; do not retry automatically")
	if ctx.Err() != nil {
		return "", interrupted
	}
	for _, args := range clipboardCommands(platform, desktop) {
		// Bound each failed backend so a broken Wayland tool cannot prevent X11
		// fallback. Discard output: clipboard helpers can echo sensitive input.
		attempt, cancel := context.WithTimeout(ctx, 2*time.Second)
		cmd := exec.CommandContext(attempt, args[0], args[1:]...)
		cmd.Env = desktop.environ()
		cmd.Stdin = strings.NewReader(text)
		cmd.WaitDelay = 100 * time.Millisecond
		err := cmd.Run()
		cancel()
		if err == nil {
			return args[0], nil
		}
		if ctx.Err() != nil {
			return "", interrupted
		}
	}
	return "", errors.New("host clipboard unavailable: use pbcopy on macOS, wl-copy on Wayland, or xclip/xsel on X11; check host PATH and graphical-session access, then reattach if the desktop environment changed")
}
