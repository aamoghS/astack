package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTreeChange(t *testing.T) {
	orig, cp := t.TempDir(), t.TempDir()
	write := func(dir, name, body string) {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0755)
		os.WriteFile(filepath.Join(dir, name), []byte(body), 0600)
	}
	write(orig, "same.txt", "x")
	write(cp, "same.txt", "x")
	write(orig, "gone.txt", "x")
	write(cp, "sub/new.txt", "fresh")
	write(orig, "node_modules/skip.js", "x")
	got := TreeChange(orig, cp)
	if !strings.Contains(got, "new file sub/new.txt") || !strings.Contains(got, "fresh") || !strings.Contains(got, "deleted gone.txt") {
		t.Fatal(got)
	}
	if strings.Contains(got, "same.txt") || strings.Contains(got, "skip.js") {
		t.Fatal(got)
	}
	if TreeChange(orig, orig) != "" {
		t.Fatal("identical trees")
	}
}
