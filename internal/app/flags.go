package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"astack/internal/agents"
)

// Flags is one parsed astack command line.
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
	Verify     string
	Retries    int
	Model      string
	Judge      string
}

func usageText() string {
	return "usage: astack [tui|agent|playbook|swarm|arena|" + strings.Join(agents.PanelRoleNames(), "|") + "] --workdir <repo> --prompt-file <file> " +
		"[--agent auto] [--model m|agent=m,...] [--verify \"<cmd>\"] [--retries 2] [--judge auto|a,b] [--n 3] [--agents claude,codex] " +
		"[--playbook file] [--list] [--bench] [--json] [--dry-run] [--timeout 0s]"
}

func parseArgs(argv []string) (Flags, error) {
	var o Flags
	retriesSet := false
	if len(argv) > 0 && !strings.HasPrefix(argv[0], "-") {
		switch argv[0] {
		case "playbook", "swarm", "arena", "tui", "agent":
			o.Cmd = argv[0]
			argv = argv[1:]
		default:
			if agents.IsPanelRole(argv[0]) {
				o.Cmd = argv[0]
				argv = argv[1:]
			}
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
		case "--verify":
			v, err := need()
			if err != nil {
				return o, err
			}
			o.Verify = v
		case "--retries":
			v, err := need()
			if err != nil {
				return o, err
			}
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 || n > 10 {
				return o, fmt.Errorf("bad --retries %s (want 0-10)", v)
			}
			o.Retries = n
			retriesSet = true
		case "--model":
			v, err := need()
			if err != nil {
				return o, err
			}
			if _, err := agents.ParseModelChoice(v); err != nil {
				return o, err
			}
			o.Model = v
		case "--judge":
			v, err := need()
			if err != nil {
				return o, err
			}
			o.Judge = v
		default:
			return o, fmt.Errorf("unknown flag %s", a)
		}
	}
	if o.Verify != "" && !retriesSet {
		o.Retries = 2
	}
	return o, nil
}
