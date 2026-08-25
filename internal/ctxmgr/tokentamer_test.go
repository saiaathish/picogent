package ctxmgr

import (
	"fmt"
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

func TestToolAwareCompactRetainsFailureFromStaleBash(t *testing.T) {
	long := strings.Repeat("normal output\n", 120) + "FAIL internal/auth: TestLogin\n" + strings.Repeat("normal output\n", 120) + "exit status 1\n"
	msgs := []llm.Message{
		{Role: "tool", Name: "bash", Content: long},
		{Role: "tool", Name: "bash", Content: long},
		{Role: "tool", Name: "bash", Content: long},
		{Role: "tool", Name: "bash", Content: long},
		{Role: "tool", Name: "bash", Content: long},
		{Role: "tool", Name: "bash", Content: long},
	}
	out := ToolAwareCompact(msgs)
	if !strings.Contains(out[0].Content, "signal: FAIL internal/auth: TestLogin") {
		t.Fatalf("stale bash lost failure target: %q", out[0].Content)
	}
	if !strings.Contains(out[0].Content, "signal: exit status 1") {
		t.Fatalf("stale bash lost exit metadata: %q", out[0].Content)
	}
	if len(out[0].Content) > maxDigestedToolChars {
		t.Fatalf("stale bash digest length=%d, want <= %d", len(out[0].Content), maxDigestedToolChars)
	}
	if out[5].Content != clipTool(long, BashMaxChars, true) {
		t.Fatal("recent bash output changed outside its existing cap")
	}
}

func TestDeduplicateToolResultsKeepsLatestReadAndRepoMap(t *testing.T) {
	msgs := []llm.Message{
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "r1", Name: "read_file", Arguments: `{"path":"internal/a.go"}`}}},
		{Role: "tool", ToolCallID: "r1", Name: "read_file", Content: "old read"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "r2", Name: "read_file", Arguments: `{"path":"internal/a.go"}`}}},
		{Role: "tool", ToolCallID: "r2", Name: "read_file", Content: "latest read"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "m1", Name: "repo_map", Arguments: `{}`}}},
		{Role: "tool", ToolCallID: "m1", Name: "repo_map", Content: "old map"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "m2", Name: "repo_map", Arguments: `{}`}}},
		{Role: "tool", ToolCallID: "m2", Name: "repo_map", Content: "latest map"},
	}
	out := DeduplicateToolResults(msgs)
	if !strings.HasPrefix(out[1].Content, "[duplicate read_file result omitted") {
		t.Fatalf("old read was not compacted: %q", out[1].Content)
	}
	if out[3].Content != "latest read" || out[7].Content != "latest map" {
		t.Fatalf("latest results changed: read=%q map=%q", out[3].Content, out[7].Content)
	}
	if !strings.HasPrefix(out[5].Content, "[duplicate repo_map result omitted") {
		t.Fatalf("old repo map was not compacted: %q", out[5].Content)
	}
}

func TestTruncateTailPreservesToolCallPairs(t *testing.T) {
	msgs := []llm.Message{
		{Role: "system", Content: "system"},
		{Role: "user", Content: "first"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c1", Name: "read_file", Arguments: `{}`}}},
		{Role: "tool", ToolCallID: "c1", Name: "read_file", Content: "one"},
		{Role: "user", Content: "second"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c2", Name: "read_file", Arguments: `{}`}, {ID: "c3", Name: "grep", Arguments: `{}`}}},
		{Role: "tool", ToolCallID: "c2", Name: "read_file", Content: "two"},
		{Role: "tool", ToolCallID: "c3", Name: "grep", Content: "three"},
		{Role: "user", Content: "third"},
	}
	out := TruncateTail(msgs, 6)
	if len(out) == 0 || out[0].Role != "system" {
		t.Fatalf("system message was lost: %#v", out)
	}
	callIDs := map[string]bool{}
	resultIDs := map[string]bool{}
	for _, message := range out {
		if message.Role == "assistant" {
			for _, call := range message.ToolCalls {
				callIDs[call.ID] = true
			}
		}
		if message.Role == "tool" {
			resultIDs[message.ToolCallID] = true
		}
	}
	for id := range resultIDs {
		if !callIDs[id] {
			t.Fatalf("orphaned tool result %q in %#v", id, out)
		}
	}
	for id := range callIDs {
		if !resultIDs[id] {
			t.Fatalf("tool call %q lost its result in %#v", id, out)
		}
	}
}

func TestManageKeepsLongToolLoopBounded(t *testing.T) {
	long := strings.Repeat("exploration output that should not accumulate forever\n", 160)
	msgs := []llm.Message{{Role: "system", Content: "you are picogent"}}
	budget := BudgetForModel("gpt-5.6-terra")
	maxTokens := 0
	for i := 0; i < 80; i++ {
		id := fmt.Sprintf("loop-%d", i)
		msgs = append(msgs,
			llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: id, Name: "bash", Arguments: `{"cmd":"inspect"}`}}},
			llm.Message{Role: "tool", ToolCallID: id, Name: "bash", Content: long},
		)
		var stats Stats
		var err error
		msgs, stats, err = Manage(t.Context(), nil, "gpt-5.6-terra", msgs, budget)
		if err != nil {
			t.Fatal(err)
		}
		if stats.Tokens > maxTokens {
			maxTokens = stats.Tokens
		}
	}
	if maxTokens > SoftTarget(budget)*3 {
		t.Fatalf("long loop grew beyond bounded working set: max=%d soft=%d", maxTokens, SoftTarget(budget))
	}
	if len(msgs) == 0 || msgs[0].Role != "system" {
		t.Fatalf("system context was lost after long loop: %#v", msgs)
	}
	callIDs := map[string]bool{}
	resultIDs := map[string]bool{}
	for _, message := range msgs {
		if message.Role == "assistant" {
			for _, call := range message.ToolCalls {
				callIDs[call.ID] = true
			}
		}
		if message.Role == "tool" {
			resultIDs[message.ToolCallID] = true
		}
	}
	for id := range callIDs {
		if !resultIDs[id] {
			t.Fatalf("long loop retained call %q without a result", id)
		}
	}
	for id := range resultIDs {
		if !callIDs[id] {
			t.Fatalf("long loop retained result %q without a call", id)
		}
	}
}
