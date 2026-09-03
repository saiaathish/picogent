package session

import (
	"fmt"
	"testing"

	"github.com/saiaathish/picogent/internal/llm"
)

func TestValueAwareRetentionKeepsLatestRequestAndNewestCompletePriorTurn(t *testing.T) {
	base := Session{ID: "retention", Title: "chat", Workspace: "workspace"}
	messages := []llm.Message{
		{Role: "user", Content: "completed request"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call-complete", Name: "inspect"}}},
		{Role: "tool", ToolCallID: "call-complete", Name: "inspect", Content: "complete result"},
		{Role: "assistant", Content: "completed response"},
		{Role: "user", Content: "latest request still running"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call-pending", Name: "inspect"}}},
	}

	got := boundedMessages(messages, base)
	if len(got) != 5 {
		t.Fatalf("retained %d messages, want 5: %#v", len(got), got)
	}
	if got[0].Content != "completed request" || got[3].Content != "completed response" || got[4].Content != "latest request still running" {
		t.Fatalf("retained transcript order/content = %#v", got)
	}
	if got[1].Role != "assistant" || len(got[1].ToolCalls) != 1 || got[2].Role != "tool" || got[2].ToolCallID != "call-complete" {
		t.Fatalf("complete tool exchange was not retained atomically: %#v", got)
	}
	for _, message := range got {
		if message.ToolCallID == "call-pending" {
			t.Fatalf("incomplete tool exchange leaked into retained history: %#v", got)
		}
	}
}

func TestValueAwareRetentionRanksCompleteToolUnitOverNewerPlainAssistantNoise(t *testing.T) {
	base := Session{ID: "retention", Title: "chat", Workspace: "workspace"}
	messages := []llm.Message{
		{Role: "user", Content: "older request with useful tool result"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call-useful", Name: "inspect"}}},
		{Role: "tool", ToolCallID: "call-useful", Name: "inspect", Content: "useful result"},
	}
	for i := 0; i < 160; i++ {
		messages = append(messages, llm.Message{Role: "assistant", Content: fmt.Sprintf("plain noise %03d", i)})
	}
	messages = append(messages,
		llm.Message{Role: "user", Content: "newest request"},
		llm.Message{Role: "assistant", Content: "newest response"},
	)

	got := boundedMessages(messages, base)
	if len(got) != MaxSessionMessages {
		t.Fatalf("retained %d messages, want message bound %d", len(got), MaxSessionMessages)
	}
	if len(got) < 2 {
		t.Fatalf("retained %d messages, want newest request and response", len(got))
	}
	usefulAssistant, usefulTool := false, false
	for _, message := range got {
		if len(message.ToolCalls) == 1 && message.ToolCalls[0].ID == "call-useful" {
			usefulAssistant = true
		}
		if message.ToolCallID == "call-useful" {
			usefulTool = true
		}
	}
	if !usefulAssistant || !usefulTool {
		t.Fatalf("value-aware ranking dropped complete useful pair: %#v", got[:min(len(got), 8)])
	}
	if got[len(got)-2].Content != "newest request" || got[len(got)-1].Content != "newest response" {
		t.Fatalf("newest complete turn was not restored at transcript tail: %#v", got[len(got)-2:])
	}
}

func TestValueAwareRetentionBoundsLargeLegacyInputAndKeepsLatestUser(t *testing.T) {
	base := Session{ID: "retention", Title: "chat", Workspace: "workspace"}
	messages := make([]llm.Message, 0, 400)
	for i := 0; i < 300; i++ {
		messages = append(messages,
			llm.Message{Role: "user", Content: fmt.Sprintf("request %03d", i)},
			llm.Message{Role: "assistant", Content: fmt.Sprintf("response %03d", i)},
		)
	}

	got := boundedMessages(messages, base)
	if len(got) == 0 || len(got) > MaxSessionMessages {
		t.Fatalf("retained message count=%d, want 1..%d", len(got), MaxSessionMessages)
	}
	if len(got) < 2 {
		t.Fatalf("retained %d messages, want latest request and response", len(got))
	}
	if got[len(got)-2].Content != "request 299" || got[len(got)-1].Content != "response 299" {
		t.Fatalf("latest complete turn was not retained: %#v", got[len(got)-2:])
	}
	for i, message := range got {
		if message.Role == "tool" && (i == 0 || got[i-1].Role != "assistant") {
			t.Fatalf("orphaned tool result at %d: %#v", i, got[i-1:])
		}
	}
}

func TestValueAwareRetentionUsesNewestPortionOfOversizedCompleteTurn(t *testing.T) {
	base := Session{ID: "retention", Title: "chat", Workspace: "workspace"}
	messages := []llm.Message{{Role: "user", Content: "large complete request"}}
	for i := 0; i < MaxSessionMessages+20; i++ {
		messages = append(messages, llm.Message{Role: "assistant", Content: fmt.Sprintf("response %03d", i)})
	}

	got := boundedMessages(messages, base)
	if len(got) != MaxSessionMessages {
		t.Fatalf("retained message count=%d, want %d", len(got), MaxSessionMessages)
	}
	if got[0].Content != "large complete request" {
		t.Fatalf("latest user request was dropped: %#v", got[:min(len(got), 3)])
	}
	if got[len(got)-1].Content != fmt.Sprintf("response %03d", MaxSessionMessages+19) {
		t.Fatalf("newest response=%q, want newest complete-turn response", got[len(got)-1].Content)
	}
}
