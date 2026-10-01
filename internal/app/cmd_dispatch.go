package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"astack/internal/agents"
	"astack/internal/proc"
	"astack/internal/workspace"
)

// DispatchCommand is the default command: one worker on the real workdir,
// with an optional verify-and-retry loop.
type DispatchCommand struct{}

func (DispatchCommand) Run(d *Dispatcher, in Invocation) int {
	o, reg, resolved, started := in.Flags, in.Registry, in.Resolved, in.Started
	if o.Workdir == "" || o.PromptFile == "" {
		fmt.Fprintln(d.Err, usageText())
		return 1
	}
	st, err := os.Stat(o.Workdir)
	if err != nil || !st.IsDir() {
		fmt.Fprintf(d.Err, "workdir not found: %s\n", o.Workdir)
		return 1
	}
	pst, err := os.Stat(o.PromptFile)
	if err != nil || pst.IsDir() {
		fmt.Fprintf(d.Err, "prompt file not found: %s\n", o.PromptFile)
		return 1
	}
	promptBytes, err := os.ReadFile(o.PromptFile)
	if err != nil {
		fmt.Fprintf(d.Err, "prompt file not found: %s\n", o.PromptFile)
		return 1
	}
	absWork, err := filepath.Abs(o.Workdir)
	if err != nil {
		fmt.Fprintln(d.Err, err)
		return 1
	}

	name := o.Agent
	if name == "" {
		name = reg.Default
	}
	if name == "" {
		name = "auto"
	}
	pick := reg.Pick(name, resolved)
	if pick.Code != 0 {
		fmt.Fprintln(d.Err, pick.Err)
		return pick.Code
	}

	if !o.DryRun {
		lock, err := workspace.AcquireLock(absWork)
		if err != nil {
			fmt.Fprintln(d.Err, err)
			return workspace.ExitBusy
		}
		defer lock.Release()
	}

	models, _ := agents.ParseModelChoice(o.Model)
	model := reg.Model(agents.RoleImplement, pick.Agent.ID, models)
	bin := resolved[pick.Agent.ID]
	task := strings.TrimRight(string(promptBytes), "\r\n") + "\n\n" + reg.Footer
	attempts := 1
	if o.Verify != "" {
		attempts += o.Retries
	}
	code := 0
	for attempt := 1; attempt <= attempts; attempt++ {
		var stop bool
		code, stop = d.attempt(pick.Agent, bin, model, task, reg.Footer, absWork, o.DryRun)
		if stop || code != 0 || o.Verify == "" {
			break
		}
		res := d.verify(o.Verify, absWork)
		if res.Passed() {
			fmt.Fprintf(d.Err, "astack verify: pass attempt=%d\n", attempt)
			break
		}
		code = proc.ExitVerify
		fmt.Fprintf(d.Err, "astack verify: fail exit=%d attempt=%d/%d\n", res.Code, attempt, attempts)
		task = proc.RetryTask(task, o.Verify, res, attempt)
	}
	if o.DryRun {
		return code
	}
	wall := time.Since(started).Seconds() * 1000
	d.printResult(pick.Agent.ID, bin, wall, code)
	return code
}

// attempt runs the worker once. stop is true for a dry run (nothing to verify).
func (d *Dispatcher) attempt(a agents.Agent, bin, model, task, footer, workdir string, dryRun bool) (code int, stop bool) {
	promptPath, err := d.Prompts.Write(workdir, task)
	if err != nil {
		fmt.Fprintf(d.Err, "astack: cannot write prompt file: %v\n", err)
		return 1, true
	}
	defer d.Prompts.Remove(promptPath)
	args := a.CommandLine(map[string]string{
		"prompt":      task,
		"footer":      footer,
		"workdir":     workdir,
		"prompt_file": promptPath,
		"model":       model,
	}, d.Platform)
	if dryRun {
		return d.printDryRun(a.ID, bin, args), true
	}
	if !d.JSON {
		fmt.Fprintf(d.Out, "astack worker: %s\n", a.ID)
	}
	return d.Runner.Run(bin, args, workdir), false
}

func (d *Dispatcher) verify(command, workdir string) proc.VerifyResult {
	v := d.Verifier
	if v == nil {
		v = proc.NewShellVerifier()
	}
	fmt.Fprintf(d.Err, "astack verify: %s\n", command)
	return v.Verify(command, workdir, d.Timeout, d.Err)
}
