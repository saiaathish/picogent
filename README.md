# Picogent

A tiny coding agent for people who find Claude Code and Codex too much.

Two modes. Uses your **ChatGPT Codex subscription** by default (same `~/.codex/auth.json` as Codex CLI, OpenClaw, and kestrel). It edits the folder you are in and tells you what it did.

This is **not** [MiniCode](https://github.com/LiuMengxuan04/MiniCode) (a Claude Code clone for learning internals). Picogent is a daily-driver pocket agent: 80% of the useful loop, 20% of the machinery.

## Install

Needs [Go 1.25+](https://go.dev/dl/).

```bash
git clone https://github.com/saiaathish/picogent.git
cd picogent
go build -o picogent ./cmd/picogent
```

Or:

```bash
go install github.com/saiaathish/picogent/cmd/picogent@latest
```

Homebrew (from this repo, while we have no tap yet):

```bash
brew install --build-from-source ./contrib/homebrew/picogent.rb
```

## First run

```bash
./picogent
```

If this is a new machine, Picogent opens a **browser setup page**. It installs missing Codex / Claude Code CLIs itself, then you tap **Log in to ChatGPT Codex** (and optionally **Log in to Claude**). OAuth opens in the browser, then you bounce back to setup.

Already set up:

```bash
picogent          # TUI
picogent gui      # browser chat
picogent setup    # open setup again
```

**Claude Code subscription** (no Anthropic API key — same login as the `claude` CLI):

```bash
picogent login claude
# then in Settings → provider → Claude Code
```

**Local model:**

```bash
ollama pull qwen2.5-coder:7b
./picogent init --ollama
./picogent
```

**Your own key (OpenAI, OpenRouter, or any OpenAI-compatible URL):**

```bash
./picogent init --base-url https://openrouter.ai/api/v1 --model openai/gpt-4.1-mini
export PICOGENT_API_KEY=sk-...
./picogent
```

Then ask: `create a file hello.txt that says picogent`

In **Safe** mode (default) it will ask before writing. Press `y` or the giant Yes button.

## Commands

```bash
picogent                  # TUI
picogent gui              # local window in your browser (127.0.0.1)
picogent login            # Codex CLI login → ~/.codex/auth.json
picogent login claude     # Claude Code CLI login (subscription)
picogent run --yes "create hello.txt that says picogent"
picogent init --ollama
picogent version
```

### Modes

| Mode | Behavior |
|---|---|
| **Safe** (default) | Asks before writes and shell |
| **Fast** | Auto-edits inside the folder; still asks for deletes, `rm`, and paths outside the workspace |

TUI commands: `/safe` `/fast` `/model` `/provider codex|ollama|openai` `/reset` `/quit`

## What it can do (80/20 Claude Code harness)

Picogent implements the **useful core** of [Claude Code](https://github.com/anthropics/claude-code) and [tanbiralam/claude-code](https://github.com/tanbiralam/claude-code) (tool/command reference), without the 512k-line machinery.

| Feature | Picogent |
|---|---|
| Read / Write / Edit / Glob / Grep / Bash | yes |
| `list_dir`, `web_fetch`, `todo_write` | yes |
| MCP (Cursor-compatible config) | yes |
| Parallel tool calls | yes |
| Project rules (`AGENTS.md`, `CLAUDE.md`) | yes |
| Self-evolution (habits + playbooks, automatic, ≤720-char budget) | yes |
| Custom slash commands (`.claude/commands/*.md`) | yes |
| Built-in `/commit`, `/review`, `/compact`, `/diff`, `/memory`, `/resume` | yes |
| Session save/resume (`~/.picogent/sessions/`) | yes |
| Safe / Fast permissions | yes |
| Subagents, LSP, skills, plugins, voice | no (on purpose) |

**Workflow:** say what you want → Picogent plans, uses tools, asks before risky work, then reports what changed. You do not need `/goal`, `/plan`, or MCP commands. After useful turns it quietly remembers habits and short playbooks for this folder (self-evolution) — check with `/memory`.

### Optional slash commands (TUI + GUI)

`/commit` `/review` `/clear` `/status` `/memory`

Custom: add `.claude/commands/deploy.md` → type `/deploy`.

### MCP

Ask in chat to connect GitHub, a browser, or search — Picogent uses `mcp_manage` and waits for Allow. Config is Cursor-compatible (`mcp_<server>_<tool>`):

1. `~/.cursor/mcp.json`
2. `~/.picogent/mcp.yaml`
3. `{project}/.cursor/mcp.json`
4. `{project}/.mcp.json`

```yaml
# ~/.picogent/mcp.yaml
servers:
  browseros-neo:
    url: http://127.0.0.1:9010/mcp
    type: http
  github:
    command: npx
    args: ["-y", "@modelcontextprotocol/server-github"]
    env:
      GITHUB_PERSONAL_ACCESS_TOKEN: "..."
```

`picogent mcp list` shows what's connected. **Fast mode** auto-runs MCP tools; **Safe mode** asks first.

### Project rules

Drop `AGENTS.md` or `CLAUDE.md` in your repo — Picogent injects them into the system prompt (like Claude Code).

## What it will not do (v1)

Subagents, skills marketplace, plugins, embedding indexes.

## Docs

- Product spec: [docs/PRD.md](docs/PRD.md)
- v0.2 finish-the-loop guide: [docs/V0.2.md](docs/V0.2.md)
- License: MIT
