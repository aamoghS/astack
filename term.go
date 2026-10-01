package main

import (
	"io"
	"os"
	"unicode/utf8"
)

func charDevice(f *os.File) bool {
	if f == nil {
		return false
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

func decodeKey(buf []byte) (k tuiKey, n int, more bool) {
	if len(buf) == 0 {
		return tuiKey{}, 0, true
	}
	b := buf[0]
	switch b {
	case 3:
		return tuiKey{Name: "ctrl-c"}, 1, false
	case 4:
		return tuiKey{Name: "ctrl-d"}, 1, false
	case 9:
		return tuiKey{Name: "tab"}, 1, false
	case 13, 10:
		return tuiKey{Name: "enter"}, 1, false
	case 127, 8:
		return tuiKey{Name: "backspace"}, 1, false
	case 27:
		return decodeCSI(buf)
	}
	if b < 32 {
		return tuiKey{Name: "ctrl"}, 1, false
	}
	r, size := utf8.DecodeRune(buf)
	if r == utf8.RuneError && size == 1 {
		if !utf8.FullRune(buf) {
			return tuiKey{}, 0, true
		}
		return tuiKey{Name: "rune", Rune: r}, 1, false
	}
	return tuiKey{Name: "rune", Rune: r}, size, false
}

func decodeCSI(buf []byte) (tuiKey, int, bool) {
	if len(buf) == 1 {
		return tuiKey{}, 0, true
	}
	if buf[1] == '[' {
		if len(buf) < 3 {
			return tuiKey{}, 0, true
		}
		switch buf[2] {
		case 'A':
			return tuiKey{Name: "up"}, 3, false
		case 'B':
			return tuiKey{Name: "down"}, 3, false
		case 'C':
			return tuiKey{Name: "right"}, 3, false
		case 'D':
			return tuiKey{Name: "left"}, 3, false
		case '3':
			if len(buf) < 4 {
				return tuiKey{}, 0, true
			}
			if buf[3] == '~' {
				return tuiKey{Name: "delete"}, 4, false
			}
		}
		return tuiKey{Name: "esc"}, 3, false
	}
	return tuiKey{Name: "esc"}, 1, false
}

func readKeyFrom(r io.Reader, leftover *[]byte) (tuiKey, error) {
	for {
		k, n, more := decodeKey(*leftover)
		if !more && n > 0 {
			*leftover = (*leftover)[n:]
			return k, nil
		}
		var tmp [8]byte
		got, err := r.Read(tmp[:])
		if got > 0 {
			*leftover = append(*leftover, tmp[:got]...)
			continue
		}
		if err != nil {
			if more && len(*leftover) > 0 && (*leftover)[0] == 27 {
				*leftover = (*leftover)[1:]
				return tuiKey{Name: "esc"}, nil
			}
			return tuiKey{}, err
		}
	}
}
