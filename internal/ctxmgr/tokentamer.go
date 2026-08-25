package ctxmgr

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/saiaathish/picogent/internal/llm"
)

// TokenTamer-style + Headroom-inspired compression (no external proxy needed):
//   - Keep the latest read of each file intact
//   - Skeletonize older duplicate file reads and huge tool dumps
//   - Cap bash/grep/list noise while preserving signal
//
// Refs: ayala3/TokenTamer, buildparafin/headroom, lm-sys/RouteLLM (routing half)

const (
	// Tighter ingress caps so each tool round adds less to the soft working set
	// (Headroom/tooltrim: compress before the transcript bloats).
	KeepRecentTools   = 3
	StaleToolMaxChars = 160
	FreshToolMaxChars = 3500
	BashMaxChars      = 1400
	SearchMaxChars    = 2000
	SkeletonMinChars  = 700
)

var (
	reFuncSig = regexp.MustCompile(`(?m)^((?:export\s+)?(?:async\s+)?(?:function\s+\w+|func\s+(?:\([^)]*\)\s*)?\w+|def\s+\w+|class\s+\w+|type\s+\w+|interface\s+\w+|struct\s+\w+|const\s+\w+|let\s+\w+|var\s+\w+|import\s+.+|package\s+\w+|from\s+.+|using\s+.+|#include\s+.+))`)
)

// ToolAwareCompact applies TokenTamer-style stale-read skeletonization every round.
func ToolAwareCompact(msgs []llm.Message) []llm.Message {
	if len(msgs) == 0 {
		return msgs
	}
	out := make([]llm.Message, len(msgs))
	copy(out, msgs)

	// Map tool_call_id → path (from preceding assistant tool_calls args).
	pathByCall := map[string]string{}
	for _, m := range out {
		if m.Role != "assistant" {
			continue
		}
		for _, tc := range m.ToolCalls {
			if p := pathFromArgs(tc.Name, tc.Arguments); p != "" {
				pathByCall[tc.ID] = p
			}
		}
	}

	// Find last index of each file path among tool results.
	lastByPath := map[string]int{}
	toolIdx := []int{}
	for i, m := range out {
		if m.Role != "tool" {
			continue
		}
		toolIdx = append(toolIdx, i)
		if p := pathByCall[m.ToolCallID]; p != "" {
			lastByPath[p] = i
		}
	}

	keepFrom := 0
	if len(toolIdx) > KeepRecentTools {
		keepFrom = len(toolIdx) - KeepRecentTools
	}

	for n, i := range toolIdx {
		m := &out[i]
		path := pathByCall[m.ToolCallID]
		freshByPath := path != "" && lastByPath[path] == i
		recent := n >= keepFrom

		name := strings.ToLower(m.Name)
		content := m.Content

		switch {
		case path != "" && !freshByPath:
			// Stale file read — skeletonize (TokenTamer core trick).
			m.Content = skeletonize(content, path)
		case path != "" && freshByPath:
			// Latest read of this path stays intact (even if also "recent").
			if len(content) > FreshToolMaxChars*3 {
				m.Content = clipTool(content, FreshToolMaxChars*3, true)
			}
		case isReadLike(name) && !recent && len(content) > SkeletonMinChars:
			m.Content = skeletonize(content, path)
		case isBashLike(name):
			if recent {
				m.Content = clipTool(content, BashMaxChars, true)
			} else {
				m.Content = digestTool(name, content)
			}
		case isSearchLike(name):
			if recent {
				m.Content = clipTool(content, SearchMaxChars, true)
			} else {
				m.Content = digestTool(name, content)
			}
		case !recent && len(content) > FreshToolMaxChars:
			m.Content = digestTool(name, content)
		case recent && len(content) > FreshToolMaxChars*2:
			m.Content = clipTool(content, FreshToolMaxChars*2, true)
		}
	}
	return out
}

func isReadLike(name string) bool {
	return name == "read_file" || strings.Contains(name, "read")
}

func isBashLike(name string) bool {
	return name == "bash" || name == "git" || name == "shell"
}

func isSearchLike(name string) bool {
	return name == "grep" || name == "glob" || name == "list_dir" || name == "web_fetch"
}

func pathFromArgs(toolName, args string) string {
	toolName = strings.ToLower(toolName)
	if toolName != "read_file" && toolName != "write_file" && toolName != "edit_file" {
		return ""
	}
	var in struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(args), &in); err != nil {
		return ""
	}
	p := strings.TrimSpace(in.Path)
	if p == "" {
		return ""
	}
	return filepath.Clean(p)
}

func clipTool(s string, max int, recent bool) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	if !recent {
		max = minInt(max, StaleToolMaxChars)
	}
	head := max / 3
	if head < 80 {
		head = 80
	}
	tail := max - head - 40
	if tail < 40 {
		tail = 40
	}
	if head+tail >= len(s) {
		return s[:max] + "\n… [tool output clipped] …"
	}
	return s[:head] + "\n… [tool output clipped · TokenTamer] …\n" + s[len(s)-tail:]
}

// skeletonize keeps imports/signatures and drops function bodies (AST-lite).
func skeletonize(src, path string) string {
	if len(src) < SkeletonMinChars {
		return clipTool(src, StaleToolMaxChars, false)
	}
	lines := strings.Split(src, "\n")
	var b strings.Builder
	if path != "" {
		b.WriteString("# skeleton: ")
		b.WriteString(path)
		b.WriteString(" (stale read compressed)\n")
	} else {
		b.WriteString("# skeleton (stale tool output compressed)\n")
	}

	kept := 0
	inBody := 0
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		// Track simple brace depth to skip bodies.
		open := strings.Count(line, "{")
		closeN := strings.Count(line, "}")
		sig := reFuncSig.MatchString(line) ||
			strings.HasPrefix(trim, "import ") ||
			strings.HasPrefix(trim, "from ") ||
			strings.HasPrefix(trim, "package ") ||
			strings.HasPrefix(trim, "using ") ||
			strings.HasPrefix(trim, "#include") ||
			strings.HasPrefix(trim, "//") ||
			strings.HasPrefix(trim, "# ") ||
			strings.HasPrefix(trim, "type ") ||
			strings.HasPrefix(trim, "interface ") ||
			strings.HasPrefix(trim, "export ")

		if inBody > 0 {
			inBody += open - closeN
			if inBody < 0 {
				inBody = 0
			}
			continue
		}
		if sig {
			b.WriteString(strings.TrimRight(line, "\r"))
			if open > closeN {
				b.WriteString(" { /* … */ }")
				inBody = open - closeN
			}
			b.WriteByte('\n')
			kept++
			if kept >= 80 {
				break
			}
			continue
		}
		// Keep short top-level decls; skip long bodies/data.
		if len(trim) < 120 && !strings.Contains(trim, " = ") {
			b.WriteString(line)
			b.WriteByte('\n')
			kept++
		}
	}
	if kept == 0 {
		return clipTool(src, StaleToolMaxChars, false)
	}
	out := b.String()
	if utf8.RuneCountInString(out) > 1800 {
		return clipTool(out, 1800, false)
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
