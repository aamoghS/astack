package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// staticLooker is a PATH stand-in used by --bench so the hop does not depend on a paid CLI.
type staticLooker struct {
	path  map[string]string
	files map[string]bool
}

func (s staticLooker) LookPath(name string) string { return s.path[name] }
func (s staticLooker) IsFile(path string) bool     { return s.files[path] }

func (d *Dispatcher) runBench() int {
	silent := *d
	silent.Out = io.Discard
	t0 := time.Now()
	listCode := silent.Main([]string{"--list"})
	list := time.Since(t0)
	if listCode != 0 {
		fmt.Fprintf(d.Err, "astack: --list during --bench exited %d\n", listCode)
		return listCode
	}

	dir, err := os.MkdirTemp("", "astack-bench-")
	if err != nil {
		fmt.Fprintln(d.Err, err)
		return 1
	}
	defer os.RemoveAll(dir)

	stub, err := writeNopWorker(dir)
	if err != nil {
		fmt.Fprintln(d.Err, err)
		return 1
	}
	prompt := filepath.Join(dir, "prompt.txt")
	if err := os.WriteFile(prompt, []byte("noop"), 0600); err != nil {
		fmt.Fprintln(d.Err, err)
		return 1
	}

	embed := []byte(`{"order":["stub"],"agents":{"stub":{"bin":["worker"],"args":["{{prompt}}"]}}}`)
	plat := HostPlatform()
	bench := &Dispatcher{
		Out:      io.Discard,
		Err:      io.Discard,
		Platform: plat,
		Loader:   testLoaderForBench(embed),
		Resolver: BinaryResolver{Platform: plat, Looker: staticLooker{path: map[string]string{"worker": stub}, files: map[string]bool{}}},
		Runner:   NewProcessRunner(plat),
		Prompts:  NewPromptStore(),
	}

	t1 := time.Now()
	hopCode := bench.Main([]string{"--workdir", dir, "--prompt-file", prompt, "--agent", "stub"})
	hop := time.Since(t1)
	if hopCode != 0 {
		fmt.Fprintf(d.Err, "astack: stub dispatch during --bench exited %d\n", hopCode)
		return hopCode
	}

	t2 := time.Now()
	modes := &Dispatcher{
		Out:      io.Discard,
		Err:      io.Discard,
		Platform: plat,
		Loader:   testLoaderForBench(embeddedAgents),
		Resolver: BinaryResolver{Platform: plat, Looker: staticLooker{path: map[string]string{"claude": stub, "gemini": stub}, files: map[string]bool{}}},
		Runner:   NewProcessRunner(plat),
		Prompts:  NewPromptStore(),
	}
	pbCode := modes.Main([]string{"playbook", "--workdir", dir, "--prompt-file", prompt, "--dry-run", "--playbook", "feature"})
	pbHop := time.Since(t2)
	if pbCode != 0 {
		fmt.Fprintf(d.Err, "astack: playbook dry-run during --bench exited %d\n", pbCode)
		return pbCode
	}
	t3 := time.Now()
	swCode := modes.Main([]string{"swarm", "--n", "3", "--workdir", dir, "--prompt-file", prompt, "--dry-run"})
	swHop := time.Since(t3)
	if swCode != 0 {
		fmt.Fprintf(d.Err, "astack: swarm dry-run during --bench exited %d\n", swCode)
		return swCode
	}
	t4 := time.Now()
	arCode := modes.Main([]string{"arena", "--agents", "claude,gemini", "--workdir", dir, "--prompt-file", prompt, "--dry-run"})
	arHop := time.Since(t4)
	if arCode != 0 {
		fmt.Fprintf(d.Err, "astack: arena dry-run during --bench exited %d\n", arCode)
		return arCode
	}

	fmt.Fprintf(d.Out, "astack_list_ms\t%.2f\n", list.Seconds()*1000)
	fmt.Fprintf(d.Out, "astack_dispatch_stub_ms\t%.2f\n", hop.Seconds()*1000)
	fmt.Fprintf(d.Out, "astack_playbook_dry_ms\t%.3f\n", pbHop.Seconds()*1000)
	fmt.Fprintf(d.Out, "astack_swarm_dry_ms\t%.3f\n", swHop.Seconds()*1000)
	fmt.Fprintf(d.Out, "astack_arena_dry_ms\t%.3f\n", arHop.Seconds()*1000)
	fmt.Fprintf(d.Out, "pstack_task\tmodel-round-trip\n")
	fmt.Fprintf(d.Out, "compare\tastack hops are local exec; pstack playbook/swarm/arena spawn Cursor Tasks\n")
	return 0
}

func testLoaderForBench(embed []byte) *ConfigLoader {
	return &ConfigLoader{
		Embed: embed,
		Env:   func(string) string { return "" },
		Cwd:   func() (string, error) { return os.TempDir(), nil },
		Exe:   func() (string, error) { return "/tmp/go-build/exe", nil },
		Read:  os.ReadFile,
	}
}

func writeNopWorker(dir string) (string, error) {
	if runtime.GOOS == "windows" {
		p := filepath.Join(dir, "worker.cmd")
		return p, os.WriteFile(p, []byte("@echo off\r\nexit /b 0\r\n"), 0644)
	}
	p := filepath.Join(dir, "worker")
	return p, os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0755)
}
