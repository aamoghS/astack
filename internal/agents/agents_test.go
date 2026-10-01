package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"astack/internal/platform"
)

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
	if ArgvBytes([]string{"a", "bb"}) != 5 {
		t.Fatalf("argvBytes=%d", ArgvBytes([]string{"a", "bb"}))
	}

	win := platform.NewPlatform("windows", platform.HostPaths{})
	mac := platform.NewPlatform("darwin", platform.HostPaths{})
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
	reg := &Registry{
		Order: []string{"claude", "gemini"},
		ByID: map[string]Agent{
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

func TestIsEphemeralExe(t *testing.T) {
	if !IsEphemeralExe(`/tmp/go-build123/exe`) {
		t.Fatal("expected ephemeral")
	}
	if IsEphemeralExe(`/usr/local/bin/astack`) {
		t.Fatal("installed binary is not ephemeral")
	}
}

type mapLooker struct {
	path  map[string]string
	files map[string]bool
}

func (m mapLooker) LookPath(name string) string { return m.path[name] }

func (m mapLooker) IsFile(path string) bool { return m.files[path] }

func TestBinaryResolver(t *testing.T) {
	plat := platform.NewPlatform("linux", platform.HostPaths{Home: "/home/dev"})
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
