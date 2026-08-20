// Package ctxmgr implements tiered context-window compaction (micro-mask → summarize → truncate).
// Pattern inspired by Microsoft Agent Framework TokenBudgetComposedStrategy and contextkit.
package ctxmgr

import (
	"context"
	"fmt"
	"strings"

	"github.com/saiaathish/picogent/internal/llm"
)

const (
	DefaultBudget   = 128_000
	WarningPct      = 0.72
	AutoCompactPct  = 0.82
	KeepRecent      = 12
	KeepAfterCompact = 20
	ToolKeepChars   = 1200
	ToolMaskChars   = 80
)

type Stats struct {
	Tokens    int     `json:"tokens"`
	Budget    int     `json:"budget"`
	Pct       float64 `json:"pct"`
	Level     string  `json:"level"` // ok, warning, critical
	Compacted bool    `json:"compacted"`
	Method    string  `json:"method,omitempty"`
}

func BudgetForModel(model string) int {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "opus"), strings.Contains(m, "fable"):
		return 200_000
	case strings.Contains(m, "sonnet"), strings.Contains(m, "terra"):
		return 128_000
	case strings.Contains(m, "haiku"), strings.Contains(m, "luna"), strings.Contains(m, "mini"):
		return 64_000
	default:
		return DefaultBudget
	}
}

func EstimateTokens(msgs []llm.Message) int {
	n := 0
	for _, m := range msgs {
		n += len(m.Content)/4 + 8
		for _, tc := range m.ToolCalls {
			n += len(tc.Name)/4 + len(tc.Arguments)/4 + 4
		}
		if m.Name != "" {
			n += len(m.Name) / 4
		}
	}
	if n < 32 {
		return 32
	}
	return n
}

func StatsFor(msgs []llm.Message, budget int) Stats {
	if budget <= 0 {
		budget = DefaultBudget
	}
	tok := EstimateTokens(msgs)
	pct := float64(tok) / float64(budget)
	level := "ok"
	switch {
	case pct >= AutoCompactPct:
		level = "critical"
	case pct >= WarningPct:
		level = "warning"
	}
	return Stats{Tokens: tok, Budget: budget, Pct: pct, Level: level}
}

func MicroCompact(msgs []llm.Message) []llm.Message {
	if len(msgs) == 0 {
		return msgs
	}
	out := make([]llm.Message, len(msgs))
	copy(out, msgs)
	const keepRecent = 6
	for i := range out {
		if out[i].Role != "tool" {
			continue
		}
		if len(out) <= keepRecent || i >= len(out)-keepRecent {
			continue
		}
		c := out[i].Content
		if len(c) <= ToolKeepChars {
			continue
		}
		head := c[:ToolMaskChars]
		out[i].Content = head + "\n… [tool output compacted] …"
	}
	return out
}

func TruncateTail(msgs []llm.Message, keep int) []llm.Message {
	if len(msgs) <= keep {
		return msgs
	}
	head := msgs[0]
	if head.Role != "system" {
		head = llm.Message{}
	}
	tail := msgs[len(msgs)-keep+1:]
	if head.Role == "system" {
		out := make([]llm.Message, 0, keep)
		out = append(out, head)
		out = append(out, tail...)
		return out
	}
	return tail
}

const summaryPrompt = `Summarize this conversation for an AI coding agent continuing the work.
Use short sections:
Goal: (one line)
Done: (bullets)
Files: (paths touched)
Open: (unresolved items, if any)
Keep facts, decisions, and errors. No filler.`

func Summarize(ctx context.Context, client llm.Client, model string, msgs []llm.Message) (string, error) {
	if client == nil {
		return "", fmt.Errorf("no LLM client for summarization")
	}
	var b strings.Builder
	for _, m := range msgs {
		switch m.Role {
		case "system":
			continue
		case "user", "assistant":
			if t := strings.TrimSpace(m.Content); t != "" {
				fmt.Fprintf(&b, "%s: %s\n", m.Role, clip(t, 2000))
			}
		case "tool":
			name := m.Name
			if name == "" {
				name = "tool"
			}
			fmt.Fprintf(&b, "tool(%s): %s\n", name, clip(m.Content, 400))
		}
	}
	body := strings.TrimSpace(b.String())
	if body == "" {
		return "", fmt.Errorf("nothing to summarize")
	}
	out, err := client.Chat(ctx, llm.ChatRequest{
		Model: model,
		Messages: []llm.Message{
			{Role: "system", Content: summaryPrompt},
			{Role: "user", Content: body},
		},
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out.Message.Content), nil
}

// Manage applies tiered compaction when approaching the token budget.
func Manage(ctx context.Context, client llm.Client, model string, msgs []llm.Message, budget int) ([]llm.Message, Stats, error) {
	if budget <= 0 {
		budget = DefaultBudget
	}
	out := MicroCompact(msgs)
	st := StatsFor(out, budget)

	if st.Pct < AutoCompactPct {
		if len(out) > 40 {
			out = TruncateTail(out, 30)
			st = StatsFor(out, budget)
			st.Compacted = true
			st.Method = "truncate"
		}
		return out, st, nil
	}

	// Tier 2: sliding window + tool trim
	if len(out) > KeepAfterCompact+1 {
		out = TruncateTail(out, KeepAfterCompact)
		st = StatsFor(out, budget)
		st.Compacted = true
		st.Method = "window"
	}
	if st.Pct < AutoCompactPct {
		return out, st, nil
	}

	// Tier 3: LLM summarization of older turns
	if len(out) <= KeepRecent+2 {
		return out, st, nil
	}
	head := out[0]
	start := 1
	if head.Role != "system" {
		head = llm.Message{}
		start = 0
	}
	old := out[start : len(out)-KeepRecent]
	recent := out[len(out)-KeepRecent:]

	summary, err := Summarize(ctx, client, model, old)
	if err != nil {
		out = TruncateTail(out, KeepRecent+1)
		st = StatsFor(out, budget)
		st.Compacted = true
		st.Method = "truncate"
		return out, st, nil
	}

	compact := []llm.Message{
		{Role: "user", Content: "[Earlier conversation summary]\n" + summary},
		{Role: "assistant", Content: "Understood. I'll continue from that context."},
	}
	out = make([]llm.Message, 0, len(recent)+len(compact)+1)
	if head.Role == "system" {
		out = append(out, head)
	}
	out = append(out, compact...)
	out = append(out, recent...)
	st = StatsFor(out, budget)
	st.Compacted = true
	st.Method = "summarize"
	return out, st, nil
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
