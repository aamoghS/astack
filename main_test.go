package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestParseArgs(t *testing.T) {
	o, err := parseArgs([]string{"--workdir", "/repo", "--prompt-file", "p.txt", "--agent", "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if o.Workdir != "/repo" || o.PromptFile != "p.txt" || o.Agent != "claude" {
		t.Fatalf("got %+v", o)
	}
	o, err = parseArgs([]string{"--list"})
	if err != nil || !o.List {
		t.Fatalf("list: %+v %v", o, err)
	}
	o, err = parseArgs([]string{"--help"})
	if err != nil || !o.Help {
		t.Fatalf("help: %+v %v", o, err)
	}
	if _, err := parseArgs([]string{"--workdir"}); err == nil {
		t.Fatal("expected missing value error")
	}
	if _, err := parseArgs([]string{"--nope"}); err == nil {
		t.Fatal("expected unknown flag")
	}
	o, err = parseArgs([]string{"--workdir", "x", "--prompt-file", "y"})
	if err != nil || o.Agent != "" {
		t.Fatalf("omitted --agent should leave Agent empty for config default: %+v %v", o, err)
	}
	o, err = parseArgs([]string{"--bench"})
	if err != nil || !o.Bench {
		t.Fatalf("bench: %+v %v", o, err)
	}
	o, err = parseArgs([]string{"--json", "--dry-run", "--timeout", "5s"})
	if err != nil || !o.JSON || !o.DryRun || o.Timeout != 5*time.Second {
		t.Fatalf("flags: %+v %v", o, err)
	}
	if _, err := parseArgs([]string{"--timeout", "nope"}); err == nil {
		t.Fatal("expected bad timeout")
	}
}

func TestConfigLoaderAgentsEnvError(t *testing.T) {
	c := &ConfigLoader{
		Embed: embeddedAgents,
		Env: func(k string) string {
			if k == "ASTACK_AGENTS" {
				return filepath.Join(t.TempDir(), "missing.json")
			}
			return ""
		},
		Read: os.ReadFile,
	}
	_, src, err := c.Load()
	if err == nil || !strings.Contains(err.Error(), "ASTACK_AGENTS") {
		t.Fatalf("src=%s err=%v", src, err)
	}
}

func TestParseRegistry(t *testing.T) {
	reg, err := ParseRegistry(embeddedAgents)
	if err != nil {
		t.Fatal(err)
	}
	a, ok := reg.Get("claude")
	if !ok || a.Args[0] != "-p" {
		t.Fatalf("unexpected registry: %+v", reg)
	}
	for id, spec := range reg.All() {
		if !strings.Contains(strings.Join(spec.Args, " "), "{{prompt}}") {
			t.Fatalf("%s args must include {{prompt}}", id)
		}
	}
	if _, err := ParseRegistry([]byte(`{"order":["nope"],"agents":{}}`)); err == nil {
		t.Fatal("expected unknown order agent")
	}
	if _, err := ParseRegistry([]byte(`{"agents":{"x":{"bin":["x"]}}}`)); err == nil {
		t.Fatal("expected missing args")
	}
}

func TestAgentCommandLine(t *testing.T) {
	a := Agent{ID: "claude", Args: []string{"-p", "{{prompt}}", "--add-dir", "{{workdir}}"}}
	got := a.Expand(map[string]string{"prompt": "hi", "workdir": "/tmp/app"})
	if strings.Join(got, " ") != "-p hi --add-dir /tmp/app" {
		t.Fatalf("got %v", got)
	}
	if argvBytes([]string{"a", "bb"}) != 5 {
		t.Fatalf("argvBytes=%d", argvBytes([]string{"a", "bb"}))
	}

	win := NewPlatform("windows", HostPaths{})
	mac := NewPlatform("darwin", HostPaths{})
	small := a.CommandLine(map[string]string{"prompt": "short", "prompt_file": "/tmp/p.txt", "workdir": "."}, win)
	if small[1] != "short" {
		t.Fatalf("small prompt should stay inline: %v", small)
	}
	big := strings.Repeat("x", 8000)
	got = a.CommandLine(map[string]string{"prompt": big, "prompt_file": `C:\repo\.astack-prompt.1.txt`, "workdir": "."}, win)
	if strings.Contains(got[1], big) {
		t.Fatal("windows must not put a huge prompt on argv")
	}
	if !strings.Contains(got[1], `C:\repo\.astack-prompt.1.txt`) {
		t.Fatalf("fallback should point at prompt file: %q", got[1])
	}
	unix := a.CommandLine(map[string]string{"prompt": strings.Repeat("y", 8000), "prompt_file": "/tmp/p.txt", "workdir": "."}, mac)
	if unix[1] != strings.Repeat("y", 8000) {
		t.Fatal("8k prompt should stay inline on macOS/Linux")
	}
}

func TestRegistryPick(t *testing.T) {
	reg := &AgentRegistry{
		Order: []string{"claude", "gemini"},
		byID: map[string]Agent{
			"claude": {ID: "claude"},
			"gemini": {ID: "gemini"},
		},
	}
	resolved := map[string]string{"claude": "", "gemini": "/usr/bin/gemini"}

	p := reg.Pick("auto", resolved)
	if p.Agent.ID != "gemini" || p.Code != 0 {
		t.Fatalf("auto pick=%s code=%d", p.Agent.ID, p.Code)
	}
	p = reg.Pick("auto", map[string]string{"claude": "", "gemini": ""})
	if p.Code != 3 {
		t.Fatalf("expected exit 3, got %d", p.Code)
	}
	p = reg.Pick("codex", resolved)
	if p.Code != 2 {
		t.Fatalf("unknown named agent: %d", p.Code)
	}
	p = reg.Pick("claude", resolved)
	if p.Code != 2 {
		t.Fatalf("named missing: %d", p.Code)
	}
	p = reg.Pick("gemini", resolved)
	if p.Agent.ID != "gemini" || p.Code != 0 {
		t.Fatalf("named hit=%s code=%d", p.Agent.ID, p.Code)
	}
}

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

func TestIsEphemeralExe(t *testing.T) {
	if !isEphemeralExe(`/tmp/go-build123/exe`) {
		t.Fatal("expected ephemeral")
	}
	if isEphemeralExe(`/usr/local/bin/astack`) {
		t.Fatal("installed binary is not ephemeral")
	}
}

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

func TestProcessRunnerWindowsCmd(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("cmd.exe wrapping is resolved on windows")
	}
	r := &ProcessRunner{Platform: NewPlatform("windows", HostPaths{}), Environ: nil}
	cmd := r.Command(`C:\npm\claude.cmd`, []string{"-p", "x"}, `C:\repo`)
	if !strings.Contains(strings.ToLower(cmd.Path), "cmd") {
		t.Fatalf("path=%q args=%v", cmd.Path, cmd.Args)
	}
	if !strings.Contains(strings.Join(cmd.Args, " "), `C:\npm\claude.cmd`) {
		t.Fatalf("args=%v", cmd.Args)
	}
}

type mapLooker struct {
	path  map[string]string
	files map[string]bool
}

func (m mapLooker) LookPath(name string) string { return m.path[name] }
func (m mapLooker) IsFile(path string) bool     { return m.files[path] }

func TestBinaryResolver(t *testing.T) {
	plat := NewPlatform("linux", HostPaths{Home: "/home/dev"})
	r := BinaryResolver{Platform: plat, Looker: mapLooker{
		path:  map[string]string{"claude": "/usr/bin/claude"},
		files: map[string]bool{},
	}}
	if got := r.Resolve(Agent{Bins: []string{"claude"}}); got != "/usr/bin/claude" {
		t.Fatalf("path resolve=%s", got)
	}

	r = BinaryResolver{Platform: plat, Looker: mapLooker{
		path:  map[string]string{},
		files: map[string]bool{"/home/dev/.local/bin/claude": true},
	}}
	if got := r.Resolve(Agent{Bins: []string{"claude"}}); got != "/home/dev/.local/bin/claude" {
		t.Fatalf("extra resolve=%s", got)
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
