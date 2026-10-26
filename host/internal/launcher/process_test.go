package launcher

import (
	"bufio"
	"bytes"
	"io"
	"strings"
	"syscall"
	"testing"
	"time"
)

func awaitLine(t *testing.T, r io.Reader, needle string) {
	t.Helper()
	found := make(chan bool, 1)
	go func() {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), needle) {
				found <- true
				io.Copy(io.Discard, r)
				return
			}
		}
		found <- false
	}()
	select {
	case ok := <-found:
		if !ok {
			t.Fatalf("process closed stderr before %q", needle)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("process did not stream %q", needle)
	}
}
func TestSignalsPreserveStatusAndUnregisterWhileOwnerAlive(t *testing.T) {
	for _, signal := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP} {
		t.Run(signal.String(), func(t *testing.T) {
			f := newFixture(t)
			f.bridge()
			cmd := f.command("", map[string]string{"WAIT_RUN": "1"})
			stderr, err := cmd.StderrPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if cmd.ProcessState == nil {
					cmd.Process.Kill()
					cmd.Wait()
				}
			})
			awaitLine(t, stderr, "child-waiting")
			if err := cmd.Process.Signal(signal); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case err := <-done:
				if exitStatus(err) != 128+int(signal) {
					t.Fatalf("signal %v: %v", signal, err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("launcher did not exit after signal")
			}
			calls := f.calls()
			if kinds(calls) != "bridge-register,run,bridge-unregister" {
				t.Fatal(calls)
			}
		})
	}
}
func TestMissingCommandPreservesShellNotFoundStatus(t *testing.T) {
	p := newProcesses()
	defer p.close()
	var diagnostic bytes.Buffer
	if status := p.command("pissbox-test-command-that-does-not-exist", nil, nil, io.Discard, &diagnostic); status != 127 || diagnostic.Len() == 0 {
		t.Fatal(status, diagnostic.String())
	}
}

func TestStderrStreamsBeforeChildExit(t *testing.T) {
	f := newFixture(t)
	cmd := f.command("", map[string]string{"WAIT_RUN": "1"})
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	// Waiting for this while the child is blocked would time out if the launcher
	// buffered stderr until exit instead of streaming it through the tee.
	awaitLine(t, stderr, "child-waiting")
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
}

// Ensure the returning entrypoint owns cleanup, not a replacement exec path.
func TestBridgeUnregisterFailureDoesNotReplaceSBXStatus(t *testing.T) {
	f := newFixture(t)
	f.bridge()
	out, status, calls := f.launch("", map[string]string{"RUN_STATUS": "29", "UNREGISTER_FAIL": "1"})
	if status != 29 || kinds(calls) != "bridge-register,run,bridge-unregister" {
		t.Fatalf("%d %v %s", status, calls, out)
	}
}
