package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

type arenaArm struct {
	Agent string `json:"agent"`
	Dir   string `json:"dir"`
	Code  int    `json:"exit"`
	Bytes int64  `json:"bytes"`
}

func pickArenaWinner(arms []arenaArm) (arenaArm, bool) {
	var win arenaArm
	found := false
	for _, a := range arms {
		if a.Code != 0 {
			continue
		}
		if !found || a.Bytes < win.Bytes {
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

	if o.DryRun {
		wall := time.Since(started).Seconds() * 1000
		if d.JSON {
			_ = json.NewEncoder(d.Out).Encode(map[string]any{"arena": ids, "dry_run": true, "wall_ms": wall, "task": false})
			return 0
		}
		fmt.Fprintf(d.Out, "astack arena agents=%v isolated copies; local exec not Cursor Tasks wall_ms=%.2f\n", ids, wall)
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
	win, ok := pickArenaWinner(arms)
	if !ok {
		fmt.Fprintln(d.Err, "astack: arena produced no arms")
		return 1
	}
	wall := time.Since(started).Seconds() * 1000
	if d.JSON {
		_ = json.NewEncoder(d.Out).Encode(map[string]any{"arms": arms, "winner": win, "wall_ms": wall, "task": false, "judge": "smallest-exit0-tree"})
	} else {
		fmt.Fprintf(d.Out, "astack arena winner=%s exit=%d bytes=%d (no LLM judge; smaller tree among exit 0) wall_ms=%.2f\n", win.Agent, win.Code, win.Bytes, wall)
	}
	return win.Code
}
