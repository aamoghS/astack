package app

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"astack/internal/agents"
	"astack/internal/platform"
	"astack/internal/proc"
)

// scriptRunner answers each worker call with reply(agentBin, prompt, cwd).
type scriptRunner struct {
	mu    sync.Mutex
	calls []scriptCall
	reply func(bin, prompt, cwd string) (int, string)
}

type scriptCall struct {
	Bin, Prompt, Cwd string
	Args             []string
}

func (r *scriptRunner) record(bin string, args []string, cwd string) (int, string) {
	prompt := ""
	for i, a := range args {
		if a == "-p" && i+1 < len(args) {
			prompt = args[i+1]
		}
	}
	r.mu.Lock()
	r.calls = append(r.calls, scriptCall{Bin: bin, Prompt: prompt, Cwd: cwd, Args: append([]string(nil), args...)})
	r.mu.Unlock()
	if r.reply == nil {
		return 0, ""
	}
	return r.reply(bin, prompt, cwd)
}

func (r *scriptRunner) Run(bin string, args []string, cwd string) int {
	code, _ := r.record(bin, args, cwd)
	return code
}

func (r *scriptRunner) RunOutput(bin string, args []string, cwd string, stdout, stderr io.Writer) int {
	code, text := r.record(bin, args, cwd)
	io.WriteString(stdout, text)
	return code
}

type fakeVerifier struct {
	results []proc.VerifyResult
	calls   int
}

func (f *fakeVerifier) Verify(command, dir string, timeout time.Duration, log io.Writer) proc.VerifyResult {
	r := f.results[len(f.results)-1]
	if f.calls < len(f.results) {
		r = f.results[f.calls]
	}
	f.calls++
	return r
}

const roleTestConfig = `{
  "order": ["claude", "codex", "gemini"],
  "agents": {
    "claude": {"bin": ["claude"], "args": ["-p", "{{prompt}}", "--sys", "{{footer}}"], "read_args": ["-p", "{{prompt}}", "--plan"], "model_args": ["--model", "{{model}}"]},
    "codex": {"bin": ["codex"], "args": ["-p", "{{prompt}}", "--write"], "read_args": ["-p", "{{prompt}}", "--read-only"], "model_args": ["-m", "{{model}}"]},
    "gemini": {"bin": ["gemini"], "args": ["-p", "{{prompt}}", "--yolo"]}
  },
  "roles": {
    "implement": {"models": {"claude": "opus"}},
    "why": {"agents": ["claude", "codex"]},
    "review": {"agents": ["codex"]},
    "judge": {"agents": ["claude", "codex"]},
    "synth": {"agents": ["claude"]}
  }
}`

func roleDisp(t *testing.T, installed []string, r proc.Runner) (*Dispatcher, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	tmp := t.TempDir()
	for _, k := range []string{"TMP", "TEMP", "TMPDIR"} {
		t.Setenv(k, tmp)
	}
	plat := platform.NewPlatform("linux", platform.HostPaths{})
	look := map[string]string{}
	for _, id := range installed {
		look[id] = "/bin/" + id
	}
	var out, errb bytes.Buffer
	return &Dispatcher{
		Out:      &out,
		Err:      &errb,
		Platform: plat,
		Loader:   testLoader(t, []byte(roleTestConfig)),
		Resolver: agents.BinaryResolver{Platform: plat, Looker: mapLooker{path: look, files: map[string]bool{}}},
		Runner:   r,
		Prompts:  proc.PromptStore{Pid: 7},
	}, &out, &errb
}

func TestParseModelChoice(t *testing.T) {
	mc, err := agents.ParseModelChoice("opus")
	if err != nil || mc[agents.AnyAgent] != "opus" {
		t.Fatal(mc, err)
	}
	mc, err = agents.ParseModelChoice("claude=opus, codex=gpt-5")
	if err != nil || mc["claude"] != "opus" || mc["codex"] != "gpt-5" {
		t.Fatal(mc, err)
	}
	if _, err := agents.ParseModelChoice("claude="); err == nil {
		t.Fatal("want error")
	}
	if _, err := parseArgs([]string{"--model", "=x"}); err == nil {
		t.Fatal("want parse error")
	}
}

func TestParseArgsNewFlags(t *testing.T) {
	o, err := parseArgs([]string{"--verify", "go test ./..."})
	if err != nil || o.Verify != "go test ./..." || o.Retries != 2 {
		t.Fatal(o, err)
	}
	o, err = parseArgs([]string{"--retries", "0", "--verify", "x"})
	if err != nil || o.Retries != 0 {
		t.Fatal(o, err)
	}
	if _, err := parseArgs([]string{"--retries", "11"}); err == nil {
		t.Fatal("want error")
	}
	for _, role := range []string{"review", "explore", "why", "architect", "reflect"} {
		o, err = parseArgs([]string{role, "--workdir", "w"})
		if err != nil || o.Cmd != role {
			t.Fatal(role, o, err)
		}
	}
	o, _ = parseArgs([]string{"arena", "--judge", "auto"})
	if o.Judge != "auto" {
		t.Fatal(o)
	}
	a := strings.Join(flattenDispatch(Flags{Workdir: "w", PromptFile: "p", Agent: "claude", Model: "opus", Verify: "make", Retries: 1}), " ")
	if a != "--workdir w --prompt-file p --agent claude --model opus --verify make --retries 1" {
		t.Fatal(a)
	}
}

func TestVerifyRetriesThenPasses(t *testing.T) {
	r := &scriptRunner{}
	d, _, errb := roleDisp(t, []string{"claude"}, r)
	v := &fakeVerifier{results: []proc.VerifyResult{{Code: 1, Tail: "FAIL TestX"}, {Code: 0}}}
	d.Verifier = v
	code := d.Main([]string{"--workdir", t.TempDir(), "--prompt-file", writePrompt(t), "--verify", "go test", "--model", "sonnet"})
	if code != 0 || len(r.calls) != 2 || v.calls != 2 {
		t.Fatalf("code=%d calls=%d verify=%d err=%s", code, len(r.calls), v.calls, errb.String())
	}
	if strings.Contains(r.calls[0].Prompt, "FAIL TestX") || !strings.Contains(r.calls[1].Prompt, "FAIL TestX") {
		t.Fatal("retry prompt must carry the failure tail")
	}
	if !contains(r.calls[0].Args, "sonnet") {
		t.Fatal("--model not passed", r.calls[0].Args)
	}
}

func TestVerifyGivesUp(t *testing.T) {
	r := &scriptRunner{}
	d, _, _ := roleDisp(t, []string{"claude"}, r)
	d.Verifier = &fakeVerifier{results: []proc.VerifyResult{{Code: 2, Tail: "boom"}}}
	code := d.Main([]string{"--workdir", t.TempDir(), "--prompt-file", writePrompt(t), "--verify", "x", "--retries", "1"})
	if code != proc.ExitVerify || len(r.calls) != 2 {
		t.Fatalf("code=%d calls=%d", code, len(r.calls))
	}
}

func TestVerifySkippedWhenWorkerFails(t *testing.T) {
	r := &scriptRunner{reply: func(string, string, string) (int, string) { return 9, "" }}
	d, _, _ := roleDisp(t, []string{"claude"}, r)
	v := &fakeVerifier{results: []proc.VerifyResult{{Code: 0}}}
	d.Verifier = v
	if code := d.Main([]string{"--workdir", t.TempDir(), "--prompt-file", writePrompt(t), "--verify", "x"}); code != 9 || v.calls != 0 {
		t.Fatalf("code=%d verify calls=%d", code, v.calls)
	}
}

func TestPanelModesAndSynthesis(t *testing.T) {
	r := &scriptRunner{reply: func(bin, prompt, cwd string) (int, string) {
		if strings.Contains(prompt, "Merge them") {
			return 0, "merged answer"
		}
		return 0, "root cause from " + filepath.Base(bin)
	}}
	d, out, _ := roleDisp(t, []string{"claude", "codex"}, r)
	work := t.TempDir()
	code := d.Main([]string{"why", "--workdir", work, "--prompt-file", writePrompt(t)})
	if code != 0 {
		t.Fatal(code)
	}
	s := out.String()
	for _, want := range []string{"why: claude", "why: codex", "root cause from codex", "why synthesis: claude", "merged answer"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in\n%s", want, s)
		}
	}
	for _, c := range r.calls[:2] {
		if c.Cwd != work || !contains(c.Args, "--plan") && !contains(c.Args, "--read-only") {
			t.Fatalf("read-only member should run in place with read args: %+v", c)
		}
	}
	entries, _ := os.ReadDir(work)
	if len(entries) != 0 {
		t.Fatal("panel left files in workdir", entries)
	}
}

func TestPanelIsolatesAgentWithoutReadArgs(t *testing.T) {
	r := &scriptRunner{reply: func(string, string, string) (int, string) { return 0, "ok" }}
	d, _, _ := roleDisp(t, []string{"gemini"}, r)
	work := t.TempDir()
	if err := os.WriteFile(filepath.Join(work, "a.txt"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if code := d.Main([]string{"explore", "--workdir", work, "--prompt-file", writePrompt(t)}); code != 0 {
		t.Fatal(code)
	}
	if len(r.calls) != 1 || r.calls[0].Cwd == work || !contains(r.calls[0].Args, "--yolo") {
		t.Fatalf("gemini must run on an isolated copy: %+v", r.calls)
	}
	if _, err := os.Stat(r.calls[0].Cwd); !os.IsNotExist(err) {
		t.Fatal("isolated copy not removed")
	}
}

func TestPanelDryRunAndExitCodes(t *testing.T) {
	d, out, _ := roleDisp(t, []string{"claude", "gemini"}, &scriptRunner{})
	code := d.Main([]string{"why", "--workdir", t.TempDir(), "--prompt-file", writePrompt(t), "--agents", "claude,gemini", "--model", "claude=opus", "--dry-run"})
	if code != 0 || !strings.Contains(out.String(), "claude(read-only,opus)") || !strings.Contains(out.String(), "gemini(isolated-copy)") {
		t.Fatal(code, out.String())
	}
	d, _, _ = roleDisp(t, nil, &scriptRunner{})
	if code := d.Main([]string{"why", "--workdir", t.TempDir(), "--prompt-file", writePrompt(t)}); code != 3 {
		t.Fatal(code)
	}
	d, _, _ = roleDisp(t, []string{"claude"}, &scriptRunner{reply: func(string, string, string) (int, string) { return 4, "" }})
	if code := d.Main([]string{"why", "--workdir", t.TempDir(), "--prompt-file", writePrompt(t)}); code != 4 {
		t.Fatal(code)
	}
}

func TestReviewNoChange(t *testing.T) {
	d, out, _ := roleDisp(t, []string{"codex"}, &scriptRunner{})
	if code := d.Main([]string{"review", "--workdir", t.TempDir()}); code != 0 || !strings.Contains(out.String(), "no changes") {
		t.Fatal(code, out.String())
	}
}

func TestParseVoteAndWinner(t *testing.T) {
	if k, ok := parseVote("I like 2.\nWINNER: 2", 3); !ok || k != 1 {
		t.Fatal(k, ok)
	}
	if _, ok := parseVote("WINNER: 5", 3); ok {
		t.Fatal("out of range")
	}
	if k, _ := parseVote("winner: 1 ... final WINNER: 3", 3); k != 2 {
		t.Fatal("last vote wins", k)
	}
	w, _ := pickArenaWinner([]arenaArm{
		{Agent: "small", Code: 0, Bytes: 1, Votes: 0},
		{Agent: "voted", Code: 0, Bytes: 9, Votes: 2},
		{Agent: "failed", Code: proc.ExitVerify, Bytes: 0, Votes: 5},
	})
	if w.Agent != "voted" {
		t.Fatal(w)
	}
}

func TestArenaJudgeVotes(t *testing.T) {
	r := &scriptRunner{reply: func(bin, prompt, cwd string) (int, string) {
		if strings.Contains(prompt, "judging") {
			return 0, "second is cleaner\nWINNER: 2"
		}
		if strings.Contains(bin, "codex") {
			os.WriteFile(filepath.Join(cwd, "out.txt"), []byte("codex version"), 0600)
		} else {
			os.WriteFile(filepath.Join(cwd, "out.txt"), []byte("claude version, longer"), 0600)
		}
		return 0, ""
	}}
	d, out, _ := roleDisp(t, []string{"claude", "codex"}, r)
	code := d.Main([]string{"arena", "--agents", "claude,codex", "--judge", "claude", "--workdir", t.TempDir(), "--prompt-file", writePrompt(t)})
	if code != 0 || !strings.Contains(out.String(), "winner=codex") || !strings.Contains(out.String(), "votes=1") {
		t.Fatal(code, out.String())
	}
	var judged string
	for _, c := range r.calls {
		if strings.Contains(c.Prompt, "judging") {
			judged = c.Prompt
		}
	}
	if !strings.Contains(judged, "codex version") || !strings.Contains(judged, "claude version") {
		t.Fatal("judge must see both diffs:\n" + judged)
	}
}

func TestPlaybookNotesReachWorker(t *testing.T) {
	r := &scriptRunner{reply: func(bin, prompt, cwd string) (int, string) {
		if strings.Contains(prompt, "root-cause investigator") {
			return 0, "the bug is in parse.go:12"
		}
		return 0, ""
	}}
	d, _, _ := roleDisp(t, []string{"claude"}, r)
	pb := filepath.Join(t.TempDir(), "pb.json")
	os.WriteFile(pb, []byte(`{"name":"t","steps":[{"id":"w","kind":"why"},{"id":"i","kind":"dispatch"},{"id":"r","kind":"review"}]}`), 0600)
	code := d.Main([]string{"playbook", "--playbook", pb, "--workdir", t.TempDir(), "--prompt-file", writePrompt(t)})
	if code != 0 || len(r.calls) != 2 {
		t.Fatalf("code=%d calls=%d", code, len(r.calls))
	}
	if !strings.Contains(r.calls[1].Prompt, "parse.go:12") || !strings.Contains(r.calls[1].Prompt, "implement this") {
		t.Fatal("dispatch prompt missing notes:\n" + r.calls[1].Prompt)
	}
}
