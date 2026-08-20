package ctxmgr

import (
	"testing"

	"github.com/saiaathish/picogent/internal/llm"
)

func TestMicroCompactMasksOldTools(t *testing.T) {
	long := stringsRepeat("x", 3000)
	msgs := []llm.Message{
		{Role: "system", Content: "sys"},
		{Role: "tool", Content: long},
		{Role: "user", Content: "a"},
		{Role: "assistant", Content: "b"},
		{Role: "user", Content: "c"},
		{Role: "assistant", Content: "d"},
		{Role: "user", Content: "e"},
		{Role: "assistant", Content: "f"},
		{Role: "tool", Content: long},
	}
	out := MicroCompact(msgs)
	if !stringsContains(out[1].Content, "compacted") {
		t.Fatalf("old tool should be masked: %q", out[1].Content[:40])
	}
	if stringsContains(out[8].Content, "compacted") {
		t.Fatal("recent tool should stay intact")
	}
}

func TestStatsForLevels(t *testing.T) {
	budget := 1000
	msgs := []llm.Message{{Role: "user", Content: stringsRepeat("word ", 900)}}
	st := StatsFor(msgs, budget)
	if st.Level != "critical" {
		t.Fatalf("level=%s pct=%f", st.Level, st.Pct)
	}
}

func stringsRepeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}

func stringsContains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
