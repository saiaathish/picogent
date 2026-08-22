package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/ctxmgr"
	"github.com/saiaathish/picogent/internal/evolve"
	"github.com/saiaathish/picogent/internal/goal"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
	"github.com/saiaathish/picogent/internal/trace"
)

const systemPromptBase = `You are Picogent — the user's coding assistant.

You already are their assistant: do the work yourself, reuse what you have learned in this workspace, and keep going. Never wait to be told to "be an assistant." Never tell the user to type /goal, /plan, /debug, /mcp, or to edit config files.

1. Explore with glob/grep/read_file, then edit.
2. For long jobs, keep going until done. When fully done, start with "Goal complete:".
3. For bugs: hypothesize, gather evidence, fix, then call verify.
4. For large changes: a short plan via todo_write, then implement (unless they only asked a question).
5. To list, add, or remove MCP servers, use mcp_manage only — never browser MCP or config files.
6. If a task needs GitHub, a browser, Slack, Postgres, or web search and it is not connected yet, mcp_manage add (user must approve). Remove it when finished.
7. After code changes, call verify (or Picogent will).
8. Never git push. Never destructive shell unless asked.
9. After successful file changes, end with:
   Changed: ...
   Run: ...
   Undo: /undo
   If nothing was written (denied, blocked, or read-only), do not invent a Changed/Run/Undo footer.

Tools: repo_map, read_file, list_dir, write_file, edit_file, glob, grep, bash, git, web_fetch, todo_write, mcp_manage, verify.
Be direct. No filler.`

const systemPromptMCP = `

MCP tools (names start with mcp_): external capabilities wired in from MCP servers — browsers, GitHub, Slack, databases, APIs, etc.
- Use MCP for anything outside plain files/shell in the workspace.
- Read each tool's description; pick the right one.
- Chain tools: navigate → snapshot/read → act, like a harness agent.
- If an MCP tool fails (server offline), say so briefly and try an alternative if obvious.`

const systemPromptBrowser = `
Browser MCP is connected. For opening websites or clicking in a page, use navigate/snapshot/act. Do not use browser tools to list Picogent MCP servers — that is mcp_manage.
Tab hygiene: prefer one owned tab for a task; reuse it; close inactive tabs you opened when done. Do not close the user's existing tabs.`

type EventHandler interface {
	OnText(text string)
	OnTextDelta(delta string)
	OnToolStart(call llm.ToolCall)
	OnToolEnd(call llm.ToolCall, result string, err error)
	OnNeedPermission(ctx context.Context, req perm.Request) (perm.Decision, error)
	OnError(err error)
}

// FinalTextHandler lets streaming surfaces replace the accumulated assistant
// text with the canonical final response. This is optional so existing event
// handlers keep working.
type FinalTextHandler interface {
	OnTextFinal(text string)
}

type Result struct {
	Text          string
	FilesChanged  []string
	ToolRounds    int
	Context       ctxmgr.Stats
	GoalDone      bool
	Verified      string
	Task          *taskstate.Task
	UndoAvailable bool
	UndoError     string
}

type Agent struct {
	CFG          config.Config
	LLM          llm.Client
	Tools        *tools.Registry
	Gate         *perm.Gate
	ProjectRules string
	SkillRules   string
	Memory       evolve.Store // learned habits/playbooks; injected per-turn with a hard byte budget
	TaskMode     TaskMode
	Goal         string
	Trace        *trace.Log
	TaskStore    *taskstate.Store
	TaskSession  string
	taskMu       sync.RWMutex
	task         *taskstate.Task
	undoMu       sync.Mutex
	latestUndo   *turnUndo
	runTool      func(context.Context, llm.ToolCall, tools.Tool, tools.Context) (string, error)
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

func (a *Agent) systemPrompt(userHint string) string {
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
	if mem := strings.TrimSpace(evolve.PromptFor(a.Memory, userHint)); mem != "" {
		p += "\n\n" + mem
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
	if suffix := a.taskPromptSuffix(); suffix != "" {
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
	a.beginDurableTask(userText)
	// Always refresh the system prompt so mid-chat task mode / goal changes take effect.
	msgs := make([]llm.Message, 0, len(history)+3)
	msgs = append(msgs, llm.Message{Role: "system", Content: a.systemPrompt(userText)})
	for i, m := range history {
		if i == 0 && m.Role == "system" {
			continue
		}
		msgs = append(msgs, m)
	}
	msgs = append(msgs, user)
	budget := ctxmgr.BudgetForModel(a.CFG.Model)
	compactMsgs, ctxStats, _ := ctxmgr.Manage(ctx, a.LLM, a.CFG.Model, msgs, budget)
	msgs = compactMsgs

	var res Result
	res.Context = ctxStats
	changed := map[string]struct{}{}
	turnUndo := newTurnUndo(a.Tools.Ctx.Workspace)
	nativeWriteRan := false
	lastToolKind := ""
	calledVerify := false
	taskBlocker := ""

	for round := 0; round < a.CFG.MaxToolRounds; round++ {
		if r, ok := a.LLM.(*llm.Router); ok {
			r.SetUserPrompt(userText)
		}
		streamed := false
		out, err := a.LLM.Chat(ctx, llm.ChatRequest{
			Model:        a.CFG.Model,
			Messages:     msgs,
			Tools:        a.Tools.Specs(),
			ToolRound:    round,
			Escalate:     round >= 10,
			TaskMode:     string(a.TaskMode),
			ReadOnly:     a.TaskMode.ReadOnly(),
			LastToolKind: lastToolKind,
			OnDelta: func(delta string) {
				if delta == "" {
					return
				}
				streamed = true
				ev.OnTextDelta(delta)
			},
		})
		if err != nil {
			wrapped := userErr("the model call failed", err)
			ev.OnError(wrapped)
			res.FilesChanged = sortedChanged(changed)
			a.finishTurnUndo(&res, turnUndo, nativeWriteRan)
			return msgs, res, wrapped
		}
		msg := out.Message
		if msg.Role == "" {
			msg.Role = "assistant"
		}
		msgs = append(msgs, msg)

		if len(msg.ToolCalls) == 0 {
			text := strings.TrimSpace(msg.Content)
			if a.continueAfterDeferral(text, round) {
				msgs = append(msgs, llm.Message{Role: "system", Content: durableContinuePrompt})
				continue
			}
			res.FilesChanged = sortedChanged(changed)
			res.Verified = a.maybeVerify(ctx, ev, res.FilesChanged, calledVerify)
			if a.continueAfterVerificationFailure(text, round, res.Verified) {
				msgs = append(msgs, llm.Message{Role: "system", Content: durableRepairPrompt(res.Verified)})
				continue
			}
			a.finishTurnUndo(&res, turnUndo, nativeWriteRan)
			for _, path := range res.FilesChanged {
				a.noteTaskChanged(path)
			}
			normalized := normalizeExplainFooter(text, res.FilesChanged, res.UndoAvailable, res.UndoError)
			oldText := text
			if normalized != text {
				text = normalized
				msg.Content = text
				msgs[len(msgs)-1] = msg
			}
			if streamed {
				if finalizer, ok := ev.(FinalTextHandler); ok {
					finalizer.OnTextFinal(text)
				} else {
					if footer := explainFooter(res.FilesChanged, res.UndoAvailable, res.UndoError); normalized != oldText && footer != "" {
						ev.OnTextDelta("\n\n" + footer)
					}
					ev.OnText("")
				}
			} else if text != "" {
				ev.OnText(text)
			}
			res.Text = text
			res.ToolRounds = round
			res.GoalDone = goal.LooksComplete(text)
			a.finishDurableTask(res.FilesChanged, text, taskBlocker)
			res.Task = a.TaskSnapshot()
			_ = a.Trace.Append("turn_end", "", text, trace.Bool(true), 0)
			msgs = stripDurableInternal(msgs)
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
			ran  bool
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
				res.FilesChanged = sortedChanged(changed)
				a.finishTurnUndo(&res, turnUndo, nativeWriteRan)
				return msgs, res, err
			}
			if dec == perm.Deny {
				taskBlocker = "permission needed"
				ev.OnToolEnd(call, "denied by user", nil)
				pending = append(pending, executed{call: call, req: req, text: "denied by user"})
				continue
			}
			if call.Name == "write_file" || call.Name == "edit_file" {
				if err := turnUndo.capture(req.Path); err != nil {
					captureErr := fmt.Errorf("cannot safely checkpoint %s: %w", req.Path, err)
					taskBlocker = "checkpoint capture failed"
					ev.OnToolEnd(call, "", captureErr)
					pending = append(pending, executed{call: call, req: req, text: "error: " + captureErr.Error(), err: captureErr})
					continue
				}
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
				pending[i].ran = true
				var outText string
				var err error
				if a.runTool != nil {
					outText, err = a.runTool(ctx, call, tool, a.Tools.Ctx)
				} else {
					outText, err = tool.Run(ctx, call.Arguments, a.Tools.Ctx)
				}
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
			if ex.ran && (ex.call.Name == "write_file" || ex.call.Name == "edit_file") {
				nativeWriteRan = true
			}
			if ex.call.Name == "verify" {
				a.noteTaskVerification(ex.text)
			}
			if toolWriteSucceeded(ex.call.Name, ex.req.Path, ex.text, ex.err) {
				if p := strings.TrimSpace(ex.req.Path); p != "" {
					changed[p] = struct{}{}
				}
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
		// TokenTamer every round — don't wait for message count / budget critical.
		compactMsgs, stats, _ := ctxmgr.Manage(ctx, a.LLM, a.CFG.Model, msgs, budget)
		msgs = compactMsgs
		res.Context = stats
	}
	err := fmt.Errorf("stopped after %d tool rounds (limit)", a.CFG.MaxToolRounds)
	res.FilesChanged = sortedChanged(changed)
	a.finishTurnUndo(&res, turnUndo, nativeWriteRan)
	a.blockDurableTask("task budget exhausted")
	res.Task = a.TaskSnapshot()
	ev.OnError(err)
	msgs = stripDurableInternal(msgs)
	final, stats, _ := ctxmgr.Manage(ctx, a.LLM, a.CFG.Model, msgs, budget)
	res.Context = stats
	return final, res, err
}

func (a *Agent) maybeVerify(ctx context.Context, ev EventHandler, filesChanged []string, already bool) string {
	if already || len(filesChanged) == 0 || a.Tools == nil || (a.Tools.Ctx.Verify == nil && a.Tools.Ctx.VerifyTargets == nil) {
		return ""
	}
	tool, ok := a.Tools.Get("verify")
	if !ok {
		return ""
	}
	args, _ := json.Marshal(map[string]any{"targets": filesChanged})
	call := llm.ToolCall{ID: "verify-auto", Name: "verify", Arguments: string(args)}
	ev.OnToolStart(call)
	req := tool.Permission(call.Arguments, a.Tools.Ctx)
	req.Hint = perm.EnrichHint(req, call.Arguments)
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
	out, err := tool.Run(ctx, call.Arguments, a.Tools.Ctx)
	if err != nil {
		out = "error: " + err.Error()
	}
	ev.OnToolEnd(call, out, err)
	okv := err == nil && strings.Contains(strings.ToLower(out), "pass") && !strings.Contains(strings.ToLower(out), "inconclusive")
	_ = a.Trace.Append("verify", "verify", out, &okv, 0)
	return out
}

type NopHandler struct{}

func (NopHandler) OnText(string)                         {}
func (NopHandler) OnTextDelta(string)                    {}
func (NopHandler) OnToolStart(llm.ToolCall)              {}
func (NopHandler) OnToolEnd(llm.ToolCall, string, error) {}
func (NopHandler) OnNeedPermission(context.Context, perm.Request) (perm.Decision, error) {
	return perm.Deny, nil
}
func (NopHandler) OnError(error) {}

func explainFooter(paths []string, undoAvailable bool, undoErr string) string {
	if len(paths) == 0 {
		return ""
	}
	undo := "Undo: /undo"
	if !undoAvailable {
		reason := strings.TrimSpace(undoErr)
		if reason == "" {
			reason = "checkpoint could not be sealed"
		}
		undo = "Undo: unavailable — " + reason
	}
	return "Changed: " + strings.Join(paths, ", ") + "\nRun: check the files above\n" + undo
}

func normalizeExplainFooter(text string, paths []string, undoAvailable bool, undoErr string) string {
	text = stripExplainFooter(strings.TrimSpace(text))
	footer := explainFooter(paths, undoAvailable, undoErr)
	if footer == "" {
		return text
	}
	if text == "" {
		return footer
	}
	return text + "\n\n" + footer
}

func stripExplainFooter(text string) string {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(lines[i])), "changed:") {
			continue
		}
		seenRun, seenUndo := false, false
		for _, line := range lines[i+1:] {
			low := strings.ToLower(strings.TrimSpace(line))
			seenRun = seenRun || strings.HasPrefix(low, "run:")
			seenUndo = seenUndo || strings.HasPrefix(low, "undo:")
		}
		if seenRun && seenUndo {
			return strings.TrimSpace(strings.Join(lines[:i], "\n"))
		}
	}
	return text
}

func sortedChanged(changed map[string]struct{}) []string {
	paths := make([]string, 0, len(changed))
	for path := range changed {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

// toolWriteSucceeded reports whether a write/edit tool call actually ran and
// succeeded. Denied, blocked, or failed calls must not enter FilesChanged or
// the "Changed:" footer (and must not trigger auto-verify).
func toolWriteSucceeded(name, path, text string, err error) bool {
	if name != "write_file" && name != "edit_file" {
		return false
	}
	if err != nil {
		return false
	}
	if strings.TrimSpace(path) == "" {
		return false
	}
	t := strings.TrimSpace(text)
	if t == "" || t == "denied by user" {
		return false
	}
	if strings.HasPrefix(t, "error:") {
		return false
	}
	// Task-mode / policy blocks set a reason string without running the tool.
	low := strings.ToLower(t)
	if strings.Contains(low, "not allowed") || strings.Contains(low, "read-only") || strings.Contains(low, "blocked") {
		return false
	}
	return true
}

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
	if strings.Contains(low, "opencode") || strings.Contains(low, "zen/go") || strings.Contains(low, "open code") {
		fix = "run `opencode auth login` for Zen and/or Go, then pick a model from that plan (Zen free models need Zen)."
	}
	if strings.Contains(low, "antigravity") || strings.Contains(low, "gemini_api_key") {
		fix = "run `agy` to sign in, or set GEMINI_API_KEY, then pick an Antigravity model."
	}
	return fmt.Errorf("Problem: %s.\nCause:   %s.\nFix:     %s", problem, cause, fix)
}
