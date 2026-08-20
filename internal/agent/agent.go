package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/ctxmgr"
	"github.com/saiaathish/picogent/internal/goal"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/tools"
	"github.com/saiaathish/picogent/internal/trace"
)

const systemPromptBase = `You are Picogent — a small coding agent.

Do the work yourself. Never tell the user to type /goal, /plan, /debug, /mcp, or to edit config files.

1. Explore with glob/grep/read_file, then edit.
2. For long jobs, keep going until done. When fully done, start with "Goal complete:".
3. For bugs: hypothesize, gather evidence, fix, then call verify.
4. For large changes: a short plan via todo_write, then implement (unless they only asked a question).
5. To list, add, or remove MCP servers, use mcp_manage only — never browser MCP or config files.
6. If a task needs GitHub, a browser, Slack, Postgres, or web search and it is not connected yet, mcp_manage add (user must approve). Remove it when finished.
7. After code changes, call verify (or Picogent will).
8. Never git push. Never destructive shell unless asked.
9. After file changes, end with:
   Changed: ...
   Run: ...
   Undo: ...

Tools: read_file, list_dir, write_file, edit_file, glob, grep, bash, git, web_fetch, todo_write, mcp_manage, verify.
Be direct. No filler.`

const systemPromptMCP = `

MCP tools (names start with mcp_): external capabilities wired in from MCP servers — browsers, GitHub, Slack, databases, APIs, etc.
- Use MCP for anything outside plain files/shell in the workspace.
- Read each tool's description; pick the right one.
- Chain tools: navigate → snapshot/read → act, like a harness agent.
- If an MCP tool fails (server offline), say so briefly and try an alternative if obvious.`

const systemPromptBrowser = `
Browser MCP is connected. For opening websites or clicking in a page, use navigate/snapshot/act. Do not use browser tools to list Picogent MCP servers — that is mcp_manage.`

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
	Context      ctxmgr.Stats
	GoalDone     bool
	Verified     string
}

type Agent struct {
	CFG          config.Config
	LLM          llm.Client
	Tools        *tools.Registry
	Gate         *perm.Gate
	ProjectRules string
	SkillRules   string
	TaskMode     TaskMode
	Goal         string
	Trace        *trace.Log
}

func New(cfg config.Config, client llm.Client, reg *tools.Registry, gate *perm.Gate) *Agent {
	return &Agent{CFG: cfg, LLM: client, Tools: reg, Gate: gate, TaskMode: ParseTaskMode(cfg.TaskMode)}
}

func (a *Agent) SetTaskMode(m TaskMode) {
	if m.Valid() {
		a.TaskMode = m
		a.CFG.TaskMode = string(m)
	}
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
	if skills := strings.TrimSpace(a.SkillRules); skills != "" {
		p += "\n\n" + skills
	}
	if a.TaskMode.Valid() && a.TaskMode != TaskAgent {
		p += a.TaskMode.Prompt()
	}
	if suffix := goal.PromptSuffix(a.Goal); suffix != "" {
		p += suffix
	}
	return p
}

func (a *Agent) Run(ctx context.Context, history []llm.Message, user llm.Message, ev EventHandler) ([]llm.Message, Result, error) {
	if ev == nil {
		ev = NopHandler{}
	}
	_ = a.Trace.Append("turn_start", "", user.Content, nil, 0)
	if err := a.CFG.MissingAuth(); err != nil {
		ev.OnError(err)
		return history, Result{}, err
	}
	a.Gate.ResetTurn()
	a.Gate.Prompt = ev.OnNeedPermission

	userText := strings.TrimSpace(user.Content)
	msgs := make([]llm.Message, 0, len(history)+3)
	if len(history) == 0 || history[0].Role != "system" {
		msgs = append(msgs, llm.Message{Role: "system", Content: a.systemPrompt()})
	}
	msgs = append(msgs, history...)
	msgs = append(msgs, user)
	budget := ctxmgr.BudgetForModel(a.CFG.Model)
	compactMsgs, ctxStats, _ := ctxmgr.Manage(ctx, a.LLM, a.CFG.Model, msgs, budget)
	msgs = compactMsgs

	var res Result
	res.Context = ctxStats
	changed := map[string]struct{}{}
	lastToolKind := ""
	calledVerify := false

	for round := 0; round < a.CFG.MaxToolRounds; round++ {
		if r, ok := a.LLM.(*llm.Router); ok {
			r.SetUserPrompt(userText)
		}
		out, err := a.LLM.Chat(ctx, llm.ChatRequest{
			Model:        a.CFG.Model,
			Messages:     msgs,
			Tools:        a.Tools.Specs(),
			ToolRound:    round,
			Escalate:     round >= 6,
			TaskMode:     string(a.TaskMode),
			ReadOnly:     a.TaskMode.ReadOnly(),
			LastToolKind: lastToolKind,
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
			res.GoalDone = goal.LooksComplete(text)
			for p := range changed {
				res.FilesChanged = append(res.FilesChanged, p)
			}
			res.Verified = a.maybeVerify(ctx, ev, len(changed) > 0, calledVerify)
			_ = a.Trace.Append("turn_end", "", text, trace.Bool(true), 0)
			final, stats, _ := ctxmgr.Manage(ctx, a.LLM, a.CFG.Model, msgs, budget)
			res.Context = stats
			return final, res, nil
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
			if call.Name == "verify" {
				calledVerify = true
			}
			ev.OnToolStart(call)
			_ = a.Trace.Append("tool_start", call.Name, call.Arguments, nil, 0)
			if blocked, reason := a.TaskMode.BlockTool(call.Name); blocked {
				ev.OnToolEnd(call, reason, nil)
				pending = append(pending, executed{call: call, text: reason})
				continue
			}
			tool, ok := a.Tools.Get(call.Name)
			if !ok {
				err := fmt.Errorf("unknown tool %s", call.Name)
				ev.OnToolEnd(call, "", err)
				pending = append(pending, executed{call: call, text: err.Error(), err: err})
				continue
			}
			req := tool.Permission(call.Arguments, a.Tools.Ctx)
			req.Hint = perm.EnrichHint(req, call.Arguments)
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
				ok := err == nil
				_ = a.Trace.Append("tool_end", call.Name, outText, &ok, 0)
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
		if len(pending) > 0 {
			lastToolKind = classifyToolKind(pending[len(pending)-1].call.Name)
		}
		if len(msgs) > 35 {
			compactMsgs, stats, _ := ctxmgr.Manage(ctx, a.LLM, a.CFG.Model, msgs, budget)
			msgs = compactMsgs
			res.Context = stats
		}
	}
	err := fmt.Errorf("stopped after %d tool rounds (limit)", a.CFG.MaxToolRounds)
	ev.OnError(err)
	final, stats, _ := ctxmgr.Manage(ctx, a.LLM, a.CFG.Model, msgs, budget)
	res.Context = stats
	return final, res, err
}

func (a *Agent) maybeVerify(ctx context.Context, ev EventHandler, filesChanged, already bool) string {
	if already || !filesChanged || a.Tools == nil || a.Tools.Ctx.Verify == nil {
		return ""
	}
	tool, ok := a.Tools.Get("verify")
	if !ok {
		return ""
	}
	call := llm.ToolCall{ID: "verify-auto", Name: "verify", Arguments: "{}"}
	ev.OnToolStart(call)
	req := tool.Permission("{}", a.Tools.Ctx)
	req.Hint = perm.EnrichHint(req, "{}")
	dec, err := a.Gate.Check(ctx, req)
	if err != nil || dec == perm.Deny {
		msg := "verify skipped"
		if err != nil {
			msg = err.Error()
		} else {
			msg = "verify denied"
		}
		ev.OnToolEnd(call, msg, err)
		okv := false
		_ = a.Trace.Append("verify", "verify", msg, &okv, 0)
		return msg
	}
	out, err := tool.Run(ctx, "{}", a.Tools.Ctx)
	if err != nil {
		out = "error: " + err.Error()
	}
	ev.OnToolEnd(call, out, err)
	okv := err == nil && !strings.Contains(strings.ToLower(out), "fail")
	_ = a.Trace.Append("verify", "verify", out, &okv, 0)
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
