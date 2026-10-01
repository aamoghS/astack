package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"astack/internal/agents"
	"astack/internal/platform"
	"astack/internal/proc"
)

// Dispatcher is the conductor-facing object: load config, pick a worker, run it.
type Dispatcher struct {
	Out      io.Writer
	Err      io.Writer
	Platform platform.Platform
	Loader   *agents.ConfigLoader
	Resolver agents.BinaryResolver
	Runner   proc.Runner
	Prompts  proc.PromptStore
	Verifier proc.Verifier
	JSON     bool
	Timeout  time.Duration
}

func NewDispatcher(agentsJSON []byte) *Dispatcher {
	p := platform.HostPlatform()
	return &Dispatcher{
		Out:      os.Stdout,
		Err:      os.Stderr,
		Platform: p,
		Loader:   agents.NewConfigLoader(agentsJSON),
		Resolver: agents.NewBinaryResolver(p),
		Runner:   proc.NewProcessRunner(p),
		Prompts:  proc.NewPromptStore(),
		Verifier: proc.NewShellVerifier(),
	}
}

func (d *Dispatcher) apply(o Flags) {
	d.JSON = o.JSON
	d.Timeout = o.Timeout
	if pr, ok := d.Runner.(*proc.ProcessRunner); ok {
		pr.Timeout = o.Timeout
	}
}

func (d *Dispatcher) fork() *Dispatcher {
	cp := *d
	cp.Prompts = proc.NewPromptStore()
	return &cp
}

// Invocation is one parsed run: the flags, the loaded registry, and where
// each installed worker CLI lives.
type Invocation struct {
	Flags    Flags
	Registry *agents.Registry
	Resolved map[string]string
	Started  time.Time
}

// Command is one astack subcommand.
type Command interface {
	Run(d *Dispatcher, in Invocation) int
}

// commandFor maps the parsed flags to the subcommand that handles them.
func commandFor(o Flags) Command {
	if o.List {
		return ListCommand{}
	}
	switch o.Cmd {
	case "tui":
		return TUICommand{}
	case "playbook":
		return PlaybookCommand{}
	case "swarm":
		return SwarmCommand{}
	case "arena":
		return ArenaCommand{}
	}
	if agents.IsPanelRole(o.Cmd) {
		return PanelCommand{Role: o.Cmd}
	}
	return DispatchCommand{}
}

func (d *Dispatcher) Main(argv []string) int {
	started := time.Now()
	o, err := parseArgs(argv)
	if err != nil {
		fmt.Fprintln(d.Err, err)
		fmt.Fprintln(d.Err, usageText())
		return 1
	}
	if o.Help {
		fmt.Fprintln(d.Out, usageText())
		return 0
	}
	d.apply(o)
	if o.Bench {
		return d.runBench()
	}

	reg, src, err := d.Loader.Load()
	if err != nil {
		fmt.Fprintf(d.Err, "astack: bad agents.json (%s): %v\n", src, err)
		return 1
	}

	in := Invocation{Flags: o, Registry: reg, Resolved: d.Resolver.ResolveAll(reg), Started: started}
	return commandFor(o).Run(d, in)
}

// ListCommand prints each configured worker as installed or absent.
type ListCommand struct{}

func (ListCommand) Run(d *Dispatcher, in Invocation) int {
	reg, resolved := in.Registry, in.Resolved
	type row struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	rows := make([]row, 0, len(reg.Order))
	for _, id := range reg.Order {
		status := "absent"
		if resolved[id] != "" {
			status = "installed"
		}
		rows = append(rows, row{ID: id, Status: status})
		if !d.JSON {
			fmt.Fprintf(d.Out, "%s\t%s\n", id, status)
		}
	}
	if d.JSON {
		enc := json.NewEncoder(d.Out)
		_ = enc.Encode(rows)
	}
	return 0
}

func (d *Dispatcher) printDryRun(worker, bin string, args []string) int {
	if d.JSON {
		_ = json.NewEncoder(d.Out).Encode(map[string]any{
			"dry_run": true,
			"worker":  worker,
			"bin":     bin,
			"argc":    len(args),
		})
		return 0
	}
	fmt.Fprintf(d.Out, "astack dry-run worker=%s bin=%s argc=%d\n", worker, bin, len(args))
	return 0
}

func (d *Dispatcher) printResult(worker, bin string, wallMs float64, code int) {
	if d.JSON {
		_ = json.NewEncoder(d.Out).Encode(map[string]any{
			"worker":  worker,
			"bin":     bin,
			"wall_ms": wallMs,
			"exit":    code,
		})
		return
	}
	fmt.Fprintf(d.Err, "astack wall_ms=%.2f worker=%s exit=%d\n", wallMs, worker, code)
	if code == 0 {
		fmt.Fprintln(d.Err, "astack: review git diff; do not spawn a Cursor Task for this work")
	}
}
