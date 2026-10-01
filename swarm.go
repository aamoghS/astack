package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

func (d *Dispatcher) runSwarm(o Flags, reg *AgentRegistry, resolved map[string]string) int {
	started := time.Now()
	n := o.N
	if n == 0 {
		n = 3
	}
	if o.Workdir == "" || o.PromptFile == "" {
		fmt.Fprintln(d.Err, usageText())
		return 1
	}
	pick := reg.Pick(orAuto(o.Agent), resolved)
	if pick.Code != 0 {
		fmt.Fprintln(d.Err, pick.Err)
		return pick.Code
	}

	type arm struct {
		I    int    `json:"i"`
		Dir  string `json:"dir"`
		Code int    `json:"exit"`
	}
	if o.DryRun {
		wall := time.Since(started).Seconds() * 1000
		if d.JSON {
			_ = json.NewEncoder(d.Out).Encode(map[string]any{
				"swarm": n, "worker": pick.Agent.ID, "dry_run": true, "wall_ms": wall, "task": false,
			})
			return 0
		}
		fmt.Fprintf(d.Out, "astack swarm n=%d worker=%s isolated workdirs; not Cursor Tasks wall_ms=%.2f\n", n, pick.Agent.ID, wall)
		return 0
	}

	lw := &lockedWriter{w: d.Out}
	le := &lockedWriter{w: d.Err}
	arms := make([]arm, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			dir, err := isolateWorkdir(o.Workdir)
			if err != nil {
				arms[i] = arm{I: i, Code: 1}
				return
			}
			child := o
			child.Cmd = ""
			child.N = 0
			child.Workdir = dir
			child.Agent = pick.Agent.ID
			child.JSON = false
			fd := d.fork()
			fd.Out, fd.Err = lw, le
			code := fd.Main(flattenDispatch(child))
			arms[i] = arm{I: i, Dir: dir, Code: code}
		}(i)
	}
	wg.Wait()
	wall := time.Since(started).Seconds() * 1000
	if d.JSON {
		_ = json.NewEncoder(d.Out).Encode(map[string]any{"swarm": n, "worker": pick.Agent.ID, "arms": arms, "wall_ms": wall, "task": false})
	} else {
		for _, a := range arms {
			fmt.Fprintf(d.Out, "astack swarm[%d] exit=%d dir=%s\n", a.I, a.Code, a.Dir)
		}
		fmt.Fprintf(d.Err, "astack swarm wall_ms=%.2f n=%d (OS goroutines, not Cursor Tasks)\n", wall, n)
	}
	for _, a := range arms {
		if a.Code != 0 {
			return a.Code
		}
	}
	return 0
}

func orAuto(s string) string {
	if s == "" {
		return "auto"
	}
	return s
}
