//go:build darwin || linux

package app

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

type winsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

func terminalWidth(file *os.File) (int, error) {
	ws := &winsize{}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(ws)))
	if errno != 0 {
		return 0, errno
	}
	if ws.Col == 0 {
		return 0, fmt.Errorf("terminal width unavailable")
	}
	return int(ws.Col), nil
}
