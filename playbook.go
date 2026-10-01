package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
		switch s.Kind {
		case "dispatch", "review":
		default:
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

func reviewWorkdir(workdir string) string {
	if workdir == "" {
		return ""
	}
	cmd := exec.Command("git", "-C", workdir, "diff", "--stat")
	b, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func (d *Dispatcher) runPlaybook(o Flags, reg *AgentRegistry, resolved map[string]string) int {
	pb, err := loadPlaybook(o.Playbook)
	if err != nil {
		fmt.Fprintln(d.Err, err)
		return 1
	}
	type stepOut struct {
		ID     string `json:"id"`
		Kind   string `json:"kind"`
		Agent  string `json:"agent,omitempty"`
		Action string `json:"action"`
	}
	out := make([]stepOut, 0, len(pb.Steps))
	for _, s := range pb.Steps {
		switch s.Kind {
		case "review":
			if !o.DryRun {
				if stat := reviewWorkdir(o.Workdir); stat != "" && !d.JSON {
					fmt.Fprintln(d.Out, stat)
				}
			}
			out = append(out, stepOut{ID: s.ID, Kind: s.Kind, Action: "conductor-git-diff"})
			if !d.JSON {
				fmt.Fprintf(d.Out, "astack playbook %s/%s: review git diff locally; not a Cursor Task\n", pb.Name, s.ID)
			}
		case "dispatch":
			agent := s.Agent
			if agent == "" {
				agent = "auto"
			}
			pick := reg.Pick(agent, resolved)
			if pick.Code != 0 {
				fmt.Fprintln(d.Err, pick.Err)
				return pick.Code
			}
			out = append(out, stepOut{ID: s.ID, Kind: s.Kind, Agent: pick.Agent.ID, Action: "dispatch"})
			child := o
			child.Cmd = ""
			child.Agent = pick.Agent.ID
			child.Playbook = ""
			child.JSON = false
			code := d.fork().Main(flattenDispatch(child))
			if code != 0 {
				return code
			}
		}
	}
	if d.JSON {
		_ = json.NewEncoder(d.Out).Encode(map[string]any{"playbook": pb.Name, "steps": out, "task": false})
	}
	return 0
}

func flattenDispatch(o Flags) []string {
	args := []string{"--workdir", o.Workdir, "--prompt-file", o.PromptFile, "--agent", o.Agent}
	if o.DryRun {
		args = append(args, "--dry-run")
	}
	if o.Timeout > 0 {
		args = append(args, "--timeout", o.Timeout.String())
	}
	return args
}
