//go:build darwin

package main

import (
	"syscall"
	"unsafe"
)

const unixWinSize = 0x40087468 // TIOCGWINSZ

type unixTermios struct {
	Iflag  uint64
	Oflag  uint64
	Cflag  uint64
	Lflag  uint64
	Cc     [20]uint8
	Pad    [4]byte
	Ispeed uint64
	Ospeed uint64
}

func unixMakeRaw(t *unixTermios) {
	const (
		ignbrk = 0x1
		brkint = 0x2
		parmrk = 0x8
		istrip = 0x20
		inlcr  = 0x40
		igncr  = 0x80
		icrnl  = 0x100
		ixon   = 0x200
		opost  = 0x1
		echo   = 0x8
		echonl = 0x10
		icanon = 0x100
		isig   = 0x80
		iexten = 0x400
		csize  = 0x300
		parenb = 0x1000
		cs8    = 0x300
	)
	t.Iflag &^= ignbrk | brkint | parmrk | istrip | inlcr | igncr | icrnl | ixon
	t.Oflag &^= opost
	t.Lflag &^= echo | echonl | icanon | isig | iexten
	t.Cflag &^= csize | parenb
	t.Cflag |= cs8
	t.Cc[16] = 1 // VMIN
	t.Cc[17] = 0 // VTIME
}

func getTermios(fd uintptr) (unixTermios, error) {
	var t unixTermios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(0x40487413), uintptr(unsafe.Pointer(&t))) // TIOCGETA
	if errno != 0 {
		return t, errno
	}
	return t, nil
}

func setTermios(fd uintptr, t *unixTermios) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(0x80487414), uintptr(unsafe.Pointer(t))) // TIOCSETA
	if errno != 0 {
		return errno
	}
	return nil
}
