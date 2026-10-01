package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestWorkdirLock(t *testing.T) {
	dir := t.TempDir()
	a, err := acquireWorkdirLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireWorkdirLock(dir); err == nil {
		t.Fatal("second lock should fail")
	}
	a.Release()
	b, err := acquireWorkdirLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	b.Release()
}

func TestDispatcherDryRunSkipsRunner(t *testing.T) {
	dir := t.TempDir()
	run := &recordRunner{code: 99}
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
	code := d.Main([]string{"--workdir", dir, "--prompt-file", writePrompt(t), "--agent", "claude", "--dry-run"})
	if code != 0 || run.calls != 0 {
		t.Fatalf("dry-run code=%d calls=%d", code, run.calls)
	}
	if _, err := os.Stat(filepath.Join(dir, lockName)); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not leave a lock: %v", err)
	}
}

func TestDispatcherJSONList(t *testing.T) {
	plat := NewPlatform("linux", HostPaths{})
	out := &bytes.Buffer{}
	d := &Dispatcher{
		Out:      out,
		Err:      &bytes.Buffer{},
		Platform: plat,
		Loader:   testLoader(t, embeddedAgents),
		Resolver: BinaryResolver{Platform: plat, Looker: mapLooker{path: map[string]string{"claude": "/c"}, files: map[string]bool{}}},
		Runner:   &recordRunner{},
		Prompts:  PromptStore{Pid: 1},
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
	lock, err := acquireWorkdirLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Release()
	plat := NewPlatform("linux", HostPaths{})
	errb := &bytes.Buffer{}
	d := &Dispatcher{
		Out:      &bytes.Buffer{},
		Err:      errb,
		Platform: plat,
		Loader:   testLoader(t, embeddedAgents),
		Resolver: BinaryResolver{Platform: plat, Looker: mapLooker{path: map[string]string{"claude": "/c"}, files: map[string]bool{}}},
		Runner:   &recordRunner{},
		Prompts:  PromptStore{Pid: 2},
	}
	if code := d.Main([]string{"--workdir", dir, "--prompt-file", writePrompt(t), "--agent", "claude"}); code != exitBusy {
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
	r := NewProcessRunner(HostPlatform())
	r.Timeout = 200 * time.Millisecond
	r.Stdout = io.Discard
	r.Stderr = io.Discard
	code := r.Run(stub, nil, dir)
	if code != exitTimeout {
		t.Fatalf("timeout code=%d want %d", code, exitTimeout)
	}
}
