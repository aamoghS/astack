package platform

import (
	"strings"
	"testing"
)

func TestPlatformsAllOS(t *testing.T) {
	darwin := NewPlatform("darwin", HostPaths{Home: "/home/dev"})
	bins := darwin.ExtraBins([]string{"claude"})
	if !contains(bins, "/opt/homebrew/bin/claude") || !contains(bins, "/home/dev/.local/bin/claude") {
		t.Fatalf("darwin bins=%v", bins)
	}
	if darwin.Name() != "darwin" || darwin.MaxArgBytes() != 100000 {
		t.Fatalf("darwin caps: %s %d", darwin.Name(), darwin.MaxArgBytes())
	}

	linux := NewPlatform("linux", HostPaths{Home: "/home/dev"})
	bins = linux.ExtraBins([]string{"claude"})
	if !contains(bins, "/usr/local/bin/claude") || !contains(bins, "/home/dev/.local/bin/claude") {
		t.Fatalf("linux bins=%v", bins)
	}

	win := NewPlatform("windows", HostPaths{
		Home:     `C:\Users\dev`,
		LocalApp: `C:\Users\dev\AppData\Local`,
		AppData:  `C:\Users\dev\AppData\Roaming`,
	})
	bins = win.ExtraBins([]string{"claude"})
	if !contains(bins, `C:\Users\dev\.local\bin\claude.exe`) || !contains(bins, `C:\Users\dev\AppData\Roaming\npm\claude.cmd`) {
		t.Fatalf("windows bins=%v", bins)
	}
	if contains(bins, "/opt/homebrew/bin/claude") {
		t.Fatal("windows must not use unix homebrew paths")
	}
	if win.MaxArgBytes() >= linux.MaxArgBytes() {
		t.Fatal("windows argv cap should be tighter than unix")
	}
}

func TestAugmentEnv(t *testing.T) {
	mac := NewPlatform("darwin", HostPaths{Home: "/Users/dev"})
	got := mac.AugmentEnv([]string{"FOO=1", "PATH=/usr/bin"})
	var path string
	for _, e := range got {
		if strings.HasPrefix(e, "PATH=") {
			path = e
		}
	}
	if !strings.Contains(path, "/opt/homebrew/bin") || !strings.Contains(path, "/Users/dev/.local/bin") {
		t.Fatalf("darwin PATH=%s", path)
	}

	win := NewPlatform("windows", HostPaths{
		Home:     `C:\Users\dev`,
		LocalApp: `C:\Users\dev\AppData\Local`,
		AppData:  `C:\Users\dev\AppData\Roaming`,
	})
	got = win.AugmentEnv([]string{`Path=C:\Windows`})
	found := false
	for _, e := range got {
		if strings.HasPrefix(strings.ToLower(e), "path=") && strings.Contains(e, `AppData\Roaming\npm`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("windows PATH not augmented: %v", got)
	}
}

func TestWindowsWrapCmdShim(t *testing.T) {
	win := NewPlatform("windows", HostPaths{})
	exe, argv := win.Wrap(`C:\npm\claude.cmd`, []string{"-p", "x"})
	if exe != "cmd.exe" || argv[0] != "/c" || argv[1] != `C:\npm\claude.cmd` {
		t.Fatalf("wrap=%s %v", exe, argv)
	}
	exe, argv = win.Wrap(`C:\bin\claude.exe`, []string{"-p", "x"})
	if exe != `C:\bin\claude.exe` {
		t.Fatalf("exe wrap=%s %v", exe, argv)
	}
	mac := NewPlatform("darwin", HostPaths{})
	exe, argv = mac.Wrap("/opt/homebrew/bin/claude", []string{"-p", "x"})
	if exe != "/opt/homebrew/bin/claude" {
		t.Fatalf("mac wrap=%s %v", exe, argv)
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
