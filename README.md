# aamogh stack (astack)

The current IDE or chat agent is the conductor. A local coding CLI is the implementer only if that CLI is already installed.

This works on Windows, macOS, and Linux. Recipients do not need Claude Max, Codex, Antigravity, Gemini, or any other paid CLI. If none of those CLIs are present, this chat writes the code.

## Dispatch

Prefer `astack` on PATH, then `go run ../astack` when astack is a sibling, then `go run .` when this workspace is the astack repo.

```text
astack --workdir <app-repo> --prompt-file <prompt.txt> --agent auto
```

Wrappers (build once into `bin/`, then exec; same flags on every OS):

- macOS / Linux / Git Bash: `./run.sh --workdir <repo> --prompt-file <file> --agent auto`
- Windows PowerShell: `.\run.ps1 --workdir <repo> --prompt-file <file> --agent auto`

`--agent auto` uses the first installed CLI in `agents.json` order.

- Exit `3`: no worker CLI. You edit the files. Do not tell anyone to install a worker.
- Exit `2`: they named an agent that is not installed. Stop. One line: `<name> is not installed; not rerouting.` No install link.

`--list` prints `installed` or `absent`. No suggestions.

`astack tui` is your coding-agent screen. Enter a task and it runs `claude` / `codex` / `gemini` / `opencode` / `grok` if that CLI is already installed. `astack agent --workdir <repo> --prompt-file <file>` is the same hop without a screen.

Playbook, swarm, and arena are local processes (no Cursor Tasks):

```text
astack playbook --workdir <repo> --prompt-file <file> [--playbook feature]
astack swarm --n 3 --workdir <repo> --prompt-file <file>
astack arena --agents claude,codex --workdir <repo> --prompt-file <file>
```

`--bench` prints hop times for `--list`, stub dispatch, and dry-run playbook/swarm/arena. Those hops are local exec. pstack playbooks, swarm, and arena spawn Cursor Tasks.

GUI IDEs (Dock, Start Menu, launcher) often have a thin PATH. The dispatcher also looks in Homebrew (`/opt/homebrew`, `/usr/local`), `~/.local/bin`, and Windows npm shim dirs, and prepends those to the worker's PATH.

`./run.sh` native-builds only when `go env GOHOSTOS` matches the kernel (Git Bash → Windows Go; macOS → Darwin Go; Linux/WSL → Linux Go). A mismatched toolchain is skipped so WSL never starts downloading a second Go. Drop a prebuilt `bin/astack-<os>-<arch>` in that case, or edit in this chat.

Large prompts are written into the workdir as `.astack-prompt.<pid>.txt` so Windows `cmd` argv limits and Unix `ARG_MAX` do not truncate the task. That file is deleted after the worker exits.

Never `npx` or `npm i -g` a worker. Never ask for API keys.

## Other IDEs

`AGENTS.md` is the shared conductor brief. Cursor, Copilot, Codex, Windsurf, Cline, Zed, Amp, and OpenCode read it. Claude Code reads `CLAUDE.md`. Gemini CLI reads `GEMINI.md`. Copy `AGENTS.md` (and this dispatcher) into an app repo, or keep astack as a sibling.
