package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const lockName = ".astack.lock"

const exitBusy = 4
const exitTimeout = 124

type workdirLock struct {
	path string
}

func acquireWorkdirLock(workdir string) (*workdirLock, error) {
	path := filepath.Join(workdir, lockName)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("astack: another writer holds %s; not rerouting", path)
		}
		return nil, err
	}
	if _, err := fmt.Fprintf(f, "%d\n", os.Getpid()); err != nil {
		f.Close()
		os.Remove(path)
		return nil, err
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return nil, err
	}
	return &workdirLock{path: path}, nil
}

func (l *workdirLock) Release() {
	if l == nil {
		return
	}
	_ = os.Remove(l.path)
}
