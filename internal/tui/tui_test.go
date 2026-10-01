package tui

import (
	"strings"
	"testing"
)

func TestTUISessionSlashAndTab(t *testing.T) {
	s := NewSession(Options{Workdir: t.TempDir()}, []string{"claude installed"})
	s.input = "/mode swarm"
	quit, send, _ := s.handle(Key{Name: "enter"})
	if quit || send || s.mode != "swarm" {
		t.Fatalf("mode quit=%v send=%v mode=%s", quit, send, s.mode)
	}
	_, _, _ = s.handle(Key{Name: "tab"})
	if s.mode != "arena" {
		t.Fatal(s.mode)
	}
	s.input = "/n 4"
	_, _, _ = s.handle(Key{Name: "enter"})
	if s.n != 4 {
		t.Fatal(s.n)
	}
	s.input = "/dry-run"
	_, _, _ = s.handle(Key{Name: "enter"})
	if !s.dryRun {
		t.Fatal("dry-run")
	}
	s.input = "/agent gemini"
	_, _, _ = s.handle(Key{Name: "enter"})
	if s.agent != "gemini" {
		t.Fatal(s.agent)
	}
	s.input = "/playbook bug-fix"
	_, _, _ = s.handle(Key{Name: "enter"})
	if s.playbook != "bug-fix" || s.mode != "playbook" {
		t.Fatal(s.playbook, s.mode)
	}
	args := s.argv("/tmp/p")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "playbook") || !strings.Contains(joined, "bug-fix") || !strings.Contains(joined, "--dry-run") {
		t.Fatal(args)
	}
	s.input = "/quit"
	quit, _, _ = s.handle(Key{Name: "enter"})
	if !quit {
		t.Fatal("quit")
	}
}

func TestTUIHistoryAndBackspace(t *testing.T) {
	s := NewSession(Options{Workdir: t.TempDir()}, nil)
	for _, r := range []rune("ab") {
		_, _, _ = s.handle(Key{Name: "rune", Rune: r})
	}
	_, _, _ = s.handle(Key{Name: "backspace"})
	if s.input != "a" {
		t.Fatal(s.input)
	}
	_, send, _ := s.handle(Key{Name: "enter"})
	if !send {
		t.Fatal("send")
	}
	_, _, _ = s.handle(Key{Name: "up"})
	if s.input != "a" {
		t.Fatal(s.input)
	}
}

func TestTUIRender(t *testing.T) {
	s := NewSession(Options{Workdir: "/tmp/app", Agent: "claude"}, []string{"claude installed"})
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

func TestWrapWidth(t *testing.T) {
	got := wrapWidth("abcdef", 3)
	if strings.Join(got, ",") != "abc,def" {
		t.Fatal(got)
	}
}

func TestNextTUIMode(t *testing.T) {
	if NextMode("dispatch") != "playbook" || NextMode("arena") != "dispatch" {
		t.Fatal("cycle")
	}
}

func TestTUIPanelModeAndSettings(t *testing.T) {
	s := NewSession(Options{Workdir: "/w"}, nil)
	for _, line := range []string{"/mode why", "/agents claude,codex", "/model claude=opus", "/verify go test ./..."} {
		s.input = line
		s.handle(Key{Name: "enter"})
	}
	got := strings.Join(s.argv("p.txt"), " ")
	if got != "why --agents claude,codex --workdir /w --prompt-file p.txt --agent auto --model claude=opus" {
		t.Fatal(got)
	}
	for _, line := range []string{"/mode arena", "/judge auto", "/verify off"} {
		s.input = line
		s.handle(Key{Name: "enter"})
	}
	got = strings.Join(s.argv("p.txt"), " ")
	if got != "arena --agents claude,codex --judge auto --workdir /w --prompt-file p.txt --agent auto --model claude=opus" {
		t.Fatal(got)
	}
	s.input = "/mode dispatch"
	s.handle(Key{Name: "enter"})
	s.input = "/verify make test"
	s.handle(Key{Name: "enter"})
	if got = strings.Join(s.argv("p.txt"), " "); !strings.HasSuffix(got, "--verify make test") {
		t.Fatal(got)
	}
}
