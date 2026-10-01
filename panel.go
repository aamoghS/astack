package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// Panel runs one read-only role on several CLIs in parallel and collects
// their replies. A CLI with read_args runs in place (the CLI enforces
// read-only). Any other CLI runs on a throwaway copy of the workdir, so a
// stray edit never reaches the real repo.
type Panel struct {
	Role     string
	Members  []Agent
	Registry *AgentRegistry
	Resolved map[string]string
	Models   ModelChoice
	Platform Platform
	Runner   Runner
	Log      io.Writer
}

type PanelReply struct {
	Agent string `json:"agent"`
	Model string `json:"model,omitempty"`
	Mode  string `json:"mode"`
	Exit  int    `json:"exit"`
	Text  string `json:"text"`
}

func (r PanelReply) OK() bool { return r.Exit == 0 && strings.TrimSpace(r.Text) != "" }

const (
	modeInPlace  = "read-only"
	modeIsolated = "isolated-copy"
	modeScratch  = "scratch-dir"
)

func (d *Dispatcher) newPanel(role string, members []Agent, reg *AgentRegistry, resolved map[string]string, models ModelChoice) Panel {
	return Panel{
		Role:     role,
		Members:  members,
		Registry: reg,
		Resolved: resolved,
		Models:   models,
		Platform: d.Platform,
		Runner:   d.Runner,
		Log:      &lockedWriter{w: d.Err},
	}
}

// Mode says where a member runs. An empty workdir means the task carries
// everything it needs (judge, synth), so members get an empty scratch dir.
func (p Panel) Mode(a Agent, workdir string) string {
	switch {
	case workdir == "":
		return modeScratch
	case a.CanReadOnly():
		return modeInPlace
	default:
		return modeIsolated
	}
}

func (p Panel) Run(task, workdir string) []PanelReply {
	replies := make([]PanelReply, len(p.Members))
	var wg sync.WaitGroup
	for i, a := range p.Members {
		wg.Add(1)
		go func(i int, a Agent) {
			defer wg.Done()
			replies[i] = p.runMember(a, task, workdir)
		}(i, a)
	}
	wg.Wait()
	return replies
}

func (p Panel) runMember(a Agent, task, workdir string) PanelReply {
	mode := p.Mode(a, workdir)
	reply := PanelReply{Agent: a.ID, Mode: mode, Model: p.Registry.Model(p.Role, a.ID, p.Models)}
	dir, cleanup, err := p.memberDir(mode, workdir)
	if err != nil {
		fmt.Fprintf(p.Log, "astack %s/%s: %v\n", p.Role, a.ID, err)
		reply.Exit = 1
		return reply
	}
	defer cleanup()

	footer := readOnlyFooter(p.Role)
	body := task + "\n\n" + footer
	store := PromptStore{Pid: os.Getpid(), Tag: p.Role + "." + a.ID}
	promptPath, err := store.Write(dir, body)
	if err != nil {
		fmt.Fprintf(p.Log, "astack %s/%s: cannot write prompt: %v\n", p.Role, a.ID, err)
		reply.Exit = 1
		return reply
	}
	defer store.Remove(promptPath)

	vars := map[string]string{
		"prompt":      body,
		"footer":      footer,
		"workdir":     dir,
		"prompt_file": promptPath,
		"model":       reply.Model,
	}
	var args []string
	if mode == modeInPlace || (mode == modeScratch && a.CanReadOnly()) {
		args = a.ReadCommandLine(vars, p.Platform)
	} else {
		args = a.CommandLine(vars, p.Platform)
	}
	bin := p.Resolved[a.ID]
	fmt.Fprintf(p.Log, "astack %s: %s running (%s)\n", p.Role, a.ID, mode)
	if or, ok := p.Runner.(OutputRunner); ok {
		var out bytes.Buffer
		reply.Exit = or.RunOutput(bin, args, dir, &out, p.Log)
		reply.Text = strings.TrimSpace(out.String())
	} else {
		reply.Exit = p.Runner.Run(bin, args, dir)
	}
	return reply
}

func (p Panel) memberDir(mode, workdir string) (string, func(), error) {
	switch mode {
	case modeInPlace:
		return workdir, func() {}, nil
	case modeIsolated:
		dir, err := isolateWorkdir(workdir)
		if err != nil {
			return "", nil, err
		}
		return dir, func() { os.RemoveAll(dir) }, nil
	default:
		dir, err := os.MkdirTemp("", "astack-"+p.Role+"-")
		if err != nil {
			return "", nil, err
		}
		return dir, func() { os.RemoveAll(dir) }, nil
	}
}

// PanelOutcome is a finished panel: every reply plus the merged answer when
// two or more members answered.
type PanelOutcome struct {
	Role      string       `json:"role"`
	Replies   []PanelReply `json:"replies"`
	Synthesis *PanelReply  `json:"synthesis,omitempty"`
}

func (o PanelOutcome) OK() bool {
	for _, r := range o.Replies {
		if r.OK() {
			return true
		}
	}
	return false
}

func (o PanelOutcome) Code() int {
	if o.OK() {
		return 0
	}
	for _, r := range o.Replies {
		if r.Exit != 0 {
			return r.Exit
		}
	}
	return 1
}

// Summary is the text later steps consume: the synthesis, or the only reply.
func (o PanelOutcome) Summary() string {
	if o.Synthesis != nil && o.Synthesis.OK() {
		return o.Synthesis.Text
	}
	var parts []string
	for _, r := range o.Replies {
		if r.OK() {
			parts = append(parts, "### "+r.Agent+"\n"+r.Text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func (o PanelOutcome) Print(w io.Writer) {
	for _, r := range o.Replies {
		fmt.Fprintf(w, "=== %s: %s (exit %d, %s) ===\n%s\n\n", o.Role, r.Agent, r.Exit, r.Mode, r.Text)
	}
	if o.Synthesis != nil {
		fmt.Fprintf(w, "=== %s synthesis: %s (exit %d) ===\n%s\n", o.Role, o.Synthesis.Agent, o.Synthesis.Exit, o.Synthesis.Text)
	}
}

// consult runs a panel for role and, when two or more members answered,
// a synth member that merges their replies.
func (d *Dispatcher) consult(role string, members []Agent, task, workdir string, reg *AgentRegistry, resolved map[string]string, models ModelChoice) PanelOutcome {
	out := PanelOutcome{Role: role, Replies: d.newPanel(role, members, reg, resolved, models).Run(task, workdir)}
	var answered []PanelReply
	for _, r := range out.Replies {
		if r.OK() {
			answered = append(answered, r)
		}
	}
	if len(answered) < 2 {
		return out
	}
	synth, pick := reg.Members(RoleSynth, nil, resolved)
	if pick.Code != 0 {
		return out
	}
	reply := d.newPanel(RoleSynth, synth[:1], reg, resolved, models).Run(synthTask(role, task, answered), "")[0]
	out.Synthesis = &reply
	return out
}

func synthTask(role, task string, replies []PanelReply) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Several independent %s agents answered the same task. Merge them into one answer. "+
		"Keep points backed by evidence, drop duplicates, and flag where they disagree and which side the evidence favors.\n\n", role)
	b.WriteString("Original task:\n")
	b.WriteString(task)
	for _, r := range replies {
		fmt.Fprintf(&b, "\n\n### Answer from %s\n%s", r.Agent, r.Text)
	}
	return b.String()
}

// roleTask frames the human's prompt for a panel role.
func roleTask(role, prompt, workdir string) string {
	return panelRoles[role] + "\n\nRepository: " + workdir + "\n\nTask:\n" + strings.TrimSpace(prompt)
}

func (d *Dispatcher) runPanelCmd(o Flags, reg *AgentRegistry, resolved map[string]string) int {
	role := o.Cmd
	if o.Workdir == "" || (o.PromptFile == "" && role != RoleReview) {
		fmt.Fprintln(d.Err, usageText())
		return 1
	}
	absWork, err := absDir(o.Workdir)
	if err != nil {
		fmt.Fprintln(d.Err, err)
		return 1
	}
	prompt := ""
	if o.PromptFile != "" {
		b, err := os.ReadFile(o.PromptFile)
		if err != nil {
			fmt.Fprintf(d.Err, "prompt file not found: %s\n", o.PromptFile)
			return 1
		}
		prompt = string(b)
	}
	members, pick := reg.Members(role, o.Agents, resolved)
	if pick.Code != 0 {
		fmt.Fprintln(d.Err, pick.Err)
		return pick.Code
	}
	models, _ := ParseModelChoice(o.Model)

	var task string
	if role == RoleReview {
		change := GitChange(absWork)
		if change == "" {
			fmt.Fprintln(d.Out, "astack review: no changes to review")
			return 0
		}
		task = reviewTask(prompt, change)
	} else {
		task = roleTask(role, prompt, absWork)
	}

	if o.DryRun {
		return d.printPanelDryRun(role, members, absWork, reg, models)
	}
	out := d.consult(role, members, task, absWork, reg, resolved, models)
	if d.JSON {
		_ = json.NewEncoder(d.Out).Encode(out)
	} else {
		out.Print(d.Out)
	}
	return out.Code()
}

func (d *Dispatcher) printPanelDryRun(role string, members []Agent, workdir string, reg *AgentRegistry, models ModelChoice) int {
	p := d.newPanel(role, members, reg, nil, models)
	rows := make([]map[string]string, 0, len(members))
	var parts []string
	for _, a := range members {
		mode := p.Mode(a, workdir)
		model := reg.Model(role, a.ID, models)
		rows = append(rows, map[string]string{"agent": a.ID, "mode": mode, "model": model})
		label := a.ID + "(" + mode
		if model != "" {
			label += "," + model
		}
		parts = append(parts, label+")")
	}
	if d.JSON {
		_ = json.NewEncoder(d.Out).Encode(map[string]any{"role": role, "dry_run": true, "members": rows})
		return 0
	}
	fmt.Fprintf(d.Out, "astack %s dry-run members=%s\n", role, strings.Join(parts, " "))
	return 0
}

func reviewTask(prompt, change string) string {
	intent := strings.TrimSpace(prompt)
	if intent == "" {
		intent = "(not given; infer it from the change)"
	}
	return panelRoles[RoleReview] + "\n\nWhat the change is meant to do:\n" + intent + "\n\nChange:\n```diff\n" + change + "\n```"
}
