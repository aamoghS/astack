package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type tuiHost interface {
	IsTTY() bool
	Size() (int, int)
	MakeRaw() (func(), error)
	ReadKey() (tuiKey, error)
	Write([]byte) (int, error)
	Alt(on bool)
}

type tuiLogWriter struct {
	s  *tuiSession
	ch chan struct{}
}

func (w tuiLogWriter) Write(p []byte) (int, error) {
	n, err := w.s.writeLog(p)
	select {
	case w.ch <- struct{}{}:
	default:
	}
	return n, err
}

func (d *Dispatcher) runTUI(o Flags, reg *AgentRegistry, resolved map[string]string) int {
	return d.runTUIHost(o, workerRows(reg, resolved), newOSHost(os.Stdin, os.Stdout))
}

func (d *Dispatcher) runTUIHost(o Flags, workers []string, host tuiHost) int {
	if host == nil || !host.IsTTY() {
		fmt.Fprintln(d.Err, "astack tui: need a terminal")
		return 1
	}
	restore, err := host.MakeRaw()
	if err != nil {
		fmt.Fprintf(d.Err, "astack tui: %v\n", err)
		return 1
	}
	defer restore()
	host.Alt(true)
	defer host.Alt(false)

	s := newTUISession(o, workers)
	redraw := make(chan struct{}, 1)
	keys := make(chan tuiKey, 8)
	errCh := make(chan error, 1)
	go func() {
		for {
			k, err := host.ReadKey()
			if err != nil {
				errCh <- err
				return
			}
			keys <- k
		}
	}()

	draw := func() {
		w, h := host.Size()
		_, _ = io.WriteString(host, s.render(w, h))
	}
	draw()

	for {
		select {
		case err := <-errCh:
			if err == io.EOF {
				return 0
			}
			s.append("tui input: " + err.Error())
			return 1
		case k := <-keys:
			quit, send, _ := s.handle(k)
			if quit {
				return 0
			}
			if send {
				d.tuiSend(s, redraw)
			}
			draw()
		case <-redraw:
			draw()
		}
	}
}

func (d *Dispatcher) tuiSend(s *tuiSession, redraw chan struct{}) {
	prompt := s.lastPrompt()
	if prompt == "" {
		return
	}
	s.mu.Lock()
	dir := s.workdir
	s.mu.Unlock()
	if dir == "" {
		s.append("workdir not set")
		return
	}
	pf := filepath.Join(dir, fmt.Sprintf(".astack-prompt.tui.%d.txt", os.Getpid()))
	if err := os.WriteFile(pf, []byte(prompt), 0600); err != nil {
		s.append("cannot write prompt: " + err.Error())
		return
	}
	argv := s.argv(pf)
	limit := 6
	if len(argv) < limit {
		limit = len(argv)
	}
	s.append("dispatch " + fmt.Sprintf("%v", argv[:limit]))
	s.setRunning(true, "dispatching")
	lw := tuiLogWriter{s: s, ch: redraw}
	go func() {
		defer os.Remove(pf)
		child := d.fork()
		child.Out, child.Err = lw, lw
		if pr, ok := child.Runner.(*ProcessRunner); ok {
			cp := *pr
			cp.Stdout, cp.Stderr = lw, lw
			cp.Stdin = nil
			child.Runner = &cp
		}
		started := time.Now()
		code := child.Main(argv)
		ms := time.Since(started).Seconds() * 1000
		s.append(fmt.Sprintf("astack tui exit=%d wall_ms=%.2f", code, ms))
		if code == 0 {
			if stat := reviewWorkdir(dir); stat != "" {
				s.append(stat)
			}
		}
		s.setRunning(false, fmt.Sprintf("exit %d", code))
		select {
		case redraw <- struct{}{}:
		default:
		}
	}()
}
