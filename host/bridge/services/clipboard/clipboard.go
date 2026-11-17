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

// CommandResult identifies the backend and the exact prepared command copied.
type CommandResult struct {
	Backend    string `json:"backend"`
	CopiedText string `json:"copiedText"`
}

// CopyCommand prepares a Bash/zsh command that displays combined stdout/stderr
// and copies it back through the successful host clipboard backend. It only
// copies the command text; it never executes that command.
func CopyCommand(ctx context.Context, desktop *DesktopEnvironment, command string) (CommandResult, error) {
	return copyText(ctx, runtime.GOOS, desktop, command, true)
}

func captureCommand(command string, args []string) string {
	// All executable names and arguments come from clipboardCommands, not callers.
	backend := args[0]
	for _, arg := range args[1:] {
		backend += " '" + arg + "'"
	}
	return "(set -o pipefail; {\n" + command + "\n} 2>&1 | tee /dev/tty | " + backend + ")"
}

func copyClipboard(ctx context.Context, platform string, desktop *DesktopEnvironment, text string) (string, error) {
	result, err := copyText(ctx, platform, desktop, text, false)
	return result.Backend, err
}

func copyText(ctx context.Context, platform string, desktop *DesktopEnvironment, text string, shellCommand bool) (CommandResult, error) {
	if err := ValidateText(text); err != nil {
		return CommandResult{}, err
	}
	if shellCommand && (strings.TrimSpace(text) == "" || strings.ContainsRune(text, 0)) {
		return CommandResult{}, errors.New("shell command must be nonempty and contain no NUL bytes")
	}
	if desktop == nil {
		desktop = CurrentDesktop()
	}
	interrupted := errors.New("clipboard request expired or cancelled; clipboard may have changed; do not retry automatically")
	if ctx.Err() != nil {
		return CommandResult{}, interrupted
	}
	for _, args := range clipboardCommands(platform, desktop) {
		copiedText := text
		if shellCommand {
			copiedText = captureCommand(text, args)
		}
		if err := ValidateText(copiedText); err != nil {
			return CommandResult{}, errors.New("prepared clipboard text exceeds 16384 bytes; shorten the command")
		}
		// Bound each failed backend so a broken Wayland tool cannot prevent X11
		// fallback. Discard output: clipboard helpers can echo sensitive input.
		attempt, cancel := context.WithTimeout(ctx, 2*time.Second)
		cmd := exec.CommandContext(attempt, args[0], args[1:]...)
		cmd.Env = desktop.environ()
		cmd.Stdin = strings.NewReader(copiedText)
		cmd.WaitDelay = 100 * time.Millisecond
		err := cmd.Run()
		cancel()
		if err == nil {
			return CommandResult{Backend: args[0], CopiedText: copiedText}, nil
		}
		if ctx.Err() != nil {
			return CommandResult{}, interrupted
		}
	}
	return CommandResult{}, errors.New("host clipboard unavailable: use pbcopy on macOS, wl-copy on Wayland, or xclip/xsel on X11; check host PATH and graphical-session access, then reattach if the desktop environment changed")
}
