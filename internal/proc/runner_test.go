package proc

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"astack/internal/platform"
)

func TestPromptStore(t *testing.T) {
	dir := t.TempDir()
	p, err := PromptStore{Pid: 42}.Write(dir, "hello")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "hello" {
		t.Fatalf("got %q", b)
	}
	if filepath.Dir(p) != dir {
		t.Fatalf("prompt file escaped workdir: %s", p)
	}
	if !strings.Contains(p, ".astack-prompt.42.txt") {
		t.Fatalf("pid not in name: %s", p)
	}
}

func TestProcessRunnerWindowsCmd(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("cmd.exe wrapping is resolved on windows")
	}
	r := &ProcessRunner{Platform: platform.NewPlatform("windows", platform.HostPaths{}), Environ: nil}
	cmd := r.Command(`C:\npm\claude.cmd`, []string{"-p", "x"}, `C:\repo`)
	if !strings.Contains(strings.ToLower(cmd.Path), "cmd") {
		t.Fatalf("path=%q args=%v", cmd.Path, cmd.Args)
	}
	if !strings.Contains(strings.Join(cmd.Args, " "), `C:\npm\claude.cmd`) {
		t.Fatalf("args=%v", cmd.Args)
	}
}
