//go:build linux

package tui

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

type terminal struct {
	original syscall.Termios
	fd       uintptr
	enabled  bool
}

func ioctl(fd uintptr, request uintptr, arg unsafe.Pointer) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, request, uintptr(arg))
	if errno != 0 {
		return errno
	}
	return nil
}
func (t *terminal) rawMode() error {
	t.fd = os.Stdin.Fd()
	var old syscall.Termios
	if err := ioctl(t.fd, syscall.TCGETS, unsafe.Pointer(&old)); err != nil {
		return fmt.Errorf("interactive input needs a TTY: %w", err)
	}
	t.original = old
	r := old
	r.Iflag &^= syscall.BRKINT | syscall.ICRNL | syscall.INPCK | syscall.ISTRIP | syscall.IXON
	r.Oflag &^= syscall.OPOST
	r.Cflag |= syscall.CS8
	r.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.IEXTEN | syscall.ISIG
	r.Cc[syscall.VMIN] = 1
	r.Cc[syscall.VTIME] = 0
	if err := ioctl(t.fd, syscall.TCSETS, unsafe.Pointer(&r)); err != nil {
		return err
	}
	t.enabled = true
	fmt.Print("\x1b[?1049h\x1b[?25l\x1b[2J\x1b[H")
	return nil
}
func (t *terminal) restore() {
	if !t.enabled {
		return
	}
	_ = ioctl(t.fd, syscall.TCSETS, unsafe.Pointer(&t.original))
	fmt.Print("\x1b[?25h\x1b[?1049l")
	t.enabled = false
}
func terminalSize() (int, int) {
	var w struct{ Rows, Cols, Xpixel, Ypixel uint16 }
	if err := ioctl(os.Stdout.Fd(), syscall.TIOCGWINSZ, unsafe.Pointer(&w)); err != nil || w.Cols == 0 {
		return 100, 28
	}
	return int(w.Cols), int(w.Rows)
}
func isTerminal(fd uintptr) bool {
	var term syscall.Termios
	return ioctl(fd, syscall.TCGETS, unsafe.Pointer(&term)) == nil
}
