package vault

import (
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func hostCommand(ctx context.Context, dir, command string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = dir
	// Do not inherit ambient repository/index overrides or interactive prompts.
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GIT_") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_TERMINAL_PROMPT=0", "GIT_LITERAL_PATHSPECS=1")
	return cmd
}
func recordHistory(ctx context.Context, d Definition, name string) error {
	// A parent repository must never silently become this vault's history.
	if _, err := os.Lstat(filepath.Join(d.Path, ".git")); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	git := func(args ...string) ([]byte, error) {
		prefix := []string{"-c", "core.hooksPath=" + os.DevNull, "-c", "commit.gpgSign=false"}
		return hostCommand(ctx, d.Path, "git", append(prefix, args...)...).Output()
	}
	top, err := git("rev-parse", "--show-toplevel")
	if err != nil {
		return errors.New("history repository unavailable")
	}
	canonical, err := filepath.EvalSymlinks(strings.TrimSpace(string(top)))
	if err != nil || canonical != d.Path {
		return errors.New("history root differs from vault")
	}
	// --only excludes all unrelated staged changes. No shell, hooks, pushes, or
	// user-provided commit arguments are allowed across this boundary.
	if _, err = git("add", "--", name); err != nil {
		return errors.New("cannot record note")
	}
	cmd := hostCommand(ctx, d.Path, "git", "diff", "--cached", "--quiet", "--", name)
	err = cmd.Run()
	if err == nil {
		return nil
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		return errors.New("cannot inspect history")
	}
	_, err = git("commit", "--only", "-m", "vault: "+name, "--", name)
	if err != nil {
		return errors.New("cannot record note history")
	}
	return nil
}
func openDesktop(ctx context.Context, uri string) error {
	command := ""
	switch runtime.GOOS {
	case "darwin":
		command = "open"
	case "linux":
		command = "xdg-open"
	default:
		return errors.New("desktop opening unsupported on this host")
	}
	if err := exec.CommandContext(ctx, command, uri).Run(); err != nil {
		return errors.New("host could not open Obsidian")
	}
	return nil
}
func (s *Service) openNote(ctx context.Context, d Definition, name string) (any, error) {
	values := url.Values{"vault": {d.Obsidian}, "file": {name}}
	if err := s.Open(ctx, "obsidian://open?"+values.Encode()); err != nil {
		return nil, err
	}
	return struct {
		Vault  string `json:"vault"`
		Path   string `json:"path"`
		Opened bool   `json:"opened"`
	}{d.Name, name, true}, nil
}
