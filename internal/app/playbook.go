package app

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"astack/internal/agents"
	"astack/internal/workspace"
)

type Playbook struct {
	Name  string         `json:"name"`
	Steps []PlaybookStep `json:"steps"`
}

type PlaybookStep struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Agent string `json:"agent"`
}

//go:embed playbooks/*.json
var embeddedPlaybooks embed.FS

const defaultPlaybookJSON = `{
  "name": "feature",
  "steps": [
    {"id": "implement", "kind": "dispatch", "agent": "auto"},
    {"id": "review", "kind": "review"}
  ]
}`

func ParsePlaybook(b []byte) (Playbook, error) {
	var p Playbook
	if err := json.Unmarshal(b, &p); err != nil {
		return p, err
	}
	if p.Name == "" {
		return p, fmt.Errorf("playbook missing name")
	}
	if len(p.Steps) == 0 {
		return p, fmt.Errorf("playbook %s has no steps", p.Name)
	}
	for i, s := range p.Steps {
		if s.ID == "" {
			return p, fmt.Errorf("playbook step %d missing id", i)
		}
		if s.Kind != "dispatch" && !agents.IsPanelRole(s.Kind) {
			return p, fmt.Errorf("playbook step %s has unknown kind %s", s.ID, s.Kind)
		}
	}
	return p, nil
}

func loadPlaybook(path string) (Playbook, error) {
	if path == "" {
		return ParsePlaybook([]byte(defaultPlaybookJSON))
	}
	if b, err := os.ReadFile(path); err == nil {
		return ParsePlaybook(b)
	} else if filepath.Dir(path) != "." {
		return Playbook{}, err
	}
	name := filepath.Base(path)
	if !strings.HasSuffix(strings.ToLower(name), ".json") {
		name += ".json"
	}
	if b, err := embeddedPlaybooks.ReadFile("playbooks/" + name); err == nil {
		return ParsePlaybook(b)
	}
	return Playbook{}, fmt.Errorf("playbook not found: %s", path)
}

// PlaybookCommand runs a playbook's steps in order.
type PlaybookCommand struct{}

func (PlaybookCommand) Run(d *Dispatcher, in Invocation) int {
	o, reg, resolved := in.Flags, in.Registry, in.Resolved
	pb, err := loadPlaybook(o.Playbook)
	if err != nil {
		fmt.Fprintln(d.Err, err)
		return 1
	}
	run := &playbookRun{d: d, o: o, reg: reg, resolved: resolved, name: pb.Name}
	defer run.cleanup()
	for _, s := range pb.Steps {
		if code := run.step(s); code != 0 {
			return code
		}
	}
	if d.JSON {
		_ = json.NewEncoder(d.Out).Encode(map[string]any{"playbook": pb.Name, "steps": run.out, "task": false})
	}
	return 0
}

type playbookStepOut struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Agent  string `json:"agent,omitempty"`
	Action string `json:"action"`
}

// playbookRun carries state between steps: notes from panel steps are
// prepended to the task the next dispatch step sends to the worker.
type playbookRun struct {
	d        *Dispatcher
	o        Flags
	reg      *agents.Registry
	resolved map[string]string
	name     string
	notes    []string
	temps    []string
	out      []playbookStepOut
}

func (r *playbookRun) step(s PlaybookStep) int {
	switch {
	case s.Kind == "dispatch":
		return r.dispatch(s)
	case s.Kind == agents.RoleReview:
		return r.review(s)
	default:
		return r.consult(s)
	}
}

func (r *playbookRun) dispatch(s PlaybookStep) int {
	pick := r.reg.Pick(orAuto(s.Agent), r.resolved)
	if pick.Code != 0 {
		fmt.Fprintln(r.d.Err, pick.Err)
		return pick.Code
	}
	r.out = append(r.out, playbookStepOut{ID: s.ID, Kind: s.Kind, Agent: pick.Agent.ID, Action: "dispatch"})
	child := r.o
	child.Cmd = ""
	child.Agent = pick.Agent.ID
	child.Playbook = ""
	child.JSON = false
	if pf, ok := r.promptWithNotes(); ok {
		child.PromptFile = pf
	}
	return r.d.fork().Main(flattenDispatch(child))
}

// review runs the review panel on the uncommitted change. It is advice: a
// reviewer failing or finding problems never fails the playbook.
func (r *playbookRun) review(s PlaybookStep) int {
	r.out = append(r.out, playbookStepOut{ID: s.ID, Kind: s.Kind, Action: "review-panel"})
	if !r.d.JSON {
		fmt.Fprintf(r.d.Out, "astack playbook %s/%s: review\n", r.name, s.ID)
	}
	if r.o.DryRun || r.o.Workdir == "" {
		return 0
	}
	if stat := workspace.DiffStat(r.o.Workdir); stat != "" && !r.d.JSON {
		fmt.Fprintln(r.d.Out, stat)
	}
	change := workspace.GitChange(r.o.Workdir)
	if change == "" {
		return 0
	}
	members, pick := r.reg.Members(agents.RoleReview, agentList(s.Agent), r.resolved)
	if pick.Code != 0 {
		return 0
	}
	models, _ := agents.ParseModelChoice(r.o.Model)
	out := r.d.consult(agents.RoleReview, members, reviewTask(r.prompt(), change), r.o.Workdir, r.reg, r.resolved, models)
	if !r.d.JSON {
		out.Print(r.d.Out)
	}
	return 0
}

// consult runs an analysis panel (why, architect, ...) and keeps its answer
// as notes for the next dispatch step.
func (r *playbookRun) consult(s PlaybookStep) int {
	members, pick := r.reg.Members(s.Kind, agentList(s.Agent), r.resolved)
	if pick.Code != 0 {
		fmt.Fprintln(r.d.Err, pick.Err)
		return pick.Code
	}
	r.out = append(r.out, playbookStepOut{ID: s.ID, Kind: s.Kind, Agent: strings.Join(agentIDs(members), ","), Action: "panel"})
	if !r.d.JSON {
		fmt.Fprintf(r.d.Out, "astack playbook %s/%s: %s panel %v\n", r.name, s.ID, s.Kind, agentIDs(members))
	}
	if r.o.DryRun {
		return 0
	}
	models, _ := agents.ParseModelChoice(r.o.Model)
	out := r.d.consult(s.Kind, members, roleTask(s.Kind, r.prompt(), r.o.Workdir), r.o.Workdir, r.reg, r.resolved, models)
	if !r.d.JSON {
		out.Print(r.d.Out)
	}
	if !out.OK() {
		fmt.Fprintf(r.d.Err, "astack playbook %s/%s: no %s answer; continuing without it\n", r.name, s.ID, s.Kind)
		return 0
	}
	r.notes = append(r.notes, "## "+s.Kind+" notes\n"+out.Summary())
	return 0
}

func (r *playbookRun) prompt() string {
	b, _ := os.ReadFile(r.o.PromptFile)
	return string(b)
}

// promptWithNotes writes the task plus panel notes to a temp file outside
// the workdir. ok is false when there are no notes.
func (r *playbookRun) promptWithNotes() (string, bool) {
	if len(r.notes) == 0 {
		return "", false
	}
	f, err := os.CreateTemp("", "astack-playbook-*.txt")
	if err != nil {
		return "", false
	}
	body := strings.TrimSpace(r.prompt()) + "\n\nNotes from earlier read-only steps (advice, not orders; verify before relying on them):\n\n" + strings.Join(r.notes, "\n\n")
	_, err = f.WriteString(body)
	f.Close()
	if err != nil {
		os.Remove(f.Name())
		return "", false
	}
	r.temps = append(r.temps, f.Name())
	return f.Name(), true
}

func (r *playbookRun) cleanup() {
	for _, p := range r.temps {
		os.Remove(p)
	}
}

// agentList reads a step's "agent" field: empty or "auto" means the role default.
func agentList(v string) []string {
	if v == "" || v == "auto" {
		return nil
	}
	var out []string
	for _, id := range strings.Split(v, ",") {
		if id = strings.TrimSpace(id); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func flattenDispatch(o Flags) []string {
	args := []string{"--workdir", o.Workdir, "--prompt-file", o.PromptFile, "--agent", o.Agent}
	if o.DryRun {
		args = append(args, "--dry-run")
	}
	if o.Timeout > 0 {
		args = append(args, "--timeout", o.Timeout.String())
	}
	if o.Model != "" {
		args = append(args, "--model", o.Model)
	}
	if o.Verify != "" {
		args = append(args, "--verify", o.Verify, "--retries", strconv.Itoa(o.Retries))
	}
	return args
}
