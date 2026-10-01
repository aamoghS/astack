package app

import (
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

type mapLooker struct {
	path  map[string]string
	files map[string]bool
}

func (m mapLooker) LookPath(name string) string { return m.path[name] }

func (m mapLooker) IsFile(path string) bool { return m.files[path] }

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
