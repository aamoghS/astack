package workspace

import (
	"testing"
)

func TestWorkdirLock(t *testing.T) {
	dir := t.TempDir()
	a, err := AcquireLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireLock(dir); err == nil {
		t.Fatal("second lock should fail")
	}
	a.Release()
	b, err := AcquireLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	b.Release()
}
