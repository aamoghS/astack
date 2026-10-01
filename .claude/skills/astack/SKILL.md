---
name: astack
description: aamogh stack (astack). The host IDE agent conducts. Optional local CLIs implement if already installed. If a CLI is missing, do the work in this chat. Do not install tools or switch to a different CLI.
---

# aamogh stack (astack)

The current IDE or chat agent is the conductor. A local coding CLI is the implementer only when that CLI is already on the machine.

The shareable copy is the `astack` repo (clone and open in any IDE, or keep it as a sibling of an app repo). Recipients do not need Claude Max, Codex, Antigravity, or Gemini. Windows, macOS, and Linux use the same dispatcher.

This skill is for Cursor and every other host. Cursor chat is the conductor here. Do not spawn Cursor Task for app code when a worker ran, or when exit is 3.

## Workers

Known CLIs: `claude`, `agy` (Antigravity), `codex`, `gemini`, `opencode`, `grok` (Grok Build). Detect with PATH plus default install locations (Homebrew, `~/.local/bin`, `~/.grok/bin`, Windows npm shims). Never `npx`/`npm i -g`. Never print install docs.

## Locate the dispatcher

Try in order. Stop at the first hit.

1. `astack` on PATH
2. `ASTACK_HOME` (folder containing `run.ps1` / `run.sh`)
3. `.astack-home` in the app repo (one line, a folder path)
4. `./run.ps1` or `./run.sh` in the current app repo
5. sibling `../astack`
6. sibling `../ok/astack`
7. `go run .` when this workspace is the astack repo

If none of those exist, edit files in this chat. Do not tell the human to clone astack or install a worker.

## How to run

From an app repo (workdir defaults to that repo when using its wrappers):

```text
.\run.ps1 --prompt-file <prompt.txt> --agent auto
./run.sh --prompt-file <prompt.txt> --agent auto
```

From the astack repo:

```text
go run . --workdir <app-repo> --prompt-file <prompt.txt> --agent auto
.\run.ps1 --workdir <app-repo> --prompt-file <prompt.txt> --agent auto
```

`--agent auto` uses the first installed CLI in `agents.json` order. Exit `3` means none installed. Then **you** edit the files.

If the human named an agent and it is absent, exit `2`. Stop. Do not fall back to another CLI. One line: `<name> is not installed; not rerouting.` No install link.

`--list` prints `installed` or `absent`. No suggestions.

`astack tui` is the coding-agent screen. It runs `claude` / `codex` / `gemini` / `opencode` / `grok` if installed. `astack agent` is that hop without a screen.

Playbook, swarm, and arena are dispatcher subcommands. They do not spawn Cursor Tasks:

```text
go run . playbook --workdir <repo> --prompt-file <file> --playbook feature
go run . swarm --n 3 --workdir <repo> --prompt-file <file>
go run . arena --agents claude,codex --workdir <repo> --prompt-file <file>
```

Large prompts are written into the workdir as `.astack-prompt.<pid>.txt` so Windows argv limits do not truncate the task. That file is deleted after the worker exits.

## Do not

- Spawn Cursor `Task` for app code when an installed worker ran successfully, or when exit is 3.
- Ask for API keys.
- Push, reroute, or advertise a missing CLI.
- Run several writer CLIs on the same checkout at once.

## Conductor

Playbooks, todos, `git diff` review, commit/push/deploy stay in this chat unless the human asked the worker to do those. Named playbooks: `feature`, `bug-fix`, `refactoring`, `perf-issue`, `hillclimb`. Swarm uses isolated copies. Arena picks the smallest exit-0 tree with no LLM judge.
