//go:build windows

package tui

import (
	"os"
	"syscall"
	"unsafe"
)

const (
	winEnableProcessedInput       = 0x0001
	winEnableLineInput            = 0x0002
	winEnableEchoInput            = 0x0004
	winEnableProcessedOutput      = 0x0001
	winEnableVirtualTerminalProc  = 0x0004
	winEnableVirtualTerminalInput = 0x0200
	winEnableExtendedFlags        = 0x0080
)

var (
	modKernel32                    = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode             = modKernel32.NewProc("GetConsoleMode")
	procSetConsoleMode             = modKernel32.NewProc("SetConsoleMode")
	procGetConsoleScreenBufferInfo = modKernel32.NewProc("GetConsoleScreenBufferInfo")
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
	type coord struct{ X, Y int16 }
	type rect struct{ L, T, R, B int16 }
	var info struct {
		Size   coord
		Cursor coord
		Attr   uint16
		Window rect
		Max    coord
	}
	ok, _, _ := procGetConsoleScreenBufferInfo.Call(h.out.Fd(), uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		return 80, 24
	}
	w := int(info.Window.R - info.Window.L + 1)
	ht := int(info.Window.B - info.Window.T + 1)
	if w < 20 {
		w = 80
	}
	if ht < 8 {
		ht = 24
	}
	return w, ht
}

func (h *osHost) MakeRaw() (func(), error) {
	var inMode, outMode uint32
	if err := getConMode(h.in.Fd(), &inMode); err != nil {
		return nil, err
	}
	if err := getConMode(h.out.Fd(), &outMode); err != nil {
		return nil, err
	}
	rawIn := inMode
	rawIn &^= winEnableLineInput | winEnableEchoInput | winEnableProcessedInput
	rawIn |= winEnableVirtualTerminalInput | winEnableExtendedFlags
	rawOut := outMode | winEnableVirtualTerminalProc | winEnableProcessedOutput
	if err := setConMode(h.in.Fd(), rawIn); err != nil {
		return nil, err
	}
	if err := setConMode(h.out.Fd(), rawOut); err != nil {
		_ = setConMode(h.in.Fd(), inMode)
		return nil, err
	}
	return func() {
		_ = setConMode(h.in.Fd(), inMode)
		_ = setConMode(h.out.Fd(), outMode)
	}, nil
}

func getConMode(fd uintptr, mode *uint32) error {
	ok, _, err := procGetConsoleMode.Call(fd, uintptr(unsafe.Pointer(mode)))
	if ok == 0 {
		return err
	}
	return nil
}

func setConMode(fd uintptr, mode uint32) error {
	ok, _, err := procSetConsoleMode.Call(fd, uintptr(mode))
	if ok == 0 {
		return err
	}
	return nil
}
