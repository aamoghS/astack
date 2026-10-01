package app

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"astack/internal/agents"
	"astack/internal/tui"
)

type fakeHost struct {
	keys []tui.Key
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

func (f *fakeHost) Alt(bool) {}

func (f *fakeHost) Write(p []byte) (int, error) {
	return f.out.Write(p)
}

func (f *fakeHost) ReadKey() (tui.Key, error) {
	if f.i >= len(f.keys) {
		return tui.Key{}, io.EOF
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
	if code := (tui.Screen{Host: &fakeHost{tty: false}, Backend: d, Err: d.Err}).Run(tui.Options{}, nil); code != 1 {
		t.Fatal(code)
	}
}

func TestTUIDispatchDryRun(t *testing.T) {
	dir := t.TempDir()
	d := caseDisp(map[string]string{"claude": "/c"})
	keys := []tui.Key{
		{Name: "rune", Rune: 'h'},
		{Name: "rune", Rune: 'i'},
		{Name: "enter"},
		{Name: "ctrl-c"},
	}
	h := &fakeHost{tty: true, keys: keys, w: 80, h: 24}
	o := tui.Options{Workdir: dir, DryRun: true, Agent: "claude"}
	code := tui.Screen{Host: h, Backend: d, Err: d.Err}.Run(o, []string{"claude installed"})
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

func TestWorkerRows(t *testing.T) {
	reg, err := agents.ParseRegistry(embeddedAgents)
	if err != nil {
		t.Fatal(err)
	}
	rows := workerRows(reg, map[string]string{"claude": "/c"})
	if len(rows) == 0 || !strings.Contains(rows[0], "claude") {
		t.Fatal(rows)
	}
}
