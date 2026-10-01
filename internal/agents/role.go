package agents

import (
	"fmt"
	"sort"
	"strings"
)

// Role is one job in agents.json "roles": which CLIs sit on its panel and
// which model each one runs. A missing role falls back to the first
// installed worker on its default model.
type Role struct {
	Agents []string          `json:"agents"`
	Models map[string]string `json:"models"`
}

const (
	RoleImplement = "implement"
	RoleReview    = "review"
	RoleExplore   = "explore"
	RoleWhy       = "why"
	RoleArchitect = "architect"
	RoleReflect   = "reflect"
	RoleJudge     = "judge"
	RoleSynth     = "synth"
)

// panelRoles are the read-only subcommands. Each one is a prompt framing.
var RolePrompts = map[string]string{
	RoleReview: "You are a code reviewer. Review the change below for correctness bugs, security problems, and missing tests. " +
		"Report each finding on one line as `file:line: severity: problem. fix.` Reply LGTM if you find nothing real.",
	RoleExplore: "You are a codebase explorer. Answer the question by reading the code. Cite file:line for every claim.",
	RoleWhy: "You are a root-cause investigator. Find why the behavior below happens. " +
		"Give the root cause with evidence (file:line, commands you ran, output). Separate what you verified from what you suspect.",
	RoleArchitect: "You are a software architect. Propose an implementation plan for the task below: " +
		"files to change, the approach, alternatives you rejected and why, risks, and how to verify it.",
	RoleReflect: "You are a critic. Look at the task and the current state of the repo. " +
		"Say what is wrong or missing, what is risky, and the single most useful next step.",
}

func IsPanelRole(name string) bool {
	_, ok := RolePrompts[name]
	return ok
}

func PanelRoleNames() []string {
	names := make([]string, 0, len(RolePrompts))
	for n := range RolePrompts {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func ReadOnlyFooter(role string) string {
	return "You are the astack " + role + " agent, run by a conductor. Do not create, edit, or delete any file. " +
		"Do not commit. Reply in plain text; your reply is the whole deliverable."
}

// ModelChoice is the parsed --model flag: one model for every agent
// ("opus") or one per agent ("claude=opus,codex=gpt-5").
type ModelChoice map[string]string

const AnyAgent = "*"

func ParseModelChoice(v string) (ModelChoice, error) {
	mc := ModelChoice{}
	v = strings.TrimSpace(v)
	if v == "" {
		return mc, nil
	}
	if !strings.Contains(v, "=") {
		mc[AnyAgent] = v
		return mc, nil
	}
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, m, ok := strings.Cut(part, "=")
		k, m = strings.TrimSpace(k), strings.TrimSpace(m)
		if !ok || k == "" || m == "" {
			return nil, fmt.Errorf("bad --model %s (want model or agent=model,...)", v)
		}
		mc[k] = m
	}
	return mc, nil
}

// Model picks the model for agent id in role: --model first, then the role config.
func (r *Registry) Model(role, id string, override ModelChoice) string {
	if m := override[id]; m != "" {
		return m
	}
	if m := override[AnyAgent]; m != "" {
		return m
	}
	return r.Roles[role].Models[id]
}

// Members picks the panel for a role. Explicit names must all be installed
// (exit 2 otherwise). Otherwise the installed agents listed for the role run,
// or the first installed worker when none of them are present (exit 3 if none).
func (r *Registry) Members(role string, explicit []string, resolved map[string]string) ([]Agent, PickResult) {
	if len(explicit) > 0 {
		out := make([]Agent, 0, len(explicit))
		for _, id := range explicit {
			p := r.Pick(id, resolved)
			if p.Code != 0 {
				return nil, p
			}
			out = append(out, p.Agent)
		}
		return out, PickResult{}
	}
	var out []Agent
	for _, id := range r.Roles[role].Agents {
		if resolved[id] != "" {
			out = append(out, r.ByID[id])
		}
	}
	if len(out) > 0 {
		return out, PickResult{}
	}
	p := r.Pick("auto", resolved)
	if p.Code != 0 {
		return nil, p
	}
	return []Agent{p.Agent}, PickResult{}
}
