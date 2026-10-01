package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	tuiMaxLog   = 400
	tuiMinW     = 40
	tuiMinH     = 12
	tuiHelpText = `/help  /list  /clear  /quit
/mode dispatch|playbook|swarm|arena|review|why|architect|explore|reflect
/agent auto|claude|codex|gemini|agy|opencode|grok
/playbook <name>  /n 1-8  /agents a,b  /timeout 5m
/workdir <dir>  /dry-run
/verify <cmd>|off  /model <m>|agent=m,..|off  /judge auto|a,b|off
tab cycles mode  enter runs the worker  ctrl-c quit
astack is the coding agent. it calls claude/codex/gemini/opencode/grok on PATH.`
)

type tuiKey struct {
	Name string
	Rune rune
}

type tuiSession struct {
	mu       sync.Mutex
	workdir  string
	mode     string
	agent    string
	playbook string
	n        int
	agents   string
	timeout  time.Duration
	dryRun   bool
	verify   string
	model    string
	judge    string
	input    string
	history  []string
	histIdx  int
	log      []string
	workers  []string
	running  bool
	status   string
}

// TUIOptions is the starting state of a TUI session. An empty Mode means dispatch.
type TUIOptions struct {
	Workdir  string
	Mode     string
	Agent    string
	Playbook string
	N        int
	Agents   []string
	Timeout  time.Duration
	DryRun   bool
	Verify   string
	Model    string
	Judge    string
}

func newTUISession(o TUIOptions, workers []string) *tuiSession {
	wd := o.Workdir
	if wd == "" {
		wd, _ = os.Getwd()
	}
	mode := o.Mode
	if mode == "" {
		mode = "dispatch"
	}
	agent := o.Agent
	if agent == "" {
		agent = "auto"
	}
	n := o.N
	if n == 0 {
		n = 3
	}
	pb := o.Playbook
	if pb == "" {
		pb = "feature"
	}
	s := &tuiSession{
		workdir:  wd,
		mode:     mode,
		agent:    agent,
		playbook: pb,
		n:        n,
		agents:   strings.Join(o.Agents, ","),
		timeout:  o.Timeout,
		verify:   o.Verify,
		model:    o.Model,
		judge:    o.Judge,
		dryRun:   o.DryRun,
		workers:  workers,
		status:   "ready",
		histIdx:  -1,
	}
	s.append("astack tui  coding agent  runs claude/codex/gemini/opencode/grok if installed")
	s.append("type a task, or /agent claude  /list  /help")
	return s
}

func (s *tuiSession) append(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appendLocked(line)
}

func (s *tuiSession) appendLocked(line string) {
	line = strings.ReplaceAll(line, "\r", "")
	for _, part := range strings.Split(line, "\n") {
		if part == "" && len(s.log) > 0 && s.log[len(s.log)-1] == "" {
			continue
		}
		s.log = append(s.log, part)
	}
	if len(s.log) > tuiMaxLog {
		s.log = s.log[len(s.log)-tuiMaxLog:]
	}
}

func (s *tuiSession) writeLog(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	s.append(string(p))
	return len(p), nil
}

func (s *tuiSession) handle(k tuiKey) (quit, send, redraw bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch k.Name {
	case "ctrl-c", "ctrl-d":
		return true, false, false
	case "esc":
		s.input = ""
		s.histIdx = -1
		return false, false, true
	case "tab":
		s.mode = nextTUIMode(s.mode)
		s.status = "mode " + s.mode
		return false, false, true
	case "up":
		s.historyMove(-1)
		return false, false, true
	case "down":
		s.historyMove(1)
		return false, false, true
	case "backspace":
		if s.input == "" {
			return false, false, true
		}
		_, n := utf8.DecodeLastRuneInString(s.input)
		s.input = s.input[:len(s.input)-n]
		return false, false, true
	case "enter":
		line := strings.TrimRight(s.input, "\r\n")
		s.input = ""
		s.histIdx = -1
		if line == "" {
			return false, false, true
		}
		s.history = append(s.history, line)
		if strings.HasPrefix(line, "/") {
			q, snd := s.applySlash(line)
			return q, snd, true
		}
		if s.running {
			s.appendLocked("astack tui: worker still running")
			return false, false, true
		}
		s.appendLocked("> " + line)
		return false, true, true
	default:
		if k.Rune != 0 && unicode.IsPrint(k.Rune) {
			s.input += string(k.Rune)
			return false, false, true
		}
	}
	return false, false, false
}

func (s *tuiSession) historyMove(delta int) {
	if len(s.history) == 0 {
		return
	}
	if s.histIdx < 0 {
		if delta < 0 {
			s.histIdx = len(s.history) - 1
		} else {
			return
		}
	} else {
		s.histIdx += delta
	}
	if s.histIdx < 0 {
		s.histIdx = 0
	}
	if s.histIdx >= len(s.history) {
		s.histIdx = -1
		s.input = ""
		return
	}
	s.input = s.history[s.histIdx]
}

func (s *tuiSession) applySlash(line string) (quit, send bool) {
	fields := strings.Fields(line)
	cmd := strings.ToLower(fields[0])
	arg := strings.TrimSpace(strings.TrimPrefix(line, fields[0]))
	switch cmd {
	case "/quit", "/q", "/exit":
		return true, false
	case "/help", "/h", "/?":
		s.appendLocked(tuiHelpText)
	case "/clear":
		s.log = nil
		s.status = "cleared"
	case "/list":
		if len(s.workers) == 0 {
			s.appendLocked("no workers in registry")
			break
		}
		s.appendLocked(strings.Join(s.workers, "  "))
	case "/mode":
		if arg == "" {
			s.appendLocked("mode " + s.mode)
			break
		}
		switch arg {
		case "agent":
			s.mode = "dispatch"
			s.status = "mode dispatch (coding agent)"
		case "dispatch", "playbook", "swarm", "arena":
			s.mode = arg
			s.status = "mode " + s.mode
		default:
			if isPanelRole(arg) {
				s.mode = arg
				s.status = "mode " + s.mode + " (read-only panel)"
				break
			}
			s.appendLocked("unknown mode " + arg)
		}
	case "/agent":
		if arg == "" {
			s.appendLocked("agent " + s.agent)
			break
		}
		s.agent = arg
		s.status = "agent " + s.agent
	case "/playbook":
		if arg == "" {
			s.appendLocked("playbook " + s.playbook)
			break
		}
		s.playbook = arg
		s.mode = "playbook"
		s.status = "playbook " + s.playbook
	case "/n":
		n, err := strconv.Atoi(arg)
		if err != nil || n < 1 || n > 8 {
			s.appendLocked("n wants 1-8")
			break
		}
		s.n = n
		s.status = fmt.Sprintf("n=%d", n)
	case "/agents":
		s.agents = strings.ReplaceAll(arg, " ", "")
		if !isPanelRole(s.mode) {
			s.mode = "arena"
		}
		s.status = s.mode + " " + s.agents
	case "/timeout":
		d, err := time.ParseDuration(arg)
		if err != nil {
			s.appendLocked("bad --timeout " + arg)
			break
		}
		s.timeout = d
		s.status = "timeout " + d.String()
	case "/workdir":
		if arg == "" {
			s.appendLocked("workdir " + s.workdir)
			break
		}
		st, err := os.Stat(arg)
		if err != nil || !st.IsDir() {
			s.appendLocked("workdir not found: " + arg)
			break
		}
		abs, err := filepath.Abs(arg)
		if err != nil {
			s.appendLocked(err.Error())
			break
		}
		s.workdir = abs
		s.status = "workdir " + abs
	case "/verify":
		s.verify = tuiSetting(arg, s.verify)
		s.status = "verify " + orNone(s.verify)
	case "/model":
		if arg != "" && arg != "off" {
			if _, err := ParseModelChoice(arg); err != nil {
				s.appendLocked(err.Error())
				break
			}
		}
		s.model = tuiSetting(arg, s.model)
		s.status = "model " + orNone(s.model)
	case "/judge":
		s.judge = tuiSetting(arg, s.judge)
		s.status = "judge " + orNone(s.judge)
	case "/dry-run":
		s.dryRun = !s.dryRun
		s.status = fmt.Sprintf("dry-run=%v", s.dryRun)
	default:
		s.appendLocked("unknown command " + cmd + "  /help")
	}
	return false, false
}

func nextTUIMode(cur string) string {
	switch cur {
	case "dispatch":
		return "playbook"
	case "playbook":
		return "swarm"
	case "swarm":
		return "arena"
	default:
		return "dispatch"
	}
}

func (s *tuiSession) lastPrompt() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := len(s.history) - 1; i >= 0; i-- {
		if !strings.HasPrefix(s.history[i], "/") {
			return s.history[i]
		}
	}
	return ""
}

func (s *tuiSession) argv(promptFile string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var args []string
	switch s.mode {
	case "playbook":
		args = []string{"playbook", "--playbook", s.playbook}
	case "swarm":
		args = []string{"swarm", "--n", strconv.Itoa(s.n)}
	case "arena":
		args = []string{"arena"}
		if s.agents != "" {
			args = append(args, "--agents", s.agents)
		}
		if s.judge != "" {
			args = append(args, "--judge", s.judge)
		}
	default:
		if isPanelRole(s.mode) {
			args = []string{s.mode}
			if s.agents != "" {
				args = append(args, "--agents", s.agents)
			}
		}
	}
	args = append(args, "--workdir", s.workdir, "--prompt-file", promptFile, "--agent", s.agent)
	if s.dryRun {
		args = append(args, "--dry-run")
	}
	if s.timeout > 0 {
		args = append(args, "--timeout", s.timeout.String())
	}
	if s.model != "" {
		args = append(args, "--model", s.model)
	}
	if s.verify != "" && !isPanelRole(s.mode) {
		args = append(args, "--verify", s.verify)
	}
	return args
}

// tuiSetting applies a /verify-style command: no arg keeps the value, "off" clears it.
func tuiSetting(arg, cur string) string {
	switch arg {
	case "":
		return cur
	case "off":
		return ""
	}
	return arg
}

func orNone(v string) string {
	if v == "" {
		return "off"
	}
	return v
}

func (s *tuiSession) setRunning(v bool, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = v
	if status != "" {
		s.status = status
	}
}

func (s *tuiSession) render(w, h int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if w < tuiMinW {
		w = tuiMinW
	}
	if h < tuiMinH {
		h = tuiMinH
	}
	run := "idle"
	if s.running {
		run = "running"
	}
	dry := ""
	if s.dryRun {
		dry = " dry-run"
	}
	head := fmt.Sprintf(" astack  %s  agent=%s  n=%d  pb=%s  %s%s", s.mode, s.agent, s.n, s.playbook, run, dry)
	sub := " " + s.workdir
	if len(s.workers) > 0 {
		sub = " " + strings.Join(s.workers, "  ")
	}
	foot := " tab mode  /help  ctrl-c quit  " + s.status
	prompt := "> " + s.input
	rule := strings.Repeat("─", w)
	inner := h - 6
	if inner < 3 {
		inner = 3
	}
	body := wrapJoin(s.log, w, inner)
	var b strings.Builder
	b.WriteString("\x1b[?25l\x1b[H\x1b[2J")
	b.WriteString(tuiLine(head, w))
	b.WriteByte('\n')
	b.WriteString(tuiLine(sub, w))
	b.WriteByte('\n')
	b.WriteString(rule)
	b.WriteByte('\n')
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString(rule)
	b.WriteByte('\n')
	b.WriteString(tuiLine(prompt+"█", w))
	b.WriteByte('\n')
	b.WriteString(tuiLine(foot, w))
	return b.String()
}

func tuiLine(s string, w int) string {
	if w <= 0 {
		return s
	}
	rs := []rune(s)
	if len(rs) >= w {
		return string(rs[:w])
	}
	return string(rs) + strings.Repeat(" ", w-len(rs))
}

func wrapJoin(lines []string, w, h int) string {
	var wrapped []string
	for _, line := range lines {
		wrapped = append(wrapped, wrapWidth(line, w)...)
	}
	if len(wrapped) > h {
		wrapped = wrapped[len(wrapped)-h:]
	}
	for len(wrapped) < h {
		wrapped = append(wrapped, "")
	}
	for i, line := range wrapped {
		wrapped[i] = tuiLine(line, w)
	}
	return strings.Join(wrapped, "\n")
}

func wrapWidth(s string, w int) []string {
	if w <= 0 {
		return []string{s}
	}
	rs := []rune(s)
	if len(rs) == 0 {
		return []string{""}
	}
	var out []string
	for len(rs) > w {
		out = append(out, string(rs[:w]))
		rs = rs[w:]
	}
	out = append(out, string(rs))
	return out
}
