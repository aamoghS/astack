package agents

import (
	"strings"
	"testing"

	"astack/internal/platform"
)

const roleTestConfig = `{
  "order": ["claude", "codex", "gemini"],
  "agents": {
    "claude": {"bin": ["claude"], "args": ["-p", "{{prompt}}", "--sys", "{{footer}}"], "read_args": ["-p", "{{prompt}}", "--plan"], "model_args": ["--model", "{{model}}"]},
    "codex": {"bin": ["codex"], "args": ["-p", "{{prompt}}", "--write"], "read_args": ["-p", "{{prompt}}", "--read-only"], "model_args": ["-m", "{{model}}"]},
    "gemini": {"bin": ["gemini"], "args": ["-p", "{{prompt}}", "--yolo"]}
  },
  "roles": {
    "implement": {"models": {"claude": "opus"}},
    "why": {"agents": ["claude", "codex"]},
    "review": {"agents": ["codex"]},
    "judge": {"agents": ["claude", "codex"]},
    "synth": {"agents": ["claude"]}
  }
}`

func mustRegistry(t *testing.T) *Registry {
	t.Helper()
	reg, err := ParseRegistry([]byte(roleTestConfig))
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

func TestRegistryModelPrecedence(t *testing.T) {
	reg := mustRegistry(t)
	if m := reg.Model(RoleImplement, "claude", nil); m != "opus" {
		t.Fatal(m)
	}
	if m := reg.Model(RoleImplement, "claude", ModelChoice{AnyAgent: "haiku"}); m != "haiku" {
		t.Fatal(m)
	}
	if m := reg.Model(RoleImplement, "claude", ModelChoice{"claude": "sonnet", AnyAgent: "haiku"}); m != "sonnet" {
		t.Fatal(m)
	}
	if m := reg.Model(RoleWhy, "codex", nil); m != "" {
		t.Fatal(m)
	}
}

func TestRoleUnknownAgentRejected(t *testing.T) {
	_, err := ParseRegistry([]byte(`{"order":["a"],"agents":{"a":{"bin":["a"],"args":["x"]}},"roles":{"why":{"agents":["nope"]}}}`))
	if err == nil {
		t.Fatal("want error")
	}
}

func TestAgentModelAndReadArgs(t *testing.T) {
	reg := mustRegistry(t)
	a, _ := reg.Get("claude")
	plat := platform.NewPlatform("linux", platform.HostPaths{})
	got := strings.Join(a.CommandLine(map[string]string{"prompt": "P", "footer": "F", "model": "opus"}, plat), " ")
	if got != "-p P --sys F --model opus" {
		t.Fatal(got)
	}
	got = strings.Join(a.CommandLine(map[string]string{"prompt": "P", "footer": "F"}, plat), " ")
	if got != "-p P --sys F" {
		t.Fatal(got)
	}
	got = strings.Join(a.ReadCommandLine(map[string]string{"prompt": "P"}, plat), " ")
	if got != "-p P --plan" {
		t.Fatal(got)
	}
	g, _ := reg.Get("gemini")
	if g.CanReadOnly() || !a.CanReadOnly() {
		t.Fatal("CanReadOnly")
	}
}

func TestRegistryMembers(t *testing.T) {
	reg := mustRegistry(t)
	all := map[string]string{"claude": "/c", "codex": "/x", "gemini": "/g"}
	m, p := reg.Members(RoleWhy, nil, all)
	if p.Code != 0 || len(m) != 2 {
		t.Fatal(m, p)
	}
	m, _ = reg.Members(RoleWhy, nil, map[string]string{"codex": "/x"})
	if len(m) != 1 || m[0].ID != "codex" {
		t.Fatal(m)
	}
	m, _ = reg.Members(RoleExplore, nil, map[string]string{"gemini": "/g"})
	if len(m) != 1 || m[0].ID != "gemini" {
		t.Fatal("fallback to first installed", m)
	}
	if _, p = reg.Members(RoleWhy, nil, map[string]string{}); p.Code != 3 {
		t.Fatal(p)
	}
	if _, p = reg.Members(RoleWhy, []string{"gemini"}, map[string]string{"claude": "/c"}); p.Code != 2 {
		t.Fatal(p)
	}
}
