package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type arenaArm struct {
	Agent string `json:"agent"`
	Dir   string `json:"dir"`
	Code  int    `json:"exit"`
	Bytes int64  `json:"bytes"`
	Votes int    `json:"votes"`
}

// pickArenaWinner ranks exit-0 arms (exit 0 means the worker finished and,
// with --verify, the checks passed): most judge votes first, then the
// smallest tree. With no exit-0 arm it returns the first arm.
func pickArenaWinner(arms []arenaArm) (arenaArm, bool) {
	var win arenaArm
	found := false
	for _, a := range arms {
		if a.Code != 0 {
			continue
		}
		if !found || a.Votes > win.Votes || (a.Votes == win.Votes && a.Bytes < win.Bytes) {
			win = a
			found = true
		}
	}
	if found {
		return win, true
	}
	if len(arms) == 0 {
		return arenaArm{}, false
	}
	return arms[0], true
}

// ArenaJudge asks one or more CLIs which passing arm solved the task best.
// Each judge casts one vote; votes break ties before tree size does.
type ArenaJudge struct {
	d        *Dispatcher
	Members  []Agent
	Registry *AgentRegistry
	Resolved map[string]string
	Models   ModelChoice
}

var winnerLine = regexp.MustCompile(`(?i)WINNER:\s*(\d+)`)

// parseVote reads the last `WINNER: n` line; n is 1-based and must be in range.
func parseVote(text string, n int) (int, bool) {
	m := winnerLine.FindAllStringSubmatch(text, -1)
	if len(m) == 0 {
		return 0, false
	}
	k, err := strconv.Atoi(m[len(m)-1][1])
	if err != nil || k < 1 || k > n {
		return 0, false
	}
	return k - 1, true
}

// Vote fills arms[i].Votes for the candidate indexes and returns the replies.
func (j ArenaJudge) Vote(task, workdir string, arms []arenaArm, candidates []int) PanelOutcome {
	var b strings.Builder
	fmt.Fprintf(&b, "You are judging %d candidate solutions to the same coding task. Each one finished without error", len(candidates))
	b.WriteString(" (and passed the verify command when one was given). Pick the candidate that solves the task most correctly, ")
	b.WriteString("with the smallest safe change and the code a maintainer would rather keep. Explain briefly, then end with one line: `WINNER: <number>`.\n\n")
	b.WriteString("Task:\n" + strings.TrimSpace(task))
	for k, i := range candidates {
		fmt.Fprintf(&b, "\n\n### Candidate %d\n```diff\n%s\n```", k+1, TreeChange(workdir, arms[i].Dir))
	}
	out := PanelOutcome{Role: RoleJudge, Replies: j.d.newPanel(RoleJudge, j.Members, j.Registry, j.Resolved, j.Models).Run(b.String(), "")}
	for _, r := range out.Replies {
		if k, ok := parseVote(r.Text, len(candidates)); ok && r.Exit == 0 {
			arms[candidates[k]].Votes++
		}
	}
	return out
}

func (d *Dispatcher) runArena(o Flags, reg *AgentRegistry, resolved map[string]string) int {
	started := time.Now()
	if o.Workdir == "" || o.PromptFile == "" {
		fmt.Fprintln(d.Err, usageText())
		return 1
	}
	ids := o.Agents
	if len(ids) == 0 {
		for _, id := range reg.Order {
			if resolved[id] != "" {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) < 2 {
		fmt.Fprintln(d.Err, "astack: arena needs at least two agents; not rerouting.")
		return 1
	}
	for _, id := range ids {
		p := reg.Pick(id, resolved)
		if p.Code != 0 {
			fmt.Fprintln(d.Err, p.Err)
			return p.Code
		}
	}
	judges, code := d.arenaJudges(o, reg, resolved)
	if code != 0 {
		return code
	}

	if o.DryRun {
		wall := time.Since(started).Seconds() * 1000
		if d.JSON {
			_ = json.NewEncoder(d.Out).Encode(map[string]any{"arena": ids, "judges": agentIDs(judges), "dry_run": true, "wall_ms": wall, "task": false})
			return 0
		}
		fmt.Fprintf(d.Out, "astack arena agents=%v judges=%v isolated copies; local exec not Cursor Tasks wall_ms=%.2f\n", ids, agentIDs(judges), wall)
		return 0
	}

	lw := &lockedWriter{w: d.Out}
	le := &lockedWriter{w: d.Err}
	arms := make([]arenaArm, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			dir, err := isolateWorkdir(o.Workdir)
			if err != nil {
				arms[i] = arenaArm{Agent: id, Code: 1}
				return
			}
			child := o
			child.Cmd = ""
			child.Agents = nil
			child.Judge = ""
			child.Workdir = dir
			child.Agent = id
			child.JSON = false
			fd := d.fork()
			fd.Out, fd.Err = lw, le
			code := fd.Main(flattenDispatch(child))
			arms[i] = arenaArm{Agent: id, Dir: dir, Code: code, Bytes: treeBytes(dir)}
		}(i, id)
	}
	wg.Wait()

	judgeMode := "smallest-exit0-tree"
	var verdict *PanelOutcome
	var passing []int
	for i, a := range arms {
		if a.Code == 0 {
			passing = append(passing, i)
		}
	}
	if len(judges) > 0 && len(passing) >= 2 {
		task, _ := os.ReadFile(o.PromptFile)
		models, _ := ParseModelChoice(o.Model)
		out := ArenaJudge{d: d, Members: judges, Registry: reg, Resolved: resolved, Models: models}.Vote(string(task), o.Workdir, arms, passing)
		verdict = &out
		judgeMode = "llm-vote-then-smallest-tree"
	}
	if o.Verify != "" {
		judgeMode = "verify-pass-then-" + judgeMode
	}

	win, ok := pickArenaWinner(arms)
	if !ok {
		fmt.Fprintln(d.Err, "astack: arena produced no arms")
		return 1
	}
	wall := time.Since(started).Seconds() * 1000
	if d.JSON {
		_ = json.NewEncoder(d.Out).Encode(map[string]any{"arms": arms, "winner": win, "wall_ms": wall, "task": false, "judge": judgeMode, "verdict": verdict})
	} else {
		if verdict != nil {
			verdict.Print(d.Out)
		}
		fmt.Fprintf(d.Out, "astack arena winner=%s exit=%d votes=%d bytes=%d dir=%s judge=%s wall_ms=%.2f\n", win.Agent, win.Code, win.Votes, win.Bytes, win.Dir, judgeMode, wall)
	}
	return win.Code
}

// arenaJudges resolves --judge: "" means no LLM judge, "auto" means the
// judge role, anything else is a comma list of agents.
func (d *Dispatcher) arenaJudges(o Flags, reg *AgentRegistry, resolved map[string]string) ([]Agent, int) {
	if o.Judge == "" {
		return nil, 0
	}
	var explicit []string
	if o.Judge != "auto" {
		for _, id := range strings.Split(o.Judge, ",") {
			if id = strings.TrimSpace(id); id != "" {
				explicit = append(explicit, id)
			}
		}
	}
	members, pick := reg.Members(RoleJudge, explicit, resolved)
	if pick.Code != 0 {
		fmt.Fprintln(d.Err, pick.Err)
		return nil, pick.Code
	}
	return members, 0
}

func agentIDs(as []Agent) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = a.ID
	}
	return out
}
