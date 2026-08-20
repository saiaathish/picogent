package ctxmgr

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/saiaathish/picogent/internal/llm"
)

// Soft-budget / Headroom-inspired gradual growth:
//   Hard ceiling (256k for Codex) is capacity, not a target.
//   SoftTarget keeps the live working set small so the ring grows slowly
//   even across long tool loops — Cursor-style, not "fill until critical".
//
// Refs researched: borhen68/TokenTamer, buildparafin/headroom, tooltrim,
// thedotmack/claude-mem (session memory vs raw transcript dump).

const (
	// SoftTargetPct is the preferred working-set size vs hard budget (~41k of 256k).
	SoftTargetPct = 0.16
	// SoftTrimPct triggers stronger windowing before true auto-compact.
	SoftTrimPct = 0.24
)

func SoftTarget(budget int) int {
	if budget <= 0 {
		budget = DefaultBudget
	}
	n := int(float64(budget) * SoftTargetPct)
	if n < 12_000 {
		return 12_000
	}
	return n
}

// DigestStaleTools collapses older tool dumps into one-line facts (Headroom/tooltrim).
// Keeps the newest KeepRecentTools results intact for the live turn.
func DigestStaleTools(msgs []llm.Message) []llm.Message {
	if len(msgs) == 0 {
		return msgs
	}
	out := make([]llm.Message, len(msgs))
	copy(out, msgs)

	toolIdx := make([]int, 0, 16)
	for i, m := range out {
		if m.Role == "tool" {
			toolIdx = append(toolIdx, i)
		}
	}
	keepFrom := 0
	if len(toolIdx) > KeepRecentTools {
		keepFrom = len(toolIdx) - KeepRecentTools
	}
	for n, i := range toolIdx {
		if n >= keepFrom {
			continue
		}
		m := &out[i]
		if alreadyDigested(m.Content) || len(m.Content) <= 180 {
			continue
		}
		m.Content = digestTool(m.Name, m.Content)
	}
	return out
}

func alreadyDigested(s string) bool {
	return strings.HasPrefix(s, "[digested ") || strings.HasPrefix(s, "# skeleton:") || strings.Contains(s, "[tool output compacted]")
}

func digestTool(name, content string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "tool"
	}
	lines := strings.Count(content, "\n") + 1
	runes := utf8.RuneCountInString(content)
	first := firstNonEmptyLine(content)
	if first == "" {
		first = "(empty)"
	}
	if utf8.RuneCountInString(first) > 100 {
		r := []rune(first)
		first = string(r[:100]) + "…"
	}
	return fmt.Sprintf("[digested %s · ~%d chars · %d lines] %s", name, runes, lines, first)
}

func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

// SoftFit repeatedly applies cheap compressors until under soft target (or stuck).
func SoftFit(msgs []llm.Message, soft int) ([]llm.Message, string) {
	if soft <= 0 {
		soft = SoftTarget(DefaultBudget)
	}
	out := msgs
	method := ""
	before := EstimateTokens(out)
	if before <= soft {
		return out, ""
	}

	out = DigestStaleTools(out)
	out = tightenStaleTools(out)
	out = MicroCompact(out)
	if EstimateTokens(out) < before {
		method = "soft-digest"
	}
	if EstimateTokens(out) <= soft {
		return out, method
	}

	if len(out) > KeepAfterCompact+1 {
		out = TruncateTail(out, KeepAfterCompact)
		method = "soft-window"
	}
	if EstimateTokens(out) <= soft {
		return out, method
	}

	if len(out) > KeepRecent+2 {
		out = TruncateTail(out, KeepRecent+2)
		method = "soft-trim"
	}
	return out, method
}

// tightenStaleTools applies a harder char cap to everything outside the recent tool window.
func tightenStaleTools(msgs []llm.Message) []llm.Message {
	if len(msgs) == 0 {
		return msgs
	}
	out := make([]llm.Message, len(msgs))
	copy(out, msgs)
	toolIdx := make([]int, 0, 16)
	for i, m := range out {
		if m.Role == "tool" {
			toolIdx = append(toolIdx, i)
		}
	}
	keepFrom := 0
	if len(toolIdx) > KeepRecentTools {
		keepFrom = len(toolIdx) - KeepRecentTools
	}
	const hardCap = 120
	for n, i := range toolIdx {
		if n >= keepFrom {
			continue
		}
		m := &out[i]
		if alreadyDigested(m.Content) {
			continue
		}
		if len(m.Content) > hardCap {
			m.Content = clipTool(m.Content, hardCap, false)
		}
	}
	return out
}
