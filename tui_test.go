package main

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

type fakeHost struct {
	keys []tuiKey
	i    int
	out  bytes.Buffer
	tty  bool
	w, h int
}

func (f *fakeHost) IsTTY() bool { return f.tty }
func (f *fakeHost) Size() (int, int) {
	if f.w == 0 {
		return 80, 24
	}
	return f.w, f.h
}
func (f *fakeHost) MakeRaw() (func(), error) { return func() {}, nil }
func (f *fakeHost) Alt(bool)                 {}
func (f *fakeHost) Write(p []byte) (int, error) {
	return f.out.Write(p)
}
func (f *fakeHost) ReadKey() (tuiKey, error) {
	if f.i >= len(f.keys) {
		return tuiKey{}, io.EOF
	}
	k := f.keys[f.i]
	f.i++
	return k, nil
}

func TestParseArgsTUI(t *testing.T) {
	o, err := parseArgs([]string{"tui", "--workdir", "/repo", "--agent", "claude"})
	if err != nil || o.Cmd != "tui" || o.Workdir != "/repo" || o.Agent != "claude" {
		t.Fatalf("%+v %v", o, err)
	}
}

func TestTUINeedTTY(t *testing.T) {
	d := caseDisp(map[string]string{"claude": "/c"})
	if code := d.runTUIHost(Flags{}, nil, &fakeHost{tty: false}); code != 1 {
		t.Fatal(code)
	}
}

func TestTUISessionSlashAndTab(t *testing.T) {
	s := newTUISession(Flags{Workdir: t.TempDir()}, []string{"claude installed"})
	s.input = "/mode swarm"
	quit, send, _ := s.handle(tuiKey{Name: "enter"})
	if quit || send || s.mode != "swarm" {
		t.Fatalf("mode quit=%v send=%v mode=%s", quit, send, s.mode)
	}
	_, _, _ = s.handle(tuiKey{Name: "tab"})
	if s.mode != "arena" {
		t.Fatal(s.mode)
	}
	s.input = "/n 4"
	_, _, _ = s.handle(tuiKey{Name: "enter"})
	if s.n != 4 {
		t.Fatal(s.n)
	}
	s.input = "/dry-run"
	_, _, _ = s.handle(tuiKey{Name: "enter"})
	if !s.dryRun {
		t.Fatal("dry-run")
	}
	s.input = "/agent gemini"
	_, _, _ = s.handle(tuiKey{Name: "enter"})
	if s.agent != "gemini" {
		t.Fatal(s.agent)
	}
	s.input = "/playbook bug-fix"
	_, _, _ = s.handle(tuiKey{Name: "enter"})
	if s.playbook != "bug-fix" || s.mode != "playbook" {
		t.Fatal(s.playbook, s.mode)
	}
	args := s.argv("/tmp/p")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "playbook") || !strings.Contains(joined, "bug-fix") || !strings.Contains(joined, "--dry-run") {
		t.Fatal(args)
	}
	s.input = "/quit"
	quit, _, _ = s.handle(tuiKey{Name: "enter"})
	if !quit {
		t.Fatal("quit")
	}
}

func TestTUIHistoryAndBackspace(t *testing.T) {
	s := newTUISession(Flags{Workdir: t.TempDir()}, nil)
	for _, r := range []rune("ab") {
		_, _, _ = s.handle(tuiKey{Name: "rune", Rune: r})
	}
	_, _, _ = s.handle(tuiKey{Name: "backspace"})
	if s.input != "a" {
		t.Fatal(s.input)
	}
	_, send, _ := s.handle(tuiKey{Name: "enter"})
	if !send {
		t.Fatal("send")
	}
	_, _, _ = s.handle(tuiKey{Name: "up"})
	if s.input != "a" {
		t.Fatal(s.input)
	}
}

func TestTUIRender(t *testing.T) {
	s := newTUISession(Flags{Workdir: "/tmp/app", Agent: "claude"}, []string{"claude installed"})
	out := s.render(80, 24)
	for _, want := range []string{"astack", "dispatch", "claude", "idle"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %s", want, out)
		}
	}
}

func TestDecodeKeys(t *testing.T) {
	cases := []struct {
		in   string
		name string
		r    rune
	}{
		{"\r", "enter", 0},
		{"\t", "tab", 0},
		{"\x03", "ctrl-c", 0},
		{"\x7f", "backspace", 0},
		{"\x1b[A", "up", 0},
		{"\x1b[B", "down", 0},
		{"x", "rune", 'x'},
	}
	for _, c := range cases {
		k, n, more := decodeKey([]byte(c.in))
		if more || n != len(c.in) || k.Name != c.name || k.Rune != c.r {
			t.Fatalf("%q got %+v n=%d more=%v", c.in, k, n, more)
		}
	}
	_, _, more := decodeKey([]byte{0x1b})
	if !more {
		t.Fatal("lone esc should wait")
	}
}

func TestTUIDispatchDryRun(t *testing.T) {
	dir := t.TempDir()
	d := caseDisp(map[string]string{"claude": "/c"})
	keys := []tuiKey{
		{Name: "rune", Rune: 'h'},
		{Name: "rune", Rune: 'i'},
		{Name: "enter"},
		{Name: "ctrl-c"},
	}
	h := &fakeHost{tty: true, keys: keys, w: 80, h: 24}
	o := Flags{Workdir: dir, DryRun: true, Agent: "claude"}
	code := d.runTUIHost(o, []string{"claude installed"}, h)
	if code != 0 {
		t.Fatal(code)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		out := h.out.String()
		if strings.Contains(out, "dispatch") || strings.Contains(out, "dry-run") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("tui output missing dispatch: %q", h.out.String())
}

func TestWrapWidth(t *testing.T) {
	got := wrapWidth("abcdef", 3)
	if strings.Join(got, ",") != "abc,def" {
		t.Fatal(got)
	}
}

func TestNextTUIMode(t *testing.T) {
	if nextTUIMode("dispatch") != "playbook" || nextTUIMode("arena") != "dispatch" {
		t.Fatal("cycle")
	}
}

func TestWorkerRows(t *testing.T) {
	reg, err := ParseRegistry(embeddedAgents)
	if err != nil {
		t.Fatal(err)
	}
	rows := workerRows(reg, map[string]string{"claude": "/c"})
	if len(rows) == 0 || !strings.Contains(rows[0], "claude") {
		t.Fatal(rows)
	}
}

func TestTUIPanelModeAndSettings(t *testing.T) {
	s := newTUISession(Flags{Workdir: "/w"}, nil)
	for _, line := range []string{"/mode why", "/agents claude,codex", "/model claude=opus", "/verify go test ./..."} {
		s.input = line
		s.handle(tuiKey{Name: "enter"})
	}
	got := strings.Join(s.argv("p.txt"), " ")
	if got != "why --agents claude,codex --workdir /w --prompt-file p.txt --agent auto --model claude=opus" {
		t.Fatal(got)
	}
	for _, line := range []string{"/mode arena", "/judge auto", "/verify off"} {
		s.input = line
		s.handle(tuiKey{Name: "enter"})
	}
	got = strings.Join(s.argv("p.txt"), " ")
	if got != "arena --agents claude,codex --judge auto --workdir /w --prompt-file p.txt --agent auto --model claude=opus" {
		t.Fatal(got)
	}
	s.input = "/mode dispatch"
	s.handle(tuiKey{Name: "enter"})
	s.input = "/verify make test"
	s.handle(tuiKey{Name: "enter"})
	if got = strings.Join(s.argv("p.txt"), " "); !strings.HasSuffix(got, "--verify make test") {
		t.Fatal(got)
	}
}
