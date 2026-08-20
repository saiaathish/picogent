# Picogent

A tiny coding agent for people who find Claude Code and Codex too much.

Two modes. Uses your **ChatGPT Codex subscription** by default (same `~/.codex/auth.json` as Codex CLI, OpenClaw, and kestrel). It edits the folder you are in and tells you what it did.

This is **not** [MiniCode](https://github.com/LiuMengxuan04/MiniCode) (a Claude Code clone for learning internals). Picogent is a daily-driver pocket agent: 80% of the useful loop, 20% of the machinery.

## Install

Needs [Go 1.24+](https://go.dev/dl/).

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

If this is a new machine, Picogent opens a **browser setup page**. It installs missing Codex / Claude Code CLIs itself, then you tap **Log in to ChatGPT Codex**. ChatGPT OAuth opens, then you bounce back to setup.

Already set up:

```bash
picogent          # TUI
picogent gui      # browser chat
picogent setup    # open setup again
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

## What it can do

Read, write, and edit files. Glob and grep. Bash in the project folder. Git status / diff / commit (never push).

## What it will not do (v1)

MCP, subagents, browser driving, skills, plugins, embedding indexes.

## Docs

- Product spec: [docs/PRD.md](docs/PRD.md)
- License: MIT
