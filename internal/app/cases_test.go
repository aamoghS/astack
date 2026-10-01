package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"astack/internal/agents"
	"astack/internal/platform"
	"astack/internal/proc"
	"astack/internal/workspace"
)

type case300 struct {
	name string
	run  func(*testing.T)
}

func TestUnique300(t *testing.T) {
	cs := buildUniqueCases()
	if len(cs) != 300 {
		t.Fatalf("unique cases: got %d want 300", len(cs))
	}
	seen := map[string]bool{}
	for _, c := range cs {
		if c.name == "" || seen[c.name] {
			t.Fatalf("duplicate or empty name %q", c.name)
		}
		seen[c.name] = true
		t.Run(c.name, c.run)
	}
}

func caseDisp(look map[string]string) *Dispatcher {
	plat := platform.NewPlatform("linux", platform.HostPaths{})
	return &Dispatcher{
		Out:      &strings.Builder{},
		Err:      &strings.Builder{},
		Platform: plat,
		Loader:   &agents.ConfigLoader{Embed: embeddedAgents, Env: func(string) string { return "" }, Cwd: os.Getwd, Exe: func() (string, error) { return "/tmp/go-build/x", nil }, Read: os.ReadFile},
		Resolver: agents.BinaryResolver{Platform: plat, Looker: mapLooker{path: look, files: map[string]bool{}}},
		Runner:   &recordRunner{},
		Prompts:  proc.PromptStore{Pid: 1},
	}
}

func buildUniqueCases() []case300 {
	var cs []case300
	add := func(name string, fn func(*testing.T)) {
		cs = append(cs, case300{name: name, run: fn})
	}

	add("parse/help", func(t *testing.T) {
		o, err := parseArgs([]string{"--help"})
		if err != nil || !o.Help {
			t.Fatal(err, o)
		}
	})
	add("parse/list", func(t *testing.T) {
		o, err := parseArgs([]string{"--list"})
		if err != nil || !o.List {
			t.Fatal(err)
		}
	})
	add("parse/bench", func(t *testing.T) {
		o, err := parseArgs([]string{"--bench"})
		if err != nil || !o.Bench {
			t.Fatal(err)
		}
	})
	add("parse/json", func(t *testing.T) {
		o, err := parseArgs([]string{"--json"})
		if err != nil || !o.JSON {
			t.Fatal(err)
		}
	})
	add("parse/dry-run", func(t *testing.T) {
		o, err := parseArgs([]string{"--dry-run"})
		if err != nil || !o.DryRun {
			t.Fatal(err)
		}
	})
	add("parse/cmd/playbook", func(t *testing.T) {
		o, err := parseArgs([]string{"playbook", "--playbook", "x.json"})
		if err != nil || o.Cmd != "playbook" || o.Playbook != "x.json" {
			t.Fatalf("%+v %v", o, err)
		}
	})
	add("parse/cmd/swarm", func(t *testing.T) {
		o, err := parseArgs([]string{"swarm", "--n", "4"})
		if err != nil || o.Cmd != "swarm" || o.N != 4 {
			t.Fatalf("%+v %v", o, err)
		}
	})
	add("parse/cmd/arena", func(t *testing.T) {
		o, err := parseArgs([]string{"arena", "--agents", "claude,codex"})
		if err != nil || o.Cmd != "arena" || len(o.Agents) != 2 {
			t.Fatalf("%+v %v", o, err)
		}
	})
	add("parse/agents/spaces", func(t *testing.T) {
		o, err := parseArgs([]string{"--agents", " claude , gemini "})
		if err != nil || len(o.Agents) != 2 || o.Agents[1] != "gemini" {
			t.Fatalf("%+v %v", o, err)
		}
	})
	add("parse/unknown-flag", func(t *testing.T) {
		if _, err := parseArgs([]string{"--nope"}); err == nil {
			t.Fatal("expected error")
		}
	})
	add("parse/timeout-missing", func(t *testing.T) {
		if _, err := parseArgs([]string{"--timeout"}); err == nil {
			t.Fatal("expected error")
		}
	})
	add("parse/n-missing", func(t *testing.T) {
		if _, err := parseArgs([]string{"--n"}); err == nil {
			t.Fatal("expected error")
		}
	})
	add("parse/n-zero", func(t *testing.T) {
		if _, err := parseArgs([]string{"--n", "0"}); err == nil {
			t.Fatal("expected error")
		}
	})
	add("parse/n-nine", func(t *testing.T) {
		if _, err := parseArgs([]string{"--n", "9"}); err == nil {
			t.Fatal("expected error")
		}
	})
	add("parse/timeout-bad", func(t *testing.T) {
		if _, err := parseArgs([]string{"--timeout", "zzz"}); err == nil {
			t.Fatal("expected error")
		}
	})
	add("parse/agent-missing", func(t *testing.T) {
		if _, err := parseArgs([]string{"--agent"}); err == nil {
			t.Fatal("expected error")
		}
	})
	add("parse/workdir-missing", func(t *testing.T) {
		if _, err := parseArgs([]string{"--workdir"}); err == nil {
			t.Fatal("expected error")
		}
	})
	add("parse/prompt-file-missing", func(t *testing.T) {
		if _, err := parseArgs([]string{"--prompt-file"}); err == nil {
			t.Fatal("expected error")
		}
	})
	add("parse/playbook-missing", func(t *testing.T) {
		if _, err := parseArgs([]string{"--playbook"}); err == nil {
			t.Fatal("expected error")
		}
	})
	add("parse/agents-missing", func(t *testing.T) {
		if _, err := parseArgs([]string{"--agents"}); err == nil {
			t.Fatal("expected error")
		}
	})
	add("parse/usage-mentions-modes", func(t *testing.T) {
		u := usageText()
		for _, s := range []string{"playbook", "swarm", "arena"} {
			if !strings.Contains(u, s) {
				t.Fatal(u)
			}
		}
	})

	for _, dur := range []string{"1ns", "1us", "1ms", "1s", "1m", "1h", "2h", "500ms", "0s", "15m"} {
		d := dur
		add("parse/timeout/"+d, func(t *testing.T) {
			o, err := parseArgs([]string{"--timeout", d})
			if err != nil {
				t.Fatal(err)
			}
			want, err := time.ParseDuration(d)
			if err != nil || o.Timeout != want {
				t.Fatalf("timeout %s got %v", d, o.Timeout)
			}
		})
	}
	for n := 1; n <= 8; n++ {
		n := n
		add(fmt.Sprintf("parse/n/%d", n), func(t *testing.T) {
			o, err := parseArgs([]string{"--n", fmt.Sprint(n)})
			if err != nil || o.N != n {
				t.Fatal(err, o.N)
			}
		})
	}
	for _, a := range []string{"claude", "agy", "codex", "gemini", "opencode", "auto"} {
		a := a
		add("parse/agent/"+a, func(t *testing.T) {
			o, err := parseArgs([]string{"--agent", a})
			if err != nil || o.Agent != a {
				t.Fatal(err)
			}
		})
	}

	reg, err := agents.ParseRegistry(embeddedAgents)
	if err != nil {
		panic(err)
	}
	add("registry/embedded-ok", func(t *testing.T) {
		if len(reg.Order) < 4 {
			t.Fatal(reg.Order)
		}
	})
	add("playbook/default", func(t *testing.T) {
		p, err := ParsePlaybook([]byte(defaultPlaybookJSON))
		if err != nil || p.Name != "feature" || len(p.Steps) != 2 {
			t.Fatal(err, p)
		}
	})
	add("playbook/missing-name", func(t *testing.T) {
		if _, err := ParsePlaybook([]byte(`{"steps":[{"id":"a","kind":"review"}]}`)); err == nil {
			t.Fatal("expected error")
		}
	})
	add("playbook/no-steps", func(t *testing.T) {
		if _, err := ParsePlaybook([]byte(`{"name":"x"}`)); err == nil {
			t.Fatal("expected error")
		}
	})
	add("playbook/bad-kind", func(t *testing.T) {
		if _, err := ParsePlaybook([]byte(`{"name":"x","steps":[{"id":"a","kind":"task"}]}`)); err == nil {
			t.Fatal("expected error")
		}
	})
	add("playbook/missing-id", func(t *testing.T) {
		if _, err := ParsePlaybook([]byte(`{"name":"x","steps":[{"kind":"review"}]}`)); err == nil {
			t.Fatal("expected error")
		}
	})
	add("playbook/bad-json", func(t *testing.T) {
		if _, err := ParsePlaybook([]byte(`{`)); err == nil {
			t.Fatal("expected error")
		}
	})
	add("playbook/empty-steps", func(t *testing.T) {
		if _, err := ParsePlaybook([]byte(`{"name":"x","steps":[]}`)); err == nil {
			t.Fatal("expected error")
		}
	})
	add("playbook/review-only", func(t *testing.T) {
		p, err := ParsePlaybook([]byte(`{"name":"r","steps":[{"id":"rev","kind":"review"}]}`))
		if err != nil || p.Steps[0].Kind != "review" {
			t.Fatal(err)
		}
	})
	add("playbook/dispatch-agent", func(t *testing.T) {
		p, err := ParsePlaybook([]byte(`{"name":"d","steps":[{"id":"im","kind":"dispatch","agent":"codex"}]}`))
		if err != nil || p.Steps[0].Agent != "codex" {
			t.Fatal(err)
		}
	})
	add("playbook/unknown-kind-office-hours", func(t *testing.T) {
		if _, err := ParsePlaybook([]byte(`{"name":"x","steps":[{"id":"a","kind":"office-hours"}]}`)); err == nil {
			t.Fatal("expected error")
		}
	})
	add("playbook/load-missing-file", func(t *testing.T) {
		if _, err := loadPlaybook(filepath.Join(t.TempDir(), "nope.json")); err == nil {
			t.Fatal("expected error")
		}
	})

	for _, name := range []string{"feature", "bug-fix", "refactoring", "perf-issue", "hillclimb"} {
		name := name
		add("playbook/named/"+name, func(t *testing.T) {
			p, err := loadPlaybook(name)
			if err != nil || p.Name != name {
				t.Fatal(err, p)
			}
		})
	}

	pbNames := []string{"feature", "bug-fix", "refactoring", "perf-issue", "hillclimb"}
	pbKinds := []string{"dispatch", "review"}
	pbAgents := []string{"auto", "claude", "agy", "codex", "gemini"}
	for _, n := range pbNames {
		for _, k := range pbKinds {
			for _, a := range pbAgents {
				n, k, a := n, k, a
				add(fmt.Sprintf("playbook/parse/%s/%s/%s", n, k, a), func(t *testing.T) {
					js := fmt.Sprintf(`{"name":%q,"steps":[{"id":"s","kind":%q,"agent":%q}]}`, n, k, a)
					p, err := ParsePlaybook([]byte(js))
					if err != nil || p.Name != n || p.Steps[0].Kind != k {
						t.Fatal(err, p)
					}
					if p.Steps[0].Agent != a {
						t.Fatal(p.Steps[0].Agent)
					}
				})
			}
		}
	}

	add("arena/winner/empty", func(t *testing.T) {
		if _, ok := pickArenaWinner(nil); ok {
			t.Fatal("empty")
		}
	})
	add("arena/winner/all-fail-first", func(t *testing.T) {
		w, ok := pickArenaWinner([]arenaArm{{Agent: "a", Code: 1}, {Agent: "b", Code: 2}})
		if !ok || w.Agent != "a" {
			t.Fatalf("%v %v", w, ok)
		}
	})
	add("arena/winner/smallest-success", func(t *testing.T) {
		w, ok := pickArenaWinner([]arenaArm{
			{Agent: "a", Code: 0, Bytes: 100},
			{Agent: "b", Code: 0, Bytes: 10},
			{Agent: "c", Code: 1, Bytes: 1},
		})
		if !ok || w.Agent != "b" {
			t.Fatalf("%+v", w)
		}
	})
	add("arena/winner/ignore-fail-small", func(t *testing.T) {
		w, _ := pickArenaWinner([]arenaArm{
			{Agent: "fail", Code: 1, Bytes: 1},
			{Agent: "ok", Code: 0, Bytes: 50},
		})
		if w.Agent != "ok" {
			t.Fatal(w)
		}
	})
	add("arena/winner/tie-first-smaller-equal", func(t *testing.T) {
		w, _ := pickArenaWinner([]arenaArm{
			{Agent: "a", Code: 0, Bytes: 5},
			{Agent: "b", Code: 0, Bytes: 5},
		})
		if w.Agent != "a" {
			t.Fatal(w)
		}
	})
	add("arena/winner/single-ok", func(t *testing.T) {
		w, ok := pickArenaWinner([]arenaArm{{Agent: "solo", Code: 0, Bytes: 9}})
		if !ok || w.Agent != "solo" {
			t.Fatal(w, ok)
		}
	})
	add("arena/winner/zero-bytes-wins", func(t *testing.T) {
		w, _ := pickArenaWinner([]arenaArm{
			{Agent: "fat", Code: 0, Bytes: 8},
			{Agent: "thin", Code: 0, Bytes: 0},
		})
		if w.Agent != "thin" {
			t.Fatal(w)
		}
	})
	add("arena/winner/three-fail-then-ok", func(t *testing.T) {
		w, _ := pickArenaWinner([]arenaArm{
			{Agent: "a", Code: 1, Bytes: 0},
			{Agent: "b", Code: 2, Bytes: 0},
			{Agent: "c", Code: 0, Bytes: 40},
		})
		if w.Agent != "c" {
			t.Fatal(w)
		}
	})

	for _, name := range []string{".git", "node_modules", "bin", workspace.LockName, ".astack-prompt.1.txt", ".astack-prompt.99.txt"} {
		name := name
		add("copy/skip/"+name, func(t *testing.T) {
			if !workspace.SkipName(name) {
				t.Fatalf("should skip %s", name)
			}
		})
	}
	add("copy/keep/main.go", func(t *testing.T) {
		if workspace.SkipName("main.go") {
			t.Fatal()
		}
	})
	add("copy/keep/agents.json", func(t *testing.T) {
		if workspace.SkipName("agents.json") {
			t.Fatal()
		}
	})
	add("copy/tree-and-bytes", func(t *testing.T) {
		src := t.TempDir()
		if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("hi"), 0600); err != nil {
			t.Fatal(err)
		}
		_ = os.MkdirAll(filepath.Join(src, ".git"), 0755)
		_ = os.WriteFile(filepath.Join(src, ".git", "x"), []byte("no"), 0600)
		dst := filepath.Join(t.TempDir(), "out")
		_ = os.MkdirAll(dst, 0755)
		if err := workspace.CopyTree(src, dst); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dst, "a.txt")); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dst, ".git", "x")); err == nil {
			t.Fatal("copied .git")
		}
		if workspace.TreeBytes(dst) < 2 {
			t.Fatal(workspace.TreeBytes(dst))
		}
	})
	add("copy/isolate-skips-lock", func(t *testing.T) {
		src := t.TempDir()
		_ = os.WriteFile(filepath.Join(src, "ok.txt"), []byte("x"), 0600)
		_ = os.WriteFile(filepath.Join(src, workspace.LockName), []byte("1"), 0600)
		dst, err := workspace.Isolate(src)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dst, workspace.LockName)); err == nil {
			t.Fatal("copied lock")
		}
		if _, err := os.Stat(filepath.Join(dst, "ok.txt")); err != nil {
			t.Fatal(err)
		}
	})

	bins := []string{"claude", "agy", "codex", "gemini", "opencode"}
	for _, osname := range []string{"windows", "darwin", "linux"} {
		osname := osname
		add("platform/name/"+osname, func(t *testing.T) {
			p := platform.NewPlatform(osname, platform.HostPaths{Home: "/h", LocalApp: `C:\la`, AppData: `C:\ad`})
			if p.Name() != osname {
				t.Fatal(p.Name())
			}
		})
		add("platform/maxarg/"+osname, func(t *testing.T) {
			p := platform.NewPlatform(osname, platform.HostPaths{})
			if osname == "windows" && p.MaxArgBytes() >= 100000 {
				t.Fatal()
			}
			if osname != "windows" && p.MaxArgBytes() <= 6000 {
				t.Fatal()
			}
		})
		for _, b := range bins {
			b := b
			add("platform/extrabins/"+osname+"/"+b, func(t *testing.T) {
				home := "/home/u"
				if osname == "windows" {
					home = `C:\Users\u`
				}
				p := platform.NewPlatform(osname, platform.HostPaths{Home: home, LocalApp: home + `\AppData\Local`, AppData: home + `\AppData\Roaming`})
				got := p.ExtraBins([]string{b})
				hit := false
				for _, g := range got {
					if strings.Contains(g, b) {
						hit = true
					}
				}
				if !hit || len(got) == 0 {
					t.Fatal(got)
				}
			})
		}
		add("platform/extradirs/"+osname, func(t *testing.T) {
			p := platform.NewPlatform(osname, platform.HostPaths{Home: "/home/u", LocalApp: `C:\la`, AppData: `C:\ad`})
			if len(p.ExtraDirs()) == 0 {
				t.Fatal()
			}
		})
	}
	add("platform/unknown-os-is-linux", func(t *testing.T) {
		p := platform.NewPlatform("freebsd", platform.HostPaths{})
		if p.Name() != "linux" {
			t.Fatal(p.Name())
		}
	})
	add("darwin/homebrew-on-path", func(t *testing.T) {
		p := platform.NewPlatform("darwin", platform.HostPaths{Home: "/Users/dev"})
		env := p.AugmentEnv([]string{"PATH=/usr/bin"})
		found := false
		for _, e := range env {
			if strings.Contains(e, "/opt/homebrew/bin") {
				found = true
			}
		}
		if !found {
			t.Fatal(env)
		}
	})
	add("linux/no-homebrew-on-path", func(t *testing.T) {
		p := platform.NewPlatform("linux", platform.HostPaths{Home: "/home/dev"})
		env := p.AugmentEnv([]string{"PATH=/usr/bin"})
		for _, e := range env {
			if strings.Contains(e, "/opt/homebrew/bin") {
				t.Fatal(env)
			}
		}
	})

	for _, id := range bins {
		id := id
		add("pick/unknown/"+id+"-typo", func(t *testing.T) {
			r := &agents.Registry{Order: bins, ByID: map[string]agents.Agent{id: {ID: id}}}
			p := r.Pick(id+"x", map[string]string{id: "/x"})
			if p.Code != 2 {
				t.Fatal(p)
			}
		})
		add("pick/absent/"+id, func(t *testing.T) {
			r := &agents.Registry{Order: bins, ByID: map[string]agents.Agent{id: {ID: id}}}
			p := r.Pick(id, map[string]string{id: ""})
			if p.Code != 2 {
				t.Fatal(p)
			}
		})
		add("pick/present/"+id, func(t *testing.T) {
			r := &agents.Registry{Order: []string{id}, ByID: map[string]agents.Agent{id: {ID: id}}}
			p := r.Pick(id, map[string]string{id: "/bin/" + id})
			if p.Code != 0 || p.Agent.ID != id {
				t.Fatal(p)
			}
		})
		add("pick/auto-first/"+id, func(t *testing.T) {
			resolved := map[string]string{}
			by := map[string]agents.Agent{}
			for _, x := range bins {
				by[x] = agents.Agent{ID: x}
				resolved[x] = ""
			}
			resolved[id] = "/bin/" + id
			r := &agents.Registry{Order: bins, ByID: by}
			p := r.Pick("auto", resolved)
			if p.Code != 0 || p.Agent.ID != id {
				t.Fatalf("want %s got %+v", id, p)
			}
		})
	}
	add("pick/auto-none", func(t *testing.T) {
		r := &agents.Registry{Order: []string{"claude"}, ByID: map[string]agents.Agent{"claude": {ID: "claude"}}}
		p := r.Pick("auto", map[string]string{"claude": ""})
		if p.Code != 3 {
			t.Fatal(p)
		}
	})
	add("pick/empty-name-is-auto", func(t *testing.T) {
		r := &agents.Registry{Order: []string{"claude"}, ByID: map[string]agents.Agent{"claude": {ID: "claude"}}}
		p := r.Pick("", map[string]string{"claude": "/c"})
		if p.Code != 0 || p.Agent.ID != "claude" {
			t.Fatal(p)
		}
	})

	win := platform.NewPlatform("windows", platform.HostPaths{})
	mac := platform.NewPlatform("darwin", platform.HostPaths{})
	for _, n := range []int{0, 1, 8, 64, 256, 1024, 4095, 6000, 6001, 8000, 12000, 99999} {
		n := n
		add(fmt.Sprintf("cmdline/windows/n=%d", n), func(t *testing.T) {
			a := agents.Agent{Args: []string{"-p", "{{prompt}}"}}
			prompt := strings.Repeat("x", n)
			got := a.CommandLine(map[string]string{"prompt": prompt, "prompt_file": `C:\p.txt`}, win)
			if n > 6000 && strings.Contains(got[1], strings.Repeat("x", 50)) && len(got[1]) > 200 {
				t.Fatal("windows should fall back for huge argv")
			}
			if n <= 100 && got[1] != prompt {
				t.Fatalf("small prompt %q vs %q", got[1], prompt)
			}
		})
		add(fmt.Sprintf("cmdline/darwin/n=%d", n), func(t *testing.T) {
			a := agents.Agent{Args: []string{"-p", "{{prompt}}"}}
			prompt := strings.Repeat("y", n)
			got := a.CommandLine(map[string]string{"prompt": prompt, "prompt_file": "/tmp/p"}, mac)
			if n <= 8000 && got[1] != prompt {
				t.Fatal("darwin should keep 8k inline")
			}
		})
	}

	add("expand/all-placeholders", func(t *testing.T) {
		a := agents.Agent{Args: []string{"{{prompt}}", "{{footer}}", "{{workdir}}", "{{prompt_file}}"}}
		got := a.Expand(map[string]string{"prompt": "p", "footer": "f", "workdir": "w", "prompt_file": "x"})
		if strings.Join(got, ",") != "p,f,w,x" {
			t.Fatal(got)
		}
	})
	add("expand/missing-placeholder", func(t *testing.T) {
		a := agents.Agent{Args: []string{"{{nope}}"}}
		got := a.Expand(map[string]string{"prompt": "p"})
		if got[0] != "{{nope}}" {
			t.Fatal(got)
		}
	})
	add("wrap/windows-cmd", func(t *testing.T) {
		exe, argv := win.Wrap(`C:\npm\x.cmd`, []string{"-p"})
		if exe != "cmd.exe" || argv[1] != `C:\npm\x.cmd` {
			t.Fatal(exe, argv)
		}
	})
	add("wrap/windows-bat", func(t *testing.T) {
		exe, _ := win.Wrap(`C:\x.bat`, nil)
		if exe != "cmd.exe" {
			t.Fatal(exe)
		}
	})
	add("wrap/windows-CMD-upper", func(t *testing.T) {
		exe, _ := win.Wrap(`C:\x.CMD`, nil)
		if exe != "cmd.exe" {
			t.Fatal(exe)
		}
	})
	add("wrap/windows-exe", func(t *testing.T) {
		exe, argv := win.Wrap(`C:\x.exe`, []string{"a"})
		if exe != `C:\x.exe` || argv[0] != "a" {
			t.Fatal(exe, argv)
		}
	})
	add("wrap/darwin-plain", func(t *testing.T) {
		exe, argv := mac.Wrap("/usr/bin/claude", []string{"-p"})
		if exe != "/usr/bin/claude" || argv[0] != "-p" {
			t.Fatal(exe, argv)
		}
	})
	add("or-auto/empty", func(t *testing.T) {
		if orAuto("") != "auto" {
			t.Fatal()
		}
	})
	add("or-auto/claude", func(t *testing.T) {
		if orAuto("claude") != "claude" {
			t.Fatal()
		}
	})
	add("flatten/dry-timeout", func(t *testing.T) {
		a := flattenDispatch(Flags{Workdir: "w", PromptFile: "p", Agent: "claude", JSON: true, DryRun: true, Timeout: time.Nanosecond})
		joined := strings.Join(a, " ")
		if strings.Contains(joined, "--json") {
			t.Fatal("child argv must not include --json")
		}
		if !strings.Contains(joined, "--dry-run") || !strings.Contains(joined, "--timeout") {
			t.Fatal(a)
		}
	})
	add("flatten/plain", func(t *testing.T) {
		a := flattenDispatch(Flags{Workdir: "w", PromptFile: "p", Agent: "gemini"})
		if strings.Join(a, " ") != "--workdir w --prompt-file p --agent gemini" {
			t.Fatal(a)
		}
	})
	add("is-ephemeral/go-build", func(t *testing.T) {
		if !agents.IsEphemeralExe(`/tmp/go-build99/exe`) {
			t.Fatal()
		}
	})
	add("is-ephemeral/go-trybuild", func(t *testing.T) {
		if !agents.IsEphemeralExe(`/tmp/go-trybuild/exe`) {
			t.Fatal()
		}
	})
	add("is-ephemeral/real", func(t *testing.T) {
		if agents.IsEphemeralExe(`/usr/bin/astack`) {
			t.Fatal()
		}
	})
	add("argv-bytes/empty", func(t *testing.T) {
		if agents.ArgvBytes(nil) != 0 {
			t.Fatal()
		}
	})
	add("argv-bytes/two", func(t *testing.T) {
		if agents.ArgvBytes([]string{"a", "bb"}) != 5 {
			t.Fatal()
		}
	})
	add("lock-name", func(t *testing.T) {
		if workspace.LockName != ".astack.lock" {
			t.Fatal(workspace.LockName)
		}
	})
	add("exit-busy", func(t *testing.T) {
		if workspace.ExitBusy != 4 {
			t.Fatal(workspace.ExitBusy)
		}
	})
	add("exit-timeout", func(t *testing.T) {
		if workspace.ExitTimeout != 124 {
			t.Fatal(workspace.ExitTimeout)
		}
	})
	add("review-workdir/empty", func(t *testing.T) {
		if workspace.DiffStat("") != "" {
			t.Fatal()
		}
	})
	add("review-workdir/not-git", func(t *testing.T) {
		if workspace.DiffStat(t.TempDir()) != "" {
			t.Fatal("non-git dir should not dump git errors")
		}
	})

	add("mode/swarm-dry-run", func(t *testing.T) {
		d := caseDisp(map[string]string{"claude": "/c"})
		code := d.Main([]string{"swarm", "--n", "3", "--workdir", t.TempDir(), "--prompt-file", writePrompt(t), "--dry-run"})
		if code != 0 {
			t.Fatal(code)
		}
	})
	add("mode/swarm-json-dry-run", func(t *testing.T) {
		out := &strings.Builder{}
		d := caseDisp(map[string]string{"claude": "/c"})
		d.Out = out
		code := d.Main([]string{"swarm", "--n", "2", "--workdir", t.TempDir(), "--prompt-file", writePrompt(t), "--dry-run", "--json"})
		if code != 0 || !strings.Contains(out.String(), `"swarm"`) {
			t.Fatal(code, out.String())
		}
	})
	add("mode/swarm-missing-workdir", func(t *testing.T) {
		d := caseDisp(map[string]string{"claude": "/c"})
		if code := d.Main([]string{"swarm", "--n", "2", "--prompt-file", writePrompt(t)}); code != 1 {
			t.Fatal(code)
		}
	})
	add("mode/arena-dry-run", func(t *testing.T) {
		out := &strings.Builder{}
		d := caseDisp(map[string]string{"claude": "/c", "gemini": "/g"})
		d.Out = out
		code := d.Main([]string{"arena", "--agents", "claude,gemini", "--workdir", t.TempDir(), "--prompt-file", writePrompt(t), "--dry-run"})
		if code != 0 || !strings.Contains(out.String(), "arena") {
			t.Fatal(code, out.String())
		}
	})
	add("mode/arena-one-agent", func(t *testing.T) {
		d := caseDisp(map[string]string{"claude": "/c"})
		if code := d.Main([]string{"arena", "--agents", "claude", "--workdir", t.TempDir(), "--prompt-file", writePrompt(t), "--dry-run"}); code != 1 {
			t.Fatal(code)
		}
	})
	add("mode/arena-named-missing", func(t *testing.T) {
		d := caseDisp(map[string]string{"claude": "/c"})
		if code := d.Main([]string{"arena", "--agents", "claude,gemini", "--workdir", t.TempDir(), "--prompt-file", writePrompt(t), "--dry-run"}); code != 2 {
			t.Fatal(code)
		}
	})
	add("mode/playbook-review-only", func(t *testing.T) {
		dir := t.TempDir()
		pb := filepath.Join(dir, "p.json")
		_ = os.WriteFile(pb, []byte(`{"name":"r","steps":[{"id":"rev","kind":"review"}]}`), 0600)
		out := &strings.Builder{}
		d := caseDisp(map[string]string{"claude": "/c"})
		d.Out = out
		code := d.Main([]string{"playbook", "--playbook", pb, "--workdir", dir, "--prompt-file", writePrompt(t)})
		if code != 0 || !strings.Contains(out.String(), "review") {
			t.Fatal(code, out.String())
		}
	})
	add("mode/playbook-named-feature-dry", func(t *testing.T) {
		d := caseDisp(map[string]string{"claude": "/c"})
		code := d.Main([]string{"playbook", "--playbook", "feature", "--workdir", t.TempDir(), "--prompt-file", writePrompt(t), "--dry-run"})
		if code != 0 {
			t.Fatal(code)
		}
	})
	add("mode/playbook-json-review", func(t *testing.T) {
		dir := t.TempDir()
		pb := filepath.Join(dir, "p.json")
		_ = os.WriteFile(pb, []byte(`{"name":"r","steps":[{"id":"rev","kind":"review"}]}`), 0600)
		out := &strings.Builder{}
		d := caseDisp(map[string]string{"claude": "/c"})
		d.Out = out
		code := d.Main([]string{"playbook", "--playbook", pb, "--json"})
		if code != 0 || !strings.Contains(out.String(), `"playbook"`) {
			t.Fatal(code, out.String())
		}
	})
	add("mode/playbook-missing", func(t *testing.T) {
		d := caseDisp(map[string]string{"claude": "/c"})
		if code := d.Main([]string{"playbook", "--playbook", "no-such-playbook"}); code != 1 {
			t.Fatal(code)
		}
	})
	add("mode/swarm-live-isolated", func(t *testing.T) {
		src := t.TempDir()
		_ = os.WriteFile(filepath.Join(src, "f.txt"), []byte("x"), 0600)
		run := &recordRunner{}
		d := caseDisp(map[string]string{"claude": "/c"})
		d.Runner = run
		code := d.Main([]string{"swarm", "--n", "2", "--workdir", src, "--prompt-file", writePrompt(t)})
		if code != 0 || run.calls != 2 {
			t.Fatalf("code=%d calls=%d", code, run.calls)
		}
		if run.cwd == src {
			t.Fatal("swarm must not write the source workdir")
		}
	})
	add("mode/arena-live-isolated", func(t *testing.T) {
		src := t.TempDir()
		_ = os.WriteFile(filepath.Join(src, "f.txt"), []byte("x"), 0600)
		run := &recordRunner{}
		d := caseDisp(map[string]string{"claude": "/c", "gemini": "/g"})
		d.Runner = run
		code := d.Main([]string{"arena", "--agents", "claude,gemini", "--workdir", src, "--prompt-file", writePrompt(t)})
		if code != 0 || run.calls != 2 {
			t.Fatalf("code=%d calls=%d", code, run.calls)
		}
	})
	add("mode/help", func(t *testing.T) {
		d := caseDisp(nil)
		if code := d.Main([]string{"--help"}); code != 0 {
			t.Fatal(code)
		}
	})
	add("mode/list", func(t *testing.T) {
		d := caseDisp(map[string]string{"claude": "/c"})
		if code := d.Main([]string{"--list"}); code != 0 {
			t.Fatal(code)
		}
	})

	pairs := [][2]string{{"claude", "agy"}, {"claude", "codex"}, {"claude", "gemini"}, {"claude", "opencode"}, {"agy", "codex"}, {"agy", "gemini"}, {"codex", "gemini"}, {"gemini", "opencode"}}
	for _, pair := range pairs {
		pair := pair
		add("mode/arena-pair/"+pair[0]+"+"+pair[1], func(t *testing.T) {
			look := map[string]string{pair[0]: "/a", pair[1]: "/b"}
			d := caseDisp(look)
			code := d.Main([]string{"arena", "--agents", pair[0] + "," + pair[1], "--workdir", t.TempDir(), "--prompt-file", writePrompt(t), "--dry-run"})
			if code != 0 {
				t.Fatal(code)
			}
		})
	}
	for n := 1; n <= 8; n++ {
		n := n
		add(fmt.Sprintf("mode/swarm-dry-n/%d", n), func(t *testing.T) {
			d := caseDisp(map[string]string{"claude": "/c"})
			code := d.Main([]string{"swarm", "--n", fmt.Sprint(n), "--workdir", t.TempDir(), "--prompt-file", writePrompt(t), "--dry-run"})
			if code != 0 {
				t.Fatal(code)
			}
		})
	}

	keep := []string{
		"README.md", "go.mod", "dispatch.go", "playbook.go", "swarm.go", "arena.go", "copy.go",
		"lock.go", "runner.go", "agent.go", "config.go", "resolve.go", "platform.go", "bench.go",
		"main.go", "feature.json", "src", "pkg", "internal", "testdata", "LICENSE", "Makefile",
		".gitignore", ".env.example", "Dockerfile", "app.ts", "index.js", "Cargo.toml",
	}
	for _, name := range keep {
		name := name
		add("copy/keep-name/"+name, func(t *testing.T) {
			if workspace.SkipName(name) {
				t.Fatal(name)
			}
		})
	}

	for len(cs) < 300 {
		n := len(cs)
		add(fmt.Sprintf("argv-bytes/content/%d", n), func(t *testing.T) {
			s := fmt.Sprintf("case-%d", n)
			if agents.ArgvBytes([]string{s}) != len(s)+1 {
				t.Fatal(s)
			}
		})
	}
	if len(cs) > 300 {
		cs = cs[:300]
	}
	return cs
}
