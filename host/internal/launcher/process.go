package launcher

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

// A process boundary keeps terminal descriptors intact. Only stderr is teed when
// the caller needs to recognize SBX's existing-workspace diagnostic.
type processes struct{ signals chan os.Signal }

func newProcesses() *processes {
	p := &processes{signals: make(chan os.Signal, 1)}
	signal.Notify(p.signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	return p
}
func (p *processes) close() { signal.Stop(p.signals) }
func exitStatus(err error) int {
	if err == nil {
		return 0
	}
	var exited *exec.ExitError
	if errors.As(err, &exited) {
		if s, ok := exited.Sys().(syscall.WaitStatus); ok && s.Signaled() {
			return 128 + int(s.Signal())
		}
		return exited.ExitCode()
	}
	return 1
}
func (p *processes) command(name string, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cmd := exec.Command(name, args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(stderr, err)
		if errors.Is(err, exec.ErrNotFound) || os.IsNotExist(err) {
			return 127
		}
		return 126
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	for {
		select {
		case err := <-done:
			return exitStatus(err)
		case s := <-p.signals:
			_ = cmd.Process.Signal(s)
		}
	}
}
func (p *processes) sbx(args ...string) int {
	return p.command("sbx", args, os.Stdin, os.Stdout, os.Stderr)
}
func (p *processes) capture(name string, args []string, stdin io.Reader) (string, int) {
	var output bytes.Buffer
	status := p.command(name, args, stdin, &output, os.Stderr)
	return stringsTrimNewlines(output.String()), status
}
func stringsTrimNewlines(s string) string {
	for len(s) > 0 && s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}
	return s
}
func (p *processes) initial(args []string) (string, int) {
	var diagnostic bytes.Buffer
	status := p.command("sbx", args, os.Stdin, os.Stdout, io.MultiWriter(os.Stderr, &diagnostic))
	return diagnostic.String(), status
}

// readInput never buffers ahead: menu input must not consume input intended for Pi.
func (p *processes) readInput(oneKey bool) (string, int) {
	restore := func() {}
	if oneKey {
		restore = singleKey(os.Stdin)
		defer restore()
	}
	type result struct {
		text string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		var text []byte
		b := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(b)
			if n > 0 {
				if b[0] == '\n' {
					done <- result{string(text), nil}
					return
				}
				text = append(text, b[0])
				if oneKey {
					done <- result{string(text), nil}
					return
				}
			}
			if err != nil {
				done <- result{"", err}
				return
			}
		}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			return "", 1
		}
		return r.text, 0
	case s := <-p.signals:
		return "", 128 + int(s.(syscall.Signal))
	}
}
func (p *processes) confirm(prompt string) (bool, int) {
	for {
		fmt.Fprint(os.Stderr, prompt, " ")
		answer, status := p.readInput(false)
		if status != 0 {
			fmt.Fprintln(os.Stderr)
			return false, status
		}
		switch strings.ToLower(answer) {
		case "y", "yes":
			return true, 0
		case "", "n", "no":
			return false, 0
		default:
			fmt.Fprintln(os.Stderr, "Please answer yes or no.")
		}
	}
}
