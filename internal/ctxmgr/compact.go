// Package ctxmgr implements tiered context-window compaction (micro-mask → soft-fit → summarize → truncate).
// Pattern inspired by Microsoft Agent Framework TokenBudgetComposedStrategy, TokenTamer, and Headroom.
package ctxmgr

import (
	"context"
	"fmt"
	"strings"

	"github.com/saiaathish/picogent/internal/llm"
)

const (
	// DefaultBudget is the Codex hard ceiling (capacity). SoftTarget keeps the live set far below this.
	DefaultBudget = 256_000
	// WarningPct / AutoCompactPct are relative to the hard ceiling — soft-fit already holds growth down.
	WarningPct       = 0.35
	AutoCompactPct   = 0.48
	KeepRecent       = 6
	KeepAfterCompact = 10
	ToolKeepChars    = 500
	ToolMaskChars    = 48
	// maxSummaryInputBytes keeps the provider-bound summarization request well
	// below the main context budget while retaining both ends of the older log.
	maxSummaryInputBytes = 32 * 1024
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
	case strings.Contains(m, "luna"), strings.Contains(m, "terra"), strings.Contains(m, "sol"),
		strings.Contains(m, "soul"), strings.Contains(m, "codex"), strings.Contains(m, "gpt-5"),
		strings.Contains(m, "gpt-4"), strings.HasPrefix(m, "o3"), strings.HasPrefix(m, "o4"):
		return 256_000
	case strings.Contains(m, "opus"), strings.Contains(m, "fable"):
		return 200_000
	case strings.Contains(m, "sonnet"):
		return 200_000
	case strings.Contains(m, "haiku"), strings.Contains(m, "mini"):
		return 128_000
	default:
		// Picogent is Codex-first — default to the Codex 256k window.
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
	const keepRecent = 4
	for i := range out {
		if out[i].Role != "tool" {
			continue
		}
		if len(out) <= keepRecent || i >= len(out)-keepRecent {
			continue
		}
		c := out[i].Content
		if alreadyDigested(c) || len(c) <= ToolKeepChars {
			continue
		}
		head := c
		if len(head) > ToolMaskChars {
			head = c[:ToolMaskChars]
		}
		out[i].Content = head + "\n… [tool output compacted] …"
	}
	return out
}

func TruncateTail(msgs []llm.Message, keep int) []llm.Message {
	if len(msgs) == 0 || keep <= 0 {
		return nil
	}
	if len(msgs) <= keep {
		return toolPairSafeTail(msgs)
	}
	head := msgs[0]
	if head.Role != "system" {
		head = llm.Message{}
	}
	start := len(msgs) - keep
	if head.Role == "system" {
		start++
	}
	start = backfillToolCallStart(msgs, start)
	tail := toolPairSafeTail(msgs[start:])
	if head.Role != "system" {
		return tail
	}
	out := make([]llm.Message, 0, len(tail)+1)
	out = append(out, head)
	out = append(out, tail...)
	return out
}

const summaryPrompt = `Summarize this conversation for an AI coding agent continuing the work.
Use short sections:
Goal: (one line)
Done: (bullets)
Files: (paths touched)
Open: (unresolved items, if any)
Keep facts, decisions, and errors. No filler.`

const summaryInputOmission = "\n[… middle summary input omitted …]\n"

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
	body := boundSummaryInput(strings.TrimSpace(b.String()))
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

func boundSummaryInput(body string) string {
	if len(body) <= maxSummaryInputBytes {
		return body
	}
	available := maxSummaryInputBytes - len(summaryInputOmission)
	prefixLimit := available / 2
	suffixLimit := available - prefixLimit
	prefixEnd := strings.LastIndexByte(body[:prefixLimit], '\n')
	if prefixEnd <= 0 {
		prefixEnd = prefixLimit
	}
	suffixStart := len(body) - suffixLimit
	if newline := strings.IndexByte(body[suffixStart:], '\n'); newline >= 0 {
		suffixStart += newline + 1
	}
	return body[:prefixEnd] + summaryInputOmission + body[suffixStart:]
}

// Manage applies tiered compaction every round so context grows slowly under a soft target
// while preserving a large hard ceiling (256k for Codex).
func Manage(ctx context.Context, client llm.Client, model string, msgs []llm.Message, budget int) ([]llm.Message, Stats, error) {
	if budget <= 0 {
		budget = DefaultBudget
	}
	before := EstimateTokens(msgs)

	// Tier 0 — always: TokenTamer + micro-mask + digest (cheap, every round).
	out := ToolAwareCompact(msgs)
	out = DeduplicateToolResults(out)
	out = MicroCompact(out)
	out = DigestStaleTools(out)

	soft := SoftTarget(budget)
	method := ""
	if EstimateTokens(out) > soft {
		var softMethod string
		out, softMethod = SoftFit(out, soft)
		method = softMethod
	} else if EstimateTokens(out) < before {
		method = "tokentamer"
	}

	st := StatsFor(out, budget)
	if method != "" && EstimateTokens(out) < before {
		st.Compacted = true
		st.Method = method
	}

	// Under auto-compact threshold: optional light window if the transcript is long.
	if st.Pct < SoftTrimPct {
		if len(out) > 24 {
			out = TruncateTail(out, 18)
			st = StatsFor(out, budget)
			st.Compacted = true
			if method == "" {
				st.Method = "truncate+tame"
			} else {
				st.Method = method + "+truncate"
			}
		}
		return out, st, nil
	}

	if st.Pct < AutoCompactPct {
		if len(out) > KeepAfterCompact+1 {
			out = TruncateTail(out, KeepAfterCompact)
			st = StatsFor(out, budget)
			st.Compacted = true
			st.Method = "window+soft"
		}
		return out, st, nil
	}

	// Tier 2: sliding window + tool trim near hard pressure
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
