# Picogent PRD

**The tiny coding agent for people who find Claude Code and Codex too much.**

| | |
|---|---|
| Status | v0.1 MVP |
| Owner | Sai |
| License | MIT (open source, BYOK) |
| Binary | `picogent` |
| Platforms | macOS first (Homebrew + `.dmg`), then Linux |

This document is the source of truth. If code disagrees with this file, change the code.

---

## 1. Why this exists

Claude Code, Codex, OpenCode, and Google Antigravity are powerful and heavy. They have many modes, MCP, subagents, skills, indexers, and command surfaces. That power is the product — and it is also why beginners bounce.

**Picogent** is the opposite bet: 80% of the *useful* agent loop, 20% of the machinery.

Not a teaching clone of Claude Code. There is already a project named MiniCode ([LiuMengxuan04/MiniCode](https://github.com/LiuMengxuan04/MiniCode), 1k+ stars) that ports Claude Code’s architecture across TypeScript / Python / Rust / Go. We do not compete with that name or that mission.

Picogent is a **daily-driver pocket agent**: one folder, two modes, files + shell + search, TUI and a real macOS app, cheap by default.

### One-line pitch

> `picogent` — a small Go coding agent. Two modes. Codex subscription, a key, or a local model. It edits the folder you are in and tells you what it did.

---

## 2. Who it is for

**Primary:** people who can already write some code, and who find Claude Code / Codex / Antigravity overwhelming. Fewer commands, fewer surprises, visible permissions.

**Secondary:** students and hobbyists on 8GB laptops who want an agent that does not eat RAM or tokens.

**Not for:** teams that need MCP universes, parallel subagents, browser driving, or enterprise SSO in v1.

---

## 3. Positioning

| Product | What it is | Picogent difference |
|---|---|---|
| Claude Code / Codex | Full harnesses | Tiny surface, two modes, low RAM, BYOK |
| OpenCode | Open TUI harness, lots of providers | Simpler UX; GUI + TUI; strict resource budget |
| MiniCode (existing) | Claude Code replica for learning internals | We are a product, not a clone; different name |
| picocode | Rust CI/codemod agent | Interactive beginner UI + GUI app, not a CI tool |
| Aider | Git-centric CLI | Fewer git rituals; Safe/Fast instead of many flags |

**Niche we own:** the smallest *complete* coding agent you can install with Homebrew and also open as a `.dmg`, that stays understandable.

---

## 4. Locked decisions

These are decided. Do not re-litigate during v1.

1. **Name:** Picogent. CLI: `picogent`. Config dir: `~/.picogent/`.
2. **Language:** Go. One static-ish binary. No Node, no Electron, no Python runtime for the app itself.
3. **Surfaces:** TUI (`picogent`) and GUI (`picogent gui`). Same agent core. macOS `.app` / `.dmg` wraps the GUI.
4. **v1 job:** full agent loop over one workspace: read / write / edit files, shell, glob, grep, tiny git helper.
5. **Modes (only two):**
   - **Safe** — ask before writes and before shell.
   - **Fast** — auto-apply inside the workspace; still ask for deletes, `rm -rf`, and anything that escapes the folder.
6. **Auth v1:** ChatGPT Codex subscription via `~/.codex/auth.json` (same file as Codex CLI / OpenClaw / kestrel) is the default. Also BYOK OpenAI-compatible endpoints and Ollama. Keys live in `~/.picogent/config.yaml` or env vars. Never in the repo.
7. **Auth later:** Claude Code / Antigravity / OpenCode session login. Unofficial, can break, ToS-sensitive.
8. **Resource policy:** no embeddings index, no background watcher, no subagents. Small system prompt. Cap tool rounds. Truncate tool output. Default to small models. The agent may list/add/remove catalog MCP servers via `mcp_manage` (user approves add/remove).
9. **Cut from v1:** parallel subagents, skills marketplace, plugin universe, slash-command universe, IDE extension.
10. **Keep (80/20):** chat, streaming-ish updates, file tools, bash, search, git, auto plan/debug/goal from the message (beginners never need `/goal` or `/mcp`), Safe/Fast permissions, verify after edits, Explain-what-changed footer.
11. **Distribution:** `brew install picogent` (tap later) and a signed-later unsigned-ok-for-now macOS `.dmg`.
12. **License:** MIT.

---

## 5. Product principles

1. **Visible beats clever.** The user always sees: mode, model, folder, last tool, and whether Picogent is waiting on them.
2. **Ask less, but ask at the right time.** Two modes. No permission matrix.
3. **Cheap is a feature.** Default model is small. Context is grep, not “load the repo.”
4. **One folder is the universe.** Tools cannot read/write outside the workspace without an explicit Safe-mode confirmation that names the path.
5. **Explain the diff.** Every turn that changes files ends with: what changed, how to run it, how to undo.
6. **If it needs a docs site to start, we failed.** First run is: install → set key or `ollama pull` → type a sentence.

---

## 6. User stories (v1)

1. I install with Homebrew, run `picogent`, paste an OpenRouter/OpenAI/Anthropic-compatible key (or pick Ollama), and it works.
2. I say “add a README to this folder” and in Safe mode I approve one write, then see the file.
3. I switch to Fast and it makes several edits without nagging, but it still stops if a command tries to `rm` or leave the folder.
4. I open `picogent gui` (or the `.dmg` app) and get the same agent in a window, not a different product.
5. On an 8GB laptop it stays under **~50MB RSS** for the process (excluding the LLM).
6. If something fails (bad key, Ollama down, model missing), the error names the problem, the cause, and the fix in one short block.

---

## 7. UX

### First run

```
picogent
```

If no config, open the **browser setup**:

1. Install missing cores (Git check, `~/.picogent`, Codex CLI, optional Claude Code CLI).
2. **Log in to ChatGPT Codex** (OAuth in the same browser). Optional: Log in to Claude.
3. Pick folder + Safe/Fast.
4. Drop into chat.

No extra desktop app. The GUI is a local page.

### TUI (must be boring)

- Header: `picogent  |  Safe  |  gpt-4.1-mini  |  ~/proj`
- Scrollback of messages and tool calls (name + short result).
- Permission card: `Write src/main.go (812 bytes)?  [y] [n] [always this turn]`
- Input at the bottom. `Ctrl-C` interrupts the agent. `Ctrl-D` / `/quit` exits.
- Slash commands, **only these:**
  - `/safe` `/fast`
  - `/model`
  - `/help`
  - `/reset` (new session, keep config)
  - `/quit`

### GUI (must feel like a small app, not a chatbot landing page)

- Left: conversation.
- Right: live file change list (path + +/−).
- Top bar: mode toggle, model, folder.
- Sticky permission banner when Safe needs a yes/no.
- Same slash commands as the TUI, plus clickable mode toggle.

macOS `.app` just launches `picogent gui` with a tiny native window or the default browser bound to `127.0.0.1`. Prefer an in-app webview later; **v1 may open the local URL in the default browser** if that ships faster, but the `.dmg` target is a dedicated window.

---

## 8. Functional requirements

### Agent loop

1. User message in.
2. Model may call tools.
3. Tools run under the permission gate.
4. Tool results go back to the model.
5. Repeat until the model stops calling tools or hits **max 25 tool rounds**.
6. Final assistant message must include the explain footer if any file changed.

### Tools (v1)

| Tool | Does | Notes |
|---|---|---|
| `read_file` | Read a UTF-8 file | Line cap / byte cap; skip binaries |
| `write_file` | Create or overwrite | Safe asks; Fast auto inside workspace |
| `edit_file` | Exact string replace | Fail if not unique |
| `glob` | Find paths | Skip `.git`, `node_modules`, `dist`, `.venv` |
| `grep` | Search file contents | Use `rg` if present, else walk |
| `bash` | Run a command in the workspace | Timeout 60s; truncate output; no interactive TTY |
| `git` | `status`, `diff`, `commit` | Commit only with a message; **never push** |

### Permission gate

| Action | Safe | Fast |
|---|---|---|
| read / glob / grep / git status+diff | auto | auto |
| write / edit inside workspace | ask | auto |
| bash that is not destructive and cwd stays inside | ask | auto |
| delete, `rm`, `mv` out, chmod, git commit | ask | ask |
| any path outside workspace | ask, show absolute path | ask |

Destructive bash heuristic: match `(^|[;&|]\s*)(rm|sudo|mkfs|dd|shutdown|reboot)\b` and `rm -rf`.

### Config (`~/.picogent/config.yaml`)

```yaml
workspace: "."          # overridden by cwd / --dir
mode: safe              # safe | fast
provider: openai        # openai | ollama
base_url: "https://api.openai.com/v1"
api_key: ""             # or $PICOGENT_API_KEY / $OPENAI_API_KEY
model: "gpt-4.1-mini"
ollama_url: "http://127.0.0.1:11434"
max_tool_rounds: 25
```

Project overlay (optional): `./.picogent.yaml` may set `mode` and `model` only. Never keys.

### Resource budget (enforced)

| Budget | Limit |
|---|---|
| System prompt | < 1.5k tokens of instructions |
| Tool payload | Truncate each result to 32 KiB |
| Read file | 2000 lines or 256 KiB, whichever first |
| Session | Keep last 30 messages; if larger, drop oldest tool bodies first |
| Process | No indexer, no extra daemons |
| Default timeout | 60s per bash, 120s per LLM call |

---

## 9. Architecture

```
cmd/picogent          CLI: tui | gui | run | init
internal/config       load/save yaml, env
internal/llm          OpenAI-compatible Chat Completions + tools; Ollama via same shape
internal/agent        loop + system prompt + events
internal/tools        implementations
internal/perm         Safe/Fast gate
internal/session      on-disk JSONL under ~/.picogent/sessions/
internal/tui          Bubble Tea
internal/gui          net/http + go:embed web UI
web/                  static HTML/CSS/JS (no React, no bundler)
```

**Event API** (TUI and GUI both consume this; do not fork the loop):

- `Text(delta)`
- `ToolStart` / `ToolEnd`
- `NeedPermission(Request) Decision`
- `Error`
- `Done`

Headless: `picogent run "prompt"` uses Fast inside CI only if `--yes` is passed; otherwise Safe auto-denies (exit 2) so it cannot hang.

---

## 10. Errors

Every user-facing error is three lines:

```
Problem: Ollama is not running.
Cause:   nothing is listening on 127.0.0.1:11434.
Fix:     run `ollama serve` and `ollama pull qwen2.5-coder:7b`, then /model.
```

Never dump a Go stack to the TUI.

---

## 11. Out of scope (v1)

- MCP, skills, plugins, hooks, subagents
- Browser automation
- Embedding / vector index
- Windows installer (Linux binary is nice-to-have if cheap)
- Subscription OAuth
- Multi-root workspaces
- Telemetry
- Our own hosted models

---

## 12. Success for v0.1

A stranger can:

1. `go install` or run the binary.
2. Point at Ollama or a key.
3. In this repo, ask: “create a file hello.txt that says picogent”.
4. Approve once in Safe mode.
5. See the file on disk.

TUI and GUI both do that. Tests cover the agent loop with a fake LLM (no network).

---

## 13. Implementation order

1. Config + LLM client + fake client for tests
2. Tools + permission gate
3. Agent loop
4. `picogent run` (headless)
5. TUI
6. GUI
7. Homebrew formula + macOS app wrapper

Ship as soon as 1–5 work. GUI can trail by a day, not a month.
