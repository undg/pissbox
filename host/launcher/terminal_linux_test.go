package launcher

import (
	"bytes"
	"os"
	"strconv"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func openPTY(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	var unlock int32
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock)))
	if errno != 0 {
		master.Close()
		t.Fatal(errno)
	}
	var number uint32
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&number)))
	if errno != 0 {
		master.Close()
		t.Fatal(errno)
	}
	slave, err := os.OpenFile("/dev/pts/"+strconv.Itoa(int(number)), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { master.Close(); slave.Close() })
	return master, slave
}
func TestMenuRestoresTerminalAfterChoiceAndSignals(t *testing.T) {
	for _, mode := range []string{"choice", "interrupt", "terminate"} {
		t.Run(mode, func(t *testing.T) {
			f := newFixture(t)
			master, slave := openPTY(t)
			before, err := terminalState(slave)
			if err != nil {
				t.Fatal(err)
			}
			cmd := f.command("", map[string]string{"CONFLICT": "1", "REQUIRE_TTY": "1"})
			cmd.Stdin = slave
			cmd.Stdout = slave
			var diagnostic bytes.Buffer
			cmd.Stderr = &diagnostic
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if cmd.ProcessState == nil {
					cmd.Process.Kill()
					cmd.Wait()
				}
			})
			deadline := time.Now().Add(5 * time.Second)
			for {
				state, err := terminalState(slave)
				if err != nil {
					t.Fatal(err)
				}
				if state.Lflag&syscall.ICANON == 0 && state.Lflag&syscall.ECHO == 0 {
					break
				}
				if time.Now().After(deadline) {
					cmd.Process.Kill()
					cmd.Wait()
					t.Fatalf("menu did not enter single-key mode: %s", diagnostic.String())
				}
				time.Sleep(5 * time.Millisecond)
			}
			want := 0
			switch mode {
			case "choice":
				master.Write([]byte("p"))
			case "interrupt":
				cmd.Process.Signal(syscall.SIGINT)
				want = 130
			case "terminate":
				cmd.Process.Signal(syscall.SIGTERM)
				want = 143
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case err := <-done:
				if exitStatus(err) != want {
					t.Fatalf("status %d: %s", exitStatus(err), diagnostic.String())
				}
			case <-time.After(5 * time.Second):
				t.Fatal("menu did not finish")
			}
			after, err := terminalState(slave)
			if err != nil || after != before {
				t.Fatalf("terminal not restored: before=%+v after=%+v error=%v", before, after, err)
			}
			if mode == "choice" && kinds(f.calls()) != "run,exec,run" {
				t.Fatal(f.calls())
			}
		})
	}
}
func TestTerminalOutputIdentityOnOrdinaryLaunch(t *testing.T) {
	f := newFixture(t)
	_, slave := openPTY(t)
	cmd := f.command("", map[string]string{"REQUIRE_TTY": "1"})
	cmd.Stdin = slave
	cmd.Stdout = slave
	var diagnostic bytes.Buffer
	cmd.Stderr = &diagnostic
	if err := cmd.Run(); err != nil {
		t.Fatalf("%v: %s", err, diagnostic.String())
	}
}
