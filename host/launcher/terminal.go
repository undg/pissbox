package launcher

import (
	"os"
	"syscall"
)

func singleKey(f *os.File) func() {
	old, err := terminalState(f)
	if err != nil {
		return func() {}
	}
	state := old
	state.Lflag &^= syscall.ICANON | syscall.ECHO
	state.Cc[syscall.VMIN] = 1
	state.Cc[syscall.VTIME] = 0
	setTerminal(f, &state)
	return func() { setTerminal(f, &old) }
}
func isTerminal(f *os.File) bool { _, err := terminalState(f); return err == nil }
