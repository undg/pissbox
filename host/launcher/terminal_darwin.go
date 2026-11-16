package launcher

import (
	"os"
	"syscall"
	"unsafe"
)

func terminalState(f *os.File) (syscall.Termios, error) {
	var state syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), syscall.TIOCGETA, uintptr(unsafe.Pointer(&state)))
	if errno != 0 {
		return state, errno
	}
	return state, nil
}
func setTerminal(f *os.File, state *syscall.Termios) {
	syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), syscall.TIOCSETA, uintptr(unsafe.Pointer(state)))
}
