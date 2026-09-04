package ctxmgr

import (
	"fmt"
	"sort"
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

	maxDigestedToolChars   = 900
	maxDigestedToolSignals = 6
	maxDigestedToolLine    = 220
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
	name = flattenDigestLine(strings.TrimSpace(name), 80)
	if name == "" {
		name = "tool"
	}
	lines := strings.Count(content, "\n") + 1
	runes := utf8.RuneCountInString(content)
	first := firstNonEmptyLine(content)
	if first == "" {
		first = "(empty)"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "[digested %s · ~%d chars · %d lines · untrusted tool output]\n", name, runes, lines)
	b.WriteString("summary: ")
	b.WriteString(flattenDigestLine(first, maxDigestedToolLine))

	for _, signal := range toolSignalLines(content, first) {
		b.WriteString("\nsignal: ")
		b.WriteString(flattenDigestLine(signal, maxDigestedToolLine))
	}
	return clipDigest(b.String(), maxDigestedToolChars)
}

type toolSignal struct {
	line  string
	score int
	index int
}

// toolSignalLines retains high-value failure metadata before stale output is
// removed. It is deliberately lexical: it never claims that a line is true,
// and the caller labels every retained line as untrusted tool output.
func toolSignalLines(content, summary string) []string {
	const (
		strongSignal        = 3
		weakSignal          = 1
		maxSignalCandidates = maxDigestedToolSignals * 4
	)
	needles := []struct {
		text  string
		score int
	}{
		{"panic", strongSignal},
		{"fatal", strongSignal},
		{"traceback", strongSignal},
		{"command not found", strongSignal},
		{"no such file", strongSignal},
		{"exit status", strongSignal},
		{"error", weakSignal},
		{"exception", weakSignal},
		{"fail", weakSignal},
		{"timeout", weakSignal},
		{"timed out", weakSignal},
		{"undefined", weakSignal},
	}

	var candidates []toolSignal
	start := 0
	for index := 0; start <= len(content); index++ {
		relativeEnd := strings.IndexByte(content[start:], '\n')
		end := len(content)
		if relativeEnd >= 0 {
			end = start + relativeEnd
		}
		line := strings.TrimSpace(content[start:end])
		if line == "" || line == summary {
			if relativeEnd < 0 {
				break
			}
			start = end + 1
			continue
		}
		score := 0
		for _, needle := range needles {
			if containsFold(line, needle.text) {
				score += needle.score
			}
		}
		if hasPrefixFold(line, "fail") || containsFold(line, " failed ") {
			score++
		}
		if score > 0 {
			candidate := toolSignal{line: line, score: score, index: index}
			if len(candidates) < maxSignalCandidates {
				candidates = append(candidates, candidate)
			} else {
				weakest := 0
				for i := 1; i < len(candidates); i++ {
					if candidates[i].score < candidates[weakest].score ||
						(candidates[i].score == candidates[weakest].score && candidates[i].index > candidates[weakest].index) {
						weakest = i
					}
				}
				if candidate.score > candidates[weakest].score {
					candidates[weakest] = candidate
				}
			}
		}
		if relativeEnd < 0 {
			break
		}
		start = end + 1
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].index < candidates[j].index
	})

	seen := make(map[string]struct{}, maxDigestedToolSignals)
	out := make([]string, 0, maxDigestedToolSignals)
	for _, candidate := range candidates {
		line := flattenDigestLine(candidate.line, maxDigestedToolLine)
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		out = append(out, line)
		if len(out) == maxDigestedToolSignals {
			break
		}
	}
	return out
}

func flattenDigestLine(line string, limit int) string {
	line = strings.TrimSpace(line)
	if limit <= 0 || line == "" {
		return line
	}
	// Most tool output is already single-space text. Avoid Fields/Join and
	// scanning the whole line on this hot path; retain full whitespace
	// normalization for unusual output below.
	if !strings.ContainsAny(line, "\t\r\n") && !strings.Contains(line, "  ") {
		return clipRunes(line, limit)
	}
	return clipRunes(strings.Join(strings.Fields(line), " "), limit)
}

func clipRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	index := 0
	for count := 0; count < limit && index < len(value); count++ {
		_, size := utf8.DecodeRuneInString(value[index:])
		index += size
	}
	if index == len(value) {
		return value
	}
	if limit == 1 {
		return "…"
	}
	index = 0
	for count := 0; count < limit-1 && index < len(value); count++ {
		_, size := utf8.DecodeRuneInString(value[index:])
		index += size
	}
	return value[:index] + "…"
}

func containsFold(value, needle string) bool {
	if needle == "" {
		return true
	}
	if len(needle) > len(value) {
		return false
	}
	for start := 0; start <= len(value)-len(needle); start++ {
		matched := true
		for i := range needle {
			if asciiFold(value[start+i]) != asciiFold(needle[i]) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func hasPrefixFold(value, prefix string) bool {
	if len(value) < len(prefix) {
		return false
	}
	return containsFold(value[:len(prefix)], prefix)
}

func asciiFold(value byte) byte {
	if value >= 'A' && value <= 'Z' {
		return value + ('a' - 'A')
	}
	return value
}

func clipDigest(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	marker := "\n… [digest clipped] …"
	if limit <= len(marker) {
		return safeDigestPrefix(value, limit)
	}
	return safeDigestPrefix(value, limit-len(marker)) + marker
}

func safeDigestPrefix(value string, limit int) string {
	if limit >= len(value) {
		return value
	}
	if limit <= 0 {
		return ""
	}
	for limit > 0 && !utf8.RuneStart(value[limit]) {
		limit--
	}
	return value[:limit]
}

func firstNonEmptyLine(s string) string {
	for {
		end := strings.IndexByte(s, '\n')
		line := s
		if end >= 0 {
			line = s[:end]
		}
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
		if end < 0 {
			return ""
		}
		s = s[end+1:]
	}
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
		out = ValueAwareWindow(out, KeepAfterCompact)
		method = "soft-window"
	}
	if EstimateTokens(out) <= soft {
		return out, method
	}

	if len(out) > KeepRecent+2 {
		out = ValueAwareWindow(out, KeepRecent+2)
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
