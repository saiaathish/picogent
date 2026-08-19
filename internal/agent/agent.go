package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/tools"
)

const systemPrompt = `You are Picogent, a small coding agent. You work in one project folder.

Rules:
- Inspect with glob/grep/read_file before you edit.
- Prefer edit_file for small changes. Use write_file for new files.
- Keep diffs small. Do not invent files you have not seen.
- Never git push. Never run destructive commands unless the user asked.
- After any file change, end with three short lines:
  Changed: ...
  Run: ...
  Undo: ... (git checkout -- <file> or delete the new file)
Be direct. No filler.`

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
	CFG   config.Config
	LLM   llm.Client
	Tools *tools.Registry
	Gate  *perm.Gate
}

func New(cfg config.Config, client llm.Client, reg *tools.Registry, gate *perm.Gate) *Agent {
	return &Agent{CFG: cfg, LLM: client, Tools: reg, Gate: gate}
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
		msgs = append(msgs, llm.Message{Role: "system", Content: systemPrompt})
	}
	msgs = append(msgs, history...)
	msgs = append(msgs, llm.Message{Role: "user", Content: user})
	msgs = compact(msgs)

	var res Result
	changed := map[string]struct{}{}

	for round := 0; round < a.CFG.MaxToolRounds; round++ {
		out, err := a.LLM.Chat(ctx, llm.ChatRequest{
			Model:    a.CFG.Model,
			Messages: msgs,
			Tools:    a.Tools.Specs(),
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
		for _, call := range msg.ToolCalls {
			ev.OnToolStart(call)
			tool, ok := a.Tools.Get(call.Name)
			if !ok {
				err := fmt.Errorf("unknown tool %s", call.Name)
				ev.OnToolEnd(call, "", err)
				msgs = append(msgs, llm.Message{Role: "tool", ToolCallID: call.ID, Name: call.Name, Content: err.Error()})
				continue
			}
			req := tool.Permission(call.Arguments, a.Tools.Ctx)
			dec, err := a.Gate.Check(ctx, req)
			if err != nil {
				ev.OnToolEnd(call, "", err)
				return msgs, res, err
			}
			if dec == perm.Deny {
				content := "denied by user"
				ev.OnToolEnd(call, content, nil)
				msgs = append(msgs, llm.Message{Role: "tool", ToolCallID: call.ID, Name: call.Name, Content: content})
				continue
			}
			outText, err := tool.Run(ctx, call.Arguments, a.Tools.Ctx)
			if err != nil {
				ev.OnToolEnd(call, outText, err)
				msgs = append(msgs, llm.Message{Role: "tool", ToolCallID: call.ID, Name: call.Name, Content: "error: " + err.Error()})
				continue
			}
			if call.Name == "write_file" || call.Name == "edit_file" {
				changed[req.Path] = struct{}{}
			}
			ev.OnToolEnd(call, outText, nil)
			msgs = append(msgs, llm.Message{Role: "tool", ToolCallID: call.ID, Name: call.Name, Content: outText})
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

func (NopHandler) OnText(string) {}
func (NopHandler) OnToolStart(llm.ToolCall) {}
func (NopHandler) OnToolEnd(llm.ToolCall, string, error) {}
func (NopHandler) OnNeedPermission(context.Context, perm.Request) (perm.Decision, error) {
	return perm.Deny, nil
}
func (NopHandler) OnError(error) {}

func userErr(problem string, err error) error {
	cause := err.Error()
	fix := "check your key, base URL, and model name."
	if strings.Contains(strings.ToLower(cause), "connection refused") || strings.Contains(cause, "11434") {
		fix = "run `ollama serve`, then `ollama pull` your model, and set provider: ollama."
	}
	return fmt.Errorf("Problem: %s.\nCause:   %s.\nFix:     %s", problem, cause, fix)
}
