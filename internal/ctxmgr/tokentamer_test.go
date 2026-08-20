package ctxmgr

import (
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/llm"
)

func TestToolAwareCompactSkeletonizesStaleReads(t *testing.T) {
	body := "package main\n\nimport \"fmt\"\n\nfunc Hello() {\n\tfmt.Println(\"hello world this is a long body\")\n\tfor i := 0; i < 100; i++ {\n\t\tfmt.Println(i)\n\t}\n}\n\nfunc World() {\n\tfmt.Println(\"world\")\n}\n"
	// Pad so skeletonize threshold triggers.
	for len(body) < SkeletonMinChars+50 {
		body += "// padding line to exceed skeleton threshold\n"
	}
	msgs := []llm.Message{
		{Role: "system", Content: "sys"},
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{
				{ID: "c1", Name: "read_file", Arguments: `{"path":"a.go"}`},
			},
		},
		{Role: "tool", ToolCallID: "c1", Name: "read_file", Content: body},
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{
				{ID: "c2", Name: "read_file", Arguments: `{"path":"a.go"}`},
			},
		},
		{Role: "tool", ToolCallID: "c2", Name: "read_file", Content: body},
	}
	out := ToolAwareCompact(msgs)
	if !strings.HasPrefix(out[2].Content, "# skeleton:") {
		t.Fatalf("stale read should be skeletonized: %q", clip(out[2].Content, 80))
	}
	if strings.HasPrefix(out[4].Content, "# skeleton:") {
		t.Fatal("latest read of same path must stay intact")
	}
	if len(out[2].Content) >= len(body) {
		t.Fatalf("stale should be smaller: %d vs %d", len(out[2].Content), len(body))
	}
}

func TestToolAwareCompactClipsOldBash(t *testing.T) {
	long := strings.Repeat("line of bash output\n", 400)
	msgs := []llm.Message{
		{Role: "tool", Name: "bash", Content: long},
		{Role: "tool", Name: "bash", Content: long},
		{Role: "tool", Name: "bash", Content: long},
		{Role: "tool", Name: "bash", Content: long},
		{Role: "tool", Name: "bash", Content: long},
		{Role: "tool", Name: "bash", Content: long},
	}
	out := ToolAwareCompact(msgs)
	if len(out[0].Content) >= len(long) {
		t.Fatalf("old bash should clip: %d", len(out[0].Content))
	}
}
