---
name: astack
description: aamogh stack (astack). The host IDE agent conducts. Optional local CLIs implement if already installed. If a CLI is missing, do the work in this chat. Do not install tools or switch to a different CLI.
---

# aamogh stack (astack)

The current IDE or chat agent is the conductor. A local coding CLI is the implementer only when that CLI is already on the machine.

This repo is the shareable copy. Recipients do not need Claude Max, Codex, Antigravity, or Gemini. Windows, macOS, and Linux use the same dispatcher.

## Workers

Known CLIs: `claude`, `agy` (Antigravity), `codex`, `gemini`, `opencode`. Detect with PATH plus default install locations (Homebrew, `~/.local/bin`, Windows npm shims). Never `npx`/`npm i -g`. Never print install docs.

## How to run

From this repo:

```text
go run . --workdir <app-repo> --prompt-file <prompt.txt> --agent auto
```

Or `./run.sh` (macOS, Linux, Git Bash) / `.\run.ps1` (PowerShell). Wrappers reuse `bin/` after a source change, so they are not `go run` every time.

If this chat is in a different repo (including Cursor on an app checkout), try in order: that repo's `.\run.ps1` / `./run.sh`, `astack` on PATH, `ASTACK_HOME`, `.astack-home`, sibling `../astack`, sibling `../ok/astack`. If none of those exist, edit files in this chat. Do not tell the human to clone astack or install a worker.

In Cursor: do not spawn Task for app code when a worker ran, or when exit is 3.

`--agent auto` uses the first installed CLI in `agents.json` order. Exit `3` means none installed. Then **you** edit the files.

`--json`, `--dry-run`, and `--timeout 5m` are for conductors. Exit `4` means another astack writer holds the workdir lock. Exit `124` means the worker was killed by `--timeout`.

If the human named an agent and it is absent, exit `2`. Stop. Do not fall back to another CLI. One line: `<name> is not installed; not rerouting.` No install link.

`--list` prints `installed` or `absent`. No suggestions.

`go run . tui --workdir .` is the coding-agent screen. It runs Claude Code (`claude`) or another PATH worker. `go run . agent --workdir . --prompt-file task.txt` is the same hop headless. Exit 3 if none are installed; then this chat edits. Do not ask for keys.

## Verify, review, thinking roles, models

```text
astack --workdir <repo> --prompt-file <file> --verify "go test ./..." [--retries 2]
astack review --workdir <repo> [--prompt-file <intent>] [--agents codex,claude]
astack why|explore|architect|reflect --workdir <repo> --prompt-file <file> [--agents a,b]
astack arena --agents claude,agy --workdir <repo> --prompt-file <file> --verify "<cmd>" --judge auto
astack ... --model opus | --model claude=opus,codex=gpt-5
```

- `--verify "<cmd>"` runs the command in the workdir after the worker exits 0. On failure the output tail goes back to the worker, up to `--retries` (default 2). Still failing: exit `5`.
- `review`, `why`, `explore`, `architect`, `reflect` are read-only panels. Every installed agent listed for the role in `agents.json` `roles` runs in parallel (fallback: first installed). A CLI with `read_args` (claude, codex) runs in place with its own read-only mode; any other CLI runs on a throwaway copy, so it cannot touch the repo. Two or more answers get merged by the `synth` role. `review` reads `git diff HEAD` plus untracked files.
- Arena: arms that fail `--verify` lose. `--judge auto|a,b` has the judge panel read each passing arm's diff and vote (`WINNER: n`). Ties go to the smaller tree.
- Playbooks run a panel first (`feature`/`refactoring`: architect, `bug-fix`/`perf-issue`: why, `hillclimb`: reflect), pass its answer to the implement step as notes, then run the review panel. Review is advice and never fails the playbook.
- Models: `--model` overrides; otherwise `roles.<role>.models.<agent>` in `agents.json`; otherwise the CLI default. `model_args` says how each CLI takes a model.
- TUI: `/mode why|review|...`, `/verify <cmd>|off`, `/model <m>|off`, `/judge auto|off`.

## Do not

- Spawn Cursor `Task` for app code when an installed worker ran successfully.
- Ask for API keys.
- Push, reroute, or advertise a missing CLI.
- Run several writer CLIs on the same checkout at once.

## Playbook, swarm, arena

These are dispatcher subcommands, not Cursor Tasks.

```text
go run . playbook --workdir <repo> --prompt-file <file> --playbook feature
go run . swarm --n 3 --workdir <repo> --prompt-file <file>
go run . arena --agents claude,codex --workdir <repo> --prompt-file <file>
```

Named playbooks: `feature`, `bug-fix`, `refactoring`, `perf-issue`, `hillclimb`. Review steps stay in this chat (`git diff`). Swarm uses isolated copies and OS-parallel exec. Arena picks the exit-0 tree with the fewest bytes; no LLM judge.

## Conductor

Todos, commit, push, and deploy stay in this chat unless the human asked the worker to do those.
