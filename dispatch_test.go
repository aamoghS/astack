package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type recordRunner struct {
	mu    sync.Mutex
	bin   string
	args  []string
	cwd   string
	code  int
	calls int
}

func (r *recordRunner) Run(bin string, args []string, cwd string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.bin = bin
	r.args = append([]string(nil), args...)
	r.cwd = cwd
	return r.code
}

func testLoader(t *testing.T, embed []byte) *ConfigLoader {
	t.Helper()
	return &ConfigLoader{
		Embed: embed,
		Env:   func(string) string { return "" },
		Cwd:   func() (string, error) { return t.TempDir(), nil },
		Exe:   func() (string, error) { return "/tmp/go-build/exe", nil },
		Read:  os.ReadFile,
	}
}

func TestDispatcherListHelpAndExits(t *testing.T) {
	plat := NewPlatform("linux", HostPaths{Home: "/home/dev"})
	look := mapLooker{path: map[string]string{"claude": "/usr/bin/claude"}, files: map[string]bool{}}
	var out, errb bytes.Buffer
	d := &Dispatcher{
		Out:      &out,
		Err:      &errb,
		Platform: plat,
		Loader:   testLoader(t, embeddedAgents),
		Resolver: BinaryResolver{Platform: plat, Looker: look},
		Runner:   &recordRunner{code: 7},
		Prompts:  PromptStore{Pid: 99},
	}
	if code := d.Main([]string{"--help"}); code != 0 || !strings.Contains(out.String(), "usage:") {
		t.Fatalf("help code=%d out=%s", code, out.String())
	}
	out.Reset()
	if code := d.Main([]string{"--list"}); code != 0 {
		t.Fatalf("list code=%d err=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "claude\tinstalled") || !strings.Contains(out.String(), "gemini\tabsent") {
		t.Fatalf("list out=%s", out.String())
	}
	if code := d.Main([]string{"--workdir", t.TempDir(), "--prompt-file", writePrompt(t), "--agent", "missing"}); code != 2 {
		t.Fatalf("unknown agent code=%d err=%s", code, errb.String())
	}
	empty := &Dispatcher{
		Out:      &out,
		Err:      &errb,
		Platform: plat,
		Loader:   testLoader(t, embeddedAgents),
		Resolver: BinaryResolver{Platform: plat, Looker: mapLooker{path: map[string]string{}, files: map[string]bool{}}},
		Runner:   &recordRunner{},
		Prompts:  PromptStore{Pid: 1},
	}
	if code := empty.Main([]string{"--workdir", t.TempDir(), "--prompt-file", writePrompt(t)}); code != 3 {
		t.Fatalf("no worker code=%d err=%s", code, errb.String())
	}
}

func TestDispatcherRunsAndDeletesPrompt(t *testing.T) {
	dir := t.TempDir()
	plat := NewPlatform("linux", HostPaths{Home: "/home/dev"})
	run := &recordRunner{code: 0}
	d := &Dispatcher{
		Out:      &bytes.Buffer{},
		Err:      &bytes.Buffer{},
		Platform: plat,
		Loader:   testLoader(t, embeddedAgents),
		Resolver: BinaryResolver{Platform: plat, Looker: mapLooker{path: map[string]string{"claude": "/usr/bin/claude"}, files: map[string]bool{}}},
		Runner:   run,
		Prompts:  PromptStore{Pid: 7},
	}
	code := d.Main([]string{"--workdir", dir, "--prompt-file", writePrompt(t), "--agent", "claude"})
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	abs, _ := filepath.Abs(dir)
	if run.calls != 1 || run.bin != "/usr/bin/claude" || run.cwd != abs {
		t.Fatalf("run=%+v want bin=/usr/bin/claude cwd=%s", run, abs)
	}
	if !strings.Contains(strings.Join(run.args, " "), "implement this") {
		t.Fatalf("args=%v", run.args)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, ".astack-prompt.*"))
	if len(matches) != 0 {
		t.Fatalf("prompt file leaked: %v", matches)
	}
}

func TestDispatcherWindowsHugePromptUsesFile(t *testing.T) {
	dir := t.TempDir()
	plat := NewPlatform("windows", HostPaths{Home: `C:\Users\dev`})
	run := &recordRunner{}
	d := &Dispatcher{
		Out:      &bytes.Buffer{},
		Err:      &bytes.Buffer{},
		Platform: plat,
		Loader:   testLoader(t, embeddedAgents),
		Resolver: BinaryResolver{Platform: plat, Looker: mapLooker{path: map[string]string{"claude": `C:\bin\claude.exe`}, files: map[string]bool{}}},
		Runner:   run,
		Prompts:  PromptStore{Pid: 3},
	}
	p := filepath.Join(t.TempDir(), "p.txt")
	if err := os.WriteFile(p, []byte(strings.Repeat("x", 8000)), 0600); err != nil {
		t.Fatal(err)
	}
	if code := d.Main([]string{"--workdir", dir, "--prompt-file", p, "--agent", "claude"}); code != 0 {
		t.Fatalf("code=%d", code)
	}
	joined := strings.Join(run.args, " ")
	if strings.Contains(joined, strings.Repeat("x", 8000)) {
		t.Fatal("huge prompt still on argv")
	}
	if !strings.Contains(joined, ".astack-prompt.3.txt") {
		t.Fatalf("expected prompt file in args: %v", run.args)
	}
}

func TestDispatcherConfigDefault(t *testing.T) {
	embed := []byte(`{
		"default": "claude",
		"order": ["gemini", "claude"],
		"agents": {
			"gemini": {"bin": ["gemini"], "args": ["-p", "{{prompt}}"]},
			"claude": {"bin": ["claude"], "args": ["-p", "{{prompt}}"]}
		}
	}`)
	run := &recordRunner{}
	plat := NewPlatform("linux", HostPaths{})
	d := &Dispatcher{
		Out:      &bytes.Buffer{},
		Err:      &bytes.Buffer{},
		Platform: plat,
		Loader:   testLoader(t, embed),
		Resolver: BinaryResolver{Platform: plat, Looker: mapLooker{path: map[string]string{"gemini": "/g", "claude": "/c"}, files: map[string]bool{}}},
		Runner:   run,
		Prompts:  PromptStore{Pid: 1},
	}
	if code := d.Main([]string{"--workdir", t.TempDir(), "--prompt-file", writePrompt(t)}); code != 0 {
		t.Fatalf("code=%d", code)
	}
	if run.bin != "/c" {
		t.Fatalf("config default should pick claude, got bin=%s", run.bin)
	}
}

func TestAgentAliasCallsWorker(t *testing.T) {
	run := &recordRunner{}
	plat := NewPlatform("linux", HostPaths{})
	d := &Dispatcher{
		Out:      &bytes.Buffer{},
		Err:      &bytes.Buffer{},
		Platform: plat,
		Loader:   testLoader(t, embeddedAgents),
		Resolver: BinaryResolver{Platform: plat, Looker: mapLooker{path: map[string]string{"claude": "/usr/bin/claude"}, files: map[string]bool{}}},
		Runner:   run,
		Prompts:  PromptStore{Pid: 1},
	}
	code := d.Main([]string{"agent", "--workdir", t.TempDir(), "--prompt-file", writePrompt(t)})
	if code != 0 || run.bin != "/usr/bin/claude" {
		t.Fatalf("agent alias code=%d bin=%s", code, run.bin)
	}
}

func TestLiveStubWorker(t *testing.T) {
	dir := t.TempDir()
	var stub string
	if runtime.GOOS == "windows" {
		stub = filepath.Join(dir, "worker.cmd")
		if err := os.WriteFile(stub, []byte("@echo off\r\necho ran>out.txt\r\n"), 0644); err != nil {
			t.Fatal(err)
		}
	} else {
		stub = filepath.Join(dir, "worker")
		if err := os.WriteFile(stub, []byte("#!/bin/sh\necho ran > out.txt\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	embed := []byte(`{"order":["stub"],"agents":{"stub":{"bin":["worker"],"args":["{{prompt}}"]}}}`)
	plat := HostPlatform()
	d := &Dispatcher{
		Out:      &bytes.Buffer{},
		Err:      &bytes.Buffer{},
		Platform: plat,
		Loader:   testLoader(t, embed),
		Resolver: BinaryResolver{Platform: plat, Looker: mapLooker{path: map[string]string{"worker": stub}, files: map[string]bool{}}},
		Runner:   NewProcessRunner(plat),
		Prompts:  PromptStore{Pid: os.Getpid()},
	}
	if code := d.Main([]string{"--workdir", dir, "--prompt-file", writePrompt(t), "--agent", "stub"}); code != 0 {
		t.Fatalf("stub worker code=%d", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "out.txt")); err != nil {
		t.Fatalf("stub did not run in workdir: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, ".astack-prompt.*"))
	if len(matches) != 0 {
		t.Fatalf("prompt leaked: %v", matches)
	}
}

func TestListOverheadVsPstack(t *testing.T) {
	plat := NewPlatform("linux", HostPaths{Home: "/home/dev"})
	d := &Dispatcher{
		Out:      &bytes.Buffer{},
		Err:      &bytes.Buffer{},
		Platform: plat,
		Loader:   testLoader(t, embeddedAgents),
		Resolver: BinaryResolver{Platform: plat, Looker: mapLooker{path: map[string]string{"claude": "/c"}, files: map[string]bool{}}},
		Runner:   &recordRunner{},
		Prompts:  PromptStore{Pid: 1},
	}
	start := time.Now()
	const n = 200
	for i := 0; i < n; i++ {
		if code := d.Main([]string{"--list"}); code != 0 {
			t.Fatalf("list code=%d", code)
		}
	}
	avg := time.Since(start) / n
	// pstack Task fan-out is a model round-trip (seconds). Dispatch must stay in the millisecond range.
	if avg > 50*time.Millisecond {
		t.Fatalf("list avg %s; too slow to beat pstack Task spawn", avg)
	}
	t.Logf("list avg %s", avg)
}

func TestPlaybookSwarmArenaHopBeatsTask(t *testing.T) {
	dir := t.TempDir()
	look := map[string]string{"claude": "/c", "gemini": "/g"}
	d := &Dispatcher{
		Out:      &bytes.Buffer{},
		Err:      &bytes.Buffer{},
		Platform: NewPlatform("linux", HostPaths{}),
		Loader:   testLoader(t, embeddedAgents),
		Resolver: BinaryResolver{Platform: NewPlatform("linux", HostPaths{}), Looker: mapLooker{path: look, files: map[string]bool{}}},
		Runner:   &recordRunner{},
		Prompts:  PromptStore{Pid: 1},
	}
	prompt := writePrompt(t)
	runs := [][]string{
		{"playbook", "--playbook", "feature", "--workdir", dir, "--prompt-file", prompt, "--dry-run"},
		{"swarm", "--n", "3", "--workdir", dir, "--prompt-file", prompt, "--dry-run"},
		{"arena", "--agents", "claude,gemini", "--workdir", dir, "--prompt-file", prompt, "--dry-run"},
	}
	for _, argv := range runs {
		start := time.Now()
		const n = 20
		for i := 0; i < n; i++ {
			if code := d.Main(argv); code != 0 {
				t.Fatalf("%v code=%d", argv[0], code)
			}
		}
		avg := time.Since(start) / n
		if avg > 250*time.Millisecond {
			t.Fatalf("%s avg %s; pstack would spawn Cursor Tasks (seconds)", argv[0], avg)
		}
		t.Logf("%s dry-run avg %s", argv[0], avg)
	}
}

func writePrompt(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "prompt.txt")
	if err := os.WriteFile(p, []byte("implement this"), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}
