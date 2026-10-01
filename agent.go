package main

import (
	"fmt"
	"strings"
)

// Agent is one worker CLI: binaries to find and the argv template to run.
type Agent struct {
	ID   string
	Bins []string
	Args []string
}

func (a Agent) Expand(vars map[string]string) []string {
	out := make([]string, len(a.Args))
	for i, s := range a.Args {
		for k, v := range vars {
			s = strings.ReplaceAll(s, "{{"+k+"}}", v)
		}
		out[i] = s
	}
	return out
}

func argvBytes(args []string) int {
	n := 0
	for _, a := range args {
		n += len(a) + 1
	}
	return n
}

// CommandLine fills the template and, on Windows, swaps a huge prompt for a file path.
func (a Agent) CommandLine(vars map[string]string, plat Platform) []string {
	args := a.Expand(vars)
	if argvBytes(args) <= plat.MaxArgBytes() {
		return args
	}
	next := make(map[string]string, len(vars))
	for k, v := range vars {
		next[k] = v
	}
	next["prompt"] = "Read the UTF-8 file at " + vars["prompt_file"] + " and follow it as your complete task. Do not commit that file."
	return a.Expand(next)
}

// AgentRegistry owns the configured workers and the auto-pick order.
type AgentRegistry struct {
	Default string
	Order   []string
	Footer  string
	byID    map[string]Agent
}

func (r *AgentRegistry) Get(id string) (Agent, bool) {
	a, ok := r.byID[id]
	return a, ok
}

func (r *AgentRegistry) All() map[string]Agent {
	return r.byID
}

type PickResult struct {
	Agent Agent
	Code  int
	Err   string
}

func (r *AgentRegistry) Pick(name string, resolved map[string]string) PickResult {
	if name == "auto" || name == "" {
		for _, id := range r.Order {
			if resolved[id] != "" {
				return PickResult{Agent: r.byID[id], Code: 0}
			}
		}
		return PickResult{Code: 3, Err: "astack: no worker CLI installed; conductor should edit in this chat. not rerouting."}
	}
	a, ok := r.byID[name]
	if !ok {
		return PickResult{Code: 2, Err: fmt.Sprintf("astack: unknown agent %s. not rerouting.", name)}
	}
	if resolved[name] == "" {
		return PickResult{Code: 2, Err: fmt.Sprintf("astack: %s is not installed; not rerouting.", name)}
	}
	return PickResult{Agent: a, Code: 0}
}
