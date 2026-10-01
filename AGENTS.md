# aamogh stack (astack)

The current IDE or chat agent is the conductor. A local coding CLI is the implementer only if that CLI is already installed.

This works on Windows, macOS, and Linux. Recipients do not need Claude Max, Codex, Antigravity, Gemini, or any other paid CLI.

## Dispatch

Prefer `astack` on PATH, then `go run ../astack` when astack is a sibling, then `go run .` when this workspace is the astack repo. Wrappers: `./run.sh` (macOS, Linux, Git Bash), `.\run.ps1` (Windows PowerShell). If none of those exist, edit files in this chat. Do not tell the human to clone astack or install a worker.

```text
astack --workdir <app-repo> --prompt-file <prompt.txt> --agent auto
```

`--agent auto` uses the first installed CLI in `agents.json` order.

- Exit `3`: no worker CLI. You edit the files. Do not tell anyone to install a worker.
- Exit `2`: they named an agent that is not installed. Stop. One line: `<name> is not installed; not rerouting.` No install link.

`--list` prints `installed` or `absent`. `--json` prints the same as JSON. `--dry-run` prints the chosen worker and does not exec. `--timeout 5m` kills a hung worker (exit 124). A second writer in the same workdir gets exit `4`.

`astack` is the coding agent you run. It calls Claude Code (`claude`), Codex, Gemini, Antigravity, OpenCode, or Grok Build (`grok`) if that CLI is already on PATH — the same hop this chat used before. It is not its own LLM.

```text
astack tui --workdir <repo>
astack agent --workdir <repo> --prompt-file <file>
astack --workdir <repo> --prompt-file <file> --agent auto
```

`--agent auto` (and `astack agent`) picks the first installed worker. `/agent claude` in the TUI pins Claude Code. Exit `3` if none are installed; then this chat edits. Do not ask for keys.

`--bench` times the dispatcher hop: PATH `--list`, then a local no-op stub worker. Compare `astack_dispatch_stub_ms` to a blocking Cursor Task that only replies `PONG` and uses no tools. That Task is how pstack fans out implementation. It is not the same as the worker model thinking about a real coding task.

Playbook, swarm, and arena are local OS processes. They do not spawn Cursor Tasks. pstack playbooks, `/swarm`, and `/arena` each fan out blocking Tasks (and arena adds an LLM judge). astack does not.

```text
astack playbook --workdir <repo> --prompt-file <file> [--playbook feature]
astack swarm --n 3 --workdir <repo> --prompt-file <file>
astack arena --agents claude,codex --workdir <repo> --prompt-file <file>
```

`--playbook feature` (also `bug-fix`, `refactoring`, `perf-issue`, `hillclimb`) loads the embedded JSON. A file path still works. `review` steps run `git diff --stat` in-process; they do not spawn a judgment Task.

Swarm copies the workdir N times (skips `.git` / `node_modules`) and runs the same worker in parallel goroutines, each with its own lock.

Arena runs two or more installed agents on isolated copies and picks the exit-0 tree with the fewest bytes. No LLM judge.

Never `npx` / `npm i -g` a worker. Never ask for API keys. Never switch to another CLI on exit 2. Never run several writer CLIs on the same checkout at once. Do not spawn IDE-native subagents (Cursor Task, Copilot cloud agents, JetBrains delegates) for implementation when a local worker ran successfully. That fan-out is slower than one local `astack` exec.

After a worker exits 0, review `git diff` yourself. Playbooks, todos, commit, push, and deploy stay with the conductor unless the human asked the worker to do those.
