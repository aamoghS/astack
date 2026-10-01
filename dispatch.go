package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Flags struct {
	Cmd        string
	Agent      string
	List       bool
	Help       bool
	Bench      bool
	JSON       bool
	DryRun     bool
	Timeout    time.Duration
	N          int
	Agents     []string
	Playbook   string
	Workdir    string
	PromptFile string
}

func usageText() string {
	return "usage: astack [tui|agent|playbook|swarm|arena] --workdir <repo> --prompt-file <file> [--agent auto] [--n 3] [--agents claude,codex] [--playbook file] [--list] [--bench] [--json] [--dry-run] [--timeout 0s]"
}

func parseArgs(argv []string) (Flags, error) {
	var o Flags
	if len(argv) > 0 && !strings.HasPrefix(argv[0], "-") {
		switch argv[0] {
		case "playbook", "swarm", "arena", "tui", "agent":
			o.Cmd = argv[0]
			argv = argv[1:]
		}
	}
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		need := func() (string, error) {
			i++
			if i >= len(argv) {
				return "", fmt.Errorf("missing value for %s", a)
			}
			return argv[i], nil
		}
		switch a {
		case "-h", "--help":
			o.Help = true
		case "--list":
			o.List = true
		case "--bench":
			o.Bench = true
		case "--json":
			o.JSON = true
		case "--dry-run":
			o.DryRun = true
		case "--timeout":
			v, err := need()
			if err != nil {
				return o, err
			}
			d, err := time.ParseDuration(v)
			if err != nil {
				return o, fmt.Errorf("bad --timeout %s: %v", v, err)
			}
			o.Timeout = d
		case "--n":
			v, err := need()
			if err != nil {
				return o, err
			}
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 8 {
				return o, fmt.Errorf("bad --n %s (want 1-8)", v)
			}
			o.N = n
		case "--agents":
			v, err := need()
			if err != nil {
				return o, err
			}
			for _, p := range strings.Split(v, ",") {
				p = strings.TrimSpace(p)
				if p != "" {
					o.Agents = append(o.Agents, p)
				}
			}
		case "--playbook":
			v, err := need()
			if err != nil {
				return o, err
			}
			o.Playbook = v
		case "--agent":
			v, err := need()
			if err != nil {
				return o, err
			}
			o.Agent = v
		case "--workdir":
			v, err := need()
			if err != nil {
				return o, err
			}
			o.Workdir = v
		case "--prompt-file":
			v, err := need()
			if err != nil {
				return o, err
			}
			o.PromptFile = v
		default:
			return o, fmt.Errorf("unknown flag %s", a)
		}
	}
	return o, nil
}

// Dispatcher is the conductor-facing object: load config, pick a worker, run it.
type Dispatcher struct {
	Out      io.Writer
	Err      io.Writer
	Platform Platform
	Loader   *ConfigLoader
	Resolver BinaryResolver
	Runner   Runner
	Prompts  PromptStore
	JSON     bool
	Timeout  time.Duration
}

func NewDispatcher() *Dispatcher {
	p := HostPlatform()
	return &Dispatcher{
		Out:      os.Stdout,
		Err:      os.Stderr,
		Platform: p,
		Loader:   NewConfigLoader(embeddedAgents),
		Resolver: NewBinaryResolver(p),
		Runner:   NewProcessRunner(p),
		Prompts:  NewPromptStore(),
	}
}

func (d *Dispatcher) apply(o Flags) {
	d.JSON = o.JSON
	d.Timeout = o.Timeout
	if pr, ok := d.Runner.(*ProcessRunner); ok {
		pr.Timeout = o.Timeout
	}
}

func (d *Dispatcher) fork() *Dispatcher {
	cp := *d
	cp.Prompts = NewPromptStore()
	return &cp
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

	resolved := d.Resolver.ResolveAll(reg)
	if o.List {
		return d.printList(reg, resolved)
	}
	switch o.Cmd {
	case "tui":
		return d.runTUI(o, reg, resolved)
	case "playbook":
		return d.runPlaybook(o, reg, resolved)
	case "swarm":
		return d.runSwarm(o, reg, resolved)
	case "arena":
		return d.runArena(o, reg, resolved)
	}

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
		lock, err := acquireWorkdirLock(absWork)
		if err != nil {
			fmt.Fprintln(d.Err, err)
			return exitBusy
		}
		defer lock.Release()
	}

	task := strings.TrimRight(string(promptBytes), "\r\n") + "\n\n" + reg.Footer
	promptPath, err := d.Prompts.Write(absWork, task)
	if err != nil {
		fmt.Fprintf(d.Err, "astack: cannot write prompt file: %v\n", err)
		return 1
	}
	defer d.Prompts.Remove(promptPath)

	args := pick.Agent.CommandLine(map[string]string{
		"prompt":      task,
		"footer":      reg.Footer,
		"workdir":     absWork,
		"prompt_file": promptPath,
	}, d.Platform)
	bin := resolved[pick.Agent.ID]

	if o.DryRun {
		return d.printDryRun(pick.Agent.ID, bin, args)
	}

	if !d.JSON {
		fmt.Fprintf(d.Out, "astack worker: %s\n", pick.Agent.ID)
	}
	code := d.Runner.Run(bin, args, absWork)
	wall := time.Since(started).Seconds() * 1000
	d.printResult(pick.Agent.ID, bin, wall, code)
	return code
}

func (d *Dispatcher) printList(reg *AgentRegistry, resolved map[string]string) int {
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
