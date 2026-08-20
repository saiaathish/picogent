package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/tools"
)

const systemPromptBase = `You are Picogent — a coding agent in the Claude Code mold, kept simple (80/20).

Core workflow:
1. When the user asks you to do something, DO it with tools. Do not tell them to run commands, open apps, or click links if you have a tool for it.
2. Explore before you edit: glob/grep/read_file first.
3. Prefer edit_file for small changes; write_file for new files.
4. Keep diffs small. Do not invent files you have not read.
5. Never git push. Never run destructive shell unless the user asked.
6. After file changes, end with three short lines:
   Changed: ...
   Run: ...
   Undo: ... (git checkout -- <file> or delete the new file)

Built-in tools: read_file, list_dir, write_file, edit_file, glob, grep, bash, git, web_fetch, todo_write.
Use todo_write for multi-step tasks (like Claude Code).

Be direct. No filler. No "you can run..." — just run it.`

const systemPromptMCP = `

MCP tools (names start with mcp_): external capabilities wired in from MCP servers — browsers, GitHub, Slack, databases, APIs, etc.
- Use MCP for anything outside plain files/shell in the workspace.
- Read each tool's description; pick the right one.
- Chain tools: navigate → snapshot/read → act, like a harness agent.
- If an MCP tool fails (server offline), say so briefly and try an alternative if obvious.`

const systemPromptBrowser = `
Browser MCP is connected. For "open X", "check Y in the browser", "go to GitHub": use navigate/snapshot/act/read tools immediately — do not claim you lack browser access.`

type EventHandler interface {
	OnText(text string)
	OnToolStart(call llm.ToolCall)
	OnToolEnd(call llm.ToolCall, result string, err error)
	OnNeedPermission(ctx context.Context, req perm.Request) (perm.Decision, error)
	OnError(err error)
}

type Result struct {
	Text         string
	FilesChanged []string
	ToolRounds   int
}

type Agent struct {
	CFG          config.Config
	LLM          llm.Client
	Tools        *tools.Registry
	Gate         *perm.Gate
	ProjectRules string
}

func New(cfg config.Config, client llm.Client, reg *tools.Registry, gate *perm.Gate) *Agent {
	return &Agent{CFG: cfg, LLM: client, Tools: reg, Gate: gate}
}

func (a *Agent) systemPrompt() string {
	p := systemPromptBase
	if a.Tools != nil && a.Tools.HasMCP() {
		p += systemPromptMCP
		if a.Tools.HasBrowserMCP() {
			p += systemPromptBrowser
		}
	}
	if rules := strings.TrimSpace(a.ProjectRules); rules != "" {
		p += "\n\nProject rules (follow these):\n" + rules
	}
	return p
}

func (a *Agent) Run(ctx context.Context, history []llm.Message, user string, ev EventHandler) ([]llm.Message, Result, error) {
	if ev == nil {
		ev = NopHandler{}
	}
	if err := a.CFG.MissingAuth(); err != nil {
		ev.OnError(err)
		return history, Result{}, err
	}
	a.Gate.ResetTurn()
	a.Gate.Prompt = ev.OnNeedPermission

	msgs := make([]llm.Message, 0, len(history)+3)
	if len(history) == 0 || history[0].Role != "system" {
		msgs = append(msgs, llm.Message{Role: "system", Content: a.systemPrompt()})
	}
	msgs = append(msgs, history...)
	msgs = append(msgs, llm.Message{Role: "user", Content: user})
	msgs = compact(msgs)

	var res Result
	changed := map[string]struct{}{}

	for round := 0; round < a.CFG.MaxToolRounds; round++ {
		if r, ok := a.LLM.(*llm.Router); ok {
			r.SetUserPrompt(user)
		}
		out, err := a.LLM.Chat(ctx, llm.ChatRequest{
			Model:     a.CFG.Model,
			Messages:  msgs,
			Tools:     a.Tools.Specs(),
			ToolRound: round,
			Escalate:  round >= 6,
		})
		if err != nil {
			wrapped := userErr("the model call failed", err)
			ev.OnError(wrapped)
			return msgs, res, wrapped
		}
		msg := out.Message
		if msg.Role == "" {
			msg.Role = "assistant"
		}
		msgs = append(msgs, msg)

		if len(msg.ToolCalls) == 0 {
			text := strings.TrimSpace(msg.Content)
			if text != "" {
				ev.OnText(text)
			}
			res.Text = text
			res.ToolRounds = round
			for p := range changed {
				res.FilesChanged = append(res.FilesChanged, p)
			}
			return compact(msgs), res, nil
		}

		res.ToolRounds = round + 1
		type executed struct {
			call llm.ToolCall
			req  perm.Request
			text string
			err  error
		}
		var pending []executed

		for _, call := range msg.ToolCalls {
			ev.OnToolStart(call)
			tool, ok := a.Tools.Get(call.Name)
			if !ok {
				err := fmt.Errorf("unknown tool %s", call.Name)
				ev.OnToolEnd(call, "", err)
				pending = append(pending, executed{call: call, text: err.Error(), err: err})
				continue
			}
			req := tool.Permission(call.Arguments, a.Tools.Ctx)
			dec, err := a.Gate.Check(ctx, req)
			if err != nil {
				ev.OnToolEnd(call, "", err)
				return msgs, res, err
			}
			if dec == perm.Deny {
				ev.OnToolEnd(call, "denied by user", nil)
				pending = append(pending, executed{call: call, req: req, text: "denied by user"})
				continue
			}
			pending = append(pending, executed{call: call, req: req})
		}

		var wg sync.WaitGroup
		for i := range pending {
			if pending[i].text != "" {
				continue
			}
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				call := pending[i].call
				tool, _ := a.Tools.Get(call.Name)
				outText, err := tool.Run(ctx, call.Arguments, a.Tools.Ctx)
				pending[i].text = outText
				pending[i].err = err
				if err != nil {
					pending[i].text = "error: " + err.Error()
				}
				ev.OnToolEnd(call, outText, err)
			}(i)
		}
		wg.Wait()

		for _, ex := range pending {
			if ex.call.Name == "write_file" || ex.call.Name == "edit_file" {
				changed[ex.req.Path] = struct{}{}
			}
			content := ex.text
			if content == "" && ex.err != nil {
				content = ex.err.Error()
			}
			msgs = append(msgs, llm.Message{Role: "tool", ToolCallID: ex.call.ID, Name: ex.call.Name, Content: content})
		}
	}
	err := fmt.Errorf("stopped after %d tool rounds (limit)", a.CFG.MaxToolRounds)
	ev.OnError(err)
	return compact(msgs), res, err
}

func compact(msgs []llm.Message) []llm.Message {
	const keep = 30
	if len(msgs) <= keep {
		return msgs
	}
	head := msgs[0]
	tail := msgs[len(msgs)-keep+1:]
	out := make([]llm.Message, 0, keep)
	out = append(out, head)
	out = append(out, tail...)
	for i := range out {
		if out[i].Role == "tool" && len(out[i].Content) > 2000 && i < len(out)-6 {
			out[i].Content = out[i].Content[:2000] + "\n… dropped for context …"
		}
	}
	return out
}

type NopHandler struct{}

func (NopHandler) OnText(string)                         {}
func (NopHandler) OnToolStart(llm.ToolCall)              {}
func (NopHandler) OnToolEnd(llm.ToolCall, string, error) {}
func (NopHandler) OnNeedPermission(context.Context, perm.Request) (perm.Decision, error) {
	return perm.Deny, nil
}
func (NopHandler) OnError(error) {}

func userErr(problem string, err error) error {
	cause := err.Error()
	fix := "check Codex login (`picogent login`), or your key / base URL / model name."
	low := strings.ToLower(cause)
	if strings.Contains(low, "connection refused") || strings.Contains(cause, "11434") {
		fix = "run `ollama serve`, then `ollama pull` your model, and set provider: ollama."
	}
	if strings.Contains(low, "401") || strings.Contains(low, "codex login") || strings.Contains(low, "auth.json") {
		fix = "run `picogent login` (or `codex login`) so Picogent can use ~/.codex/auth.json."
	}
	return fmt.Errorf("Problem: %s.\nCause:   %s.\nFix:     %s", problem, cause, fix)
}
