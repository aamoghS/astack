package main

import (
	"io"
	"os"
)

func (d *Dispatcher) runTUI(o Flags, reg *AgentRegistry, resolved map[string]string) int {
	t := TUI{Host: newOSHost(os.Stdin, os.Stdout), Backend: d, Err: d.Err}
	return t.Run(tuiOptions(o), workerRows(reg, resolved))
}

// RunArgv makes the Dispatcher a TUIBackend: one child run with output sent to log.
func (d *Dispatcher) RunArgv(argv []string, log io.Writer) int {
	child := d.fork()
	child.Out, child.Err = log, log
	if pr, ok := child.Runner.(*ProcessRunner); ok {
		cp := *pr
		cp.Stdout, cp.Stderr = log, log
		cp.Stdin = nil
		child.Runner = &cp
	}
	return child.Main(argv)
}

func tuiOptions(o Flags) TUIOptions {
	mode := o.Cmd
	if mode == "tui" || mode == "agent" {
		mode = ""
	}
	return TUIOptions{
		Workdir:  o.Workdir,
		Mode:     mode,
		Agent:    o.Agent,
		Playbook: o.Playbook,
		N:        o.N,
		Agents:   o.Agents,
		Timeout:  o.Timeout,
		DryRun:   o.DryRun,
		Verify:   o.Verify,
		Model:    o.Model,
		Judge:    o.Judge,
	}
}

func workerRows(reg *AgentRegistry, resolved map[string]string) []string {
	if reg == nil {
		return nil
	}
	out := make([]string, 0, len(reg.Order))
	for _, id := range reg.Order {
		st := "absent"
		if resolved[id] != "" {
			st = "installed"
		}
		out = append(out, id+" "+st)
	}
	return out
}
