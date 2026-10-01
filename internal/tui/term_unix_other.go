//go:build unix && !linux && !darwin

package tui

import "fmt"

const unixWinSize = 0

type unixTermios struct{}

func unixMakeRaw(*unixTermios) {}

func getTermios(uintptr) (unixTermios, error) {
	return unixTermios{}, fmt.Errorf("astack tui: raw mode not supported on this unix")
}

func setTermios(uintptr, *unixTermios) error {
	return fmt.Errorf("astack tui: raw mode not supported on this unix")
}
