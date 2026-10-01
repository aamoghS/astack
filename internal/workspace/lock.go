package workspace

import (
	"fmt"
	"os"
	"path/filepath"
)

const LockName = ".astack.lock"

const ExitBusy = 4

const ExitTimeout = 124

type Lock struct {
	path string
}

func AcquireLock(workdir string) (*Lock, error) {
	path := filepath.Join(workdir, LockName)
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
	return &Lock{path: path}, nil
}

func (l *Lock) Release() {
	if l == nil {
		return
	}
	_ = os.Remove(l.path)
}
