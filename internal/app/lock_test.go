package app

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"astack/internal/agents"
	"astack/internal/platform"
	"astack/internal/proc"
	"astack/internal/workspace"
)

func TestDispatcherDryRunSkipsRunner(t *testing.T) {
	dir := t.TempDir()
	run := &recordRunner{code: 99}
	plat := platform.NewPlatform("linux", platform.HostPaths{})
	d := &Dispatcher{
		Out:      &bytes.Buffer{},
		Err:      &bytes.Buffer{},
		Platform: plat,
		Loader:   testLoader(t, embeddedAgents),
		Resolver: agents.BinaryResolver{Platform: plat, Looker: mapLooker{path: map[string]string{"claude": "/usr/bin/claude"}, files: map[string]bool{}}},
		Runner:   run,
		Prompts:  proc.PromptStore{Pid: 1},
	}
	code := d.Main([]string{"--workdir", dir, "--prompt-file", writePrompt(t), "--agent", "claude", "--dry-run"})
	if code != 0 || run.calls != 0 {
		t.Fatalf("dry-run code=%d calls=%d", code, run.calls)
	}
	if _, err := os.Stat(filepath.Join(dir, workspace.LockName)); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not leave a lock: %v", err)
	}
}

func TestDispatcherJSONList(t *testing.T) {
	plat := platform.NewPlatform("linux", platform.HostPaths{})
	out := &bytes.Buffer{}
	d := &Dispatcher{
		Out:      out,
		Err:      &bytes.Buffer{},
		Platform: plat,
		Loader:   testLoader(t, embeddedAgents),
		Resolver: agents.BinaryResolver{Platform: plat, Looker: mapLooker{path: map[string]string{"claude": "/c"}, files: map[string]bool{}}},
		Runner:   &recordRunner{},
		Prompts:  proc.PromptStore{Pid: 1},
	}
	if code := d.Main([]string{"--list", "--json"}); code != 0 {
		t.Fatalf("code=%d", code)
	}
	var rows []struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 || rows[0].ID == "" {
		t.Fatalf("rows=%v", rows)
	}
}

func TestDispatcherBusyLock(t *testing.T) {
	dir := t.TempDir()
	lock, err := workspace.AcquireLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	plat := platform.NewPlatform("linux", platform.HostPaths{})
	errb := &bytes.Buffer{}
	d := &Dispatcher{
		Out:      &bytes.Buffer{},
		Err:      errb,
		Platform: plat,
		Loader:   testLoader(t, embeddedAgents),
		Resolver: agents.BinaryResolver{Platform: plat, Looker: mapLooker{path: map[string]string{"claude": "/c"}, files: map[string]bool{}}},
		Runner:   &recordRunner{},
		Prompts:  proc.PromptStore{Pid: 2},
	}
	if code := d.Main([]string{"--workdir", dir, "--prompt-file", writePrompt(t), "--agent", "claude"}); code != workspace.ExitBusy {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
}

func TestProcessRunnerTimeout(t *testing.T) {
	dir := t.TempDir()
	var stub string
	var err error
	if runtime.GOOS == "windows" {
		stub = filepath.Join(dir, "hang.cmd")
		err = os.WriteFile(stub, []byte("@echo off\r\nping -n 20 127.0.0.1 >nul\r\n"), 0644)
	} else {
		stub = filepath.Join(dir, "hang")
		err = os.WriteFile(stub, []byte("#!/bin/sh\nsleep 20\n"), 0755)
	}
	if err != nil {
		t.Fatal(err)
	}
	r := proc.NewProcessRunner(platform.HostPlatform())
	r.Timeout = 200 * time.Millisecond
	r.Stdout = io.Discard
	r.Stderr = io.Discard
	code := r.Run(stub, nil, dir)
	if code != workspace.ExitTimeout {
		t.Fatalf("timeout code=%d want %d", code, workspace.ExitTimeout)
	}
}
