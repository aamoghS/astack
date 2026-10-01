//go:build unix

package tui

import (
	"os"
	"syscall"
	"unsafe"
)

type osHost struct {
	in, out *os.File
	left    []byte
}

func NewOSHost(in, out *os.File) Host {
	return &osHost{in: in, out: out}
}

func (h *osHost) IsTTY() bool {
	return charDevice(h.in) && charDevice(h.out)
}

func (h *osHost) Write(p []byte) (int, error) { return h.out.Write(p) }

func (h *osHost) ReadKey() (Key, error) {
	return readKeyFrom(h.in, &h.left)
}

func (h *osHost) Alt(on bool) {
	if on {
		_, _ = h.out.Write([]byte("\x1b[?1049h\x1b[?25l"))
		return
	}
	_, _ = h.out.Write([]byte("\x1b[?25h\x1b[?1049l"))
}

func (h *osHost) Size() (int, int) {
	var ws struct {
		Row, Col, X, Y uint16
	}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, h.out.Fd(), uintptr(unixWinSize), uintptr(unsafe.Pointer(&ws)))
	if errno != 0 || ws.Col == 0 {
		return 80, 24
	}
	return int(ws.Col), int(ws.Row)
}

func (h *osHost) MakeRaw() (func(), error) {
	old, err := getTermios(h.in.Fd())
	if err != nil {
		return nil, err
	}
	raw := old
	unixMakeRaw(&raw)
	if err := setTermios(h.in.Fd(), &raw); err != nil {
		return nil, err
	}
	return func() { _ = setTermios(h.in.Fd(), &old) }, nil
}
