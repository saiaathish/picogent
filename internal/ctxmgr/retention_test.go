package ctxmgr

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/llm"
)

func TestValueAwareWindowRetainsHistoricalCompleteToolExchange(t *testing.T) {
	fixture := liveRetentionFixture()
	got := ValueAwareWindow(fixture, 10)

	if len(got) > 10 {
		t.Fatalf("window length=%d, want <= 10", len(got))
	}
	if len(got) == 0 || got[0].Role != "system" {
		t.Fatalf("system prompt was not preserved: %#v", got)
	}
	if !containsContextText(got, "latest request") {
		t.Fatalf("latest user request was not preserved: %#v", got)
	}
	if !containsContextToolPair(got, "retention-target") {
		t.Fatalf("value-aware window dropped the historical complete tool exchange: %#v", got)
	}
	if containsContextToolPair(TruncateTail(fixture, 10), "retention-target") {
		t.Fatal("recency-only control unexpectedly retained the historical target")
	}
	assertContextToolPairsComplete(t, got)
}

func TestValueAwareWindowDropsIncompleteCurrentToolUnit(t *testing.T) {
	fixture := []llm.Message{
		{Role: "system", Content: "system"},
		{Role: "user", Content: "older request"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "old", Name: "inspect"}}},
		{Role: "tool", ToolCallID: "old", Name: "inspect", Content: "old result"},
		{Role: "user", Content: "latest request"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "pending", Name: "inspect"}}},
	}

	got := ValueAwareWindow(fixture, 10)
	if !containsContextText(got, "latest request") {
		t.Fatalf("latest request was not retained: %#v", got)
	}
	if containsContextCall(got, "pending") {
		t.Fatalf("incomplete assistant tool call leaked into context: %#v", got)
	}
	assertContextToolPairsComplete(t, got)
}

func TestValueAwareWindowBoundsLargeCandidateProjection(t *testing.T) {
	fixture := make([]llm.Message, 0, 1+300*2)
	fixture = append(fixture, llm.Message{Role: "system", Content: "system"})
	for i := 0; i < 300; i++ {
		fixture = append(fixture,
			llm.Message{Role: "user", Content: fmt.Sprintf("request %03d", i)},
			llm.Message{Role: "assistant", Content: fmt.Sprintf("response %03d", i)},
		)
	}

	got := ValueAwareWindow(fixture, 10)
	if len(got) > 10 {
		t.Fatalf("window length=%d, want <= 10", len(got))
	}
	if !containsContextText(got, "request 299") {
		t.Fatalf("latest request was dropped after bounded projection: %#v", got)
	}
	if !containsContextText(got, "response 299") {
		t.Fatalf("latest complete response was dropped after bounded projection: %#v", got)
	}
}

func TestManageUsesValueAwareWindowAtLiveCompactionBoundary(t *testing.T) {
	fixture := liveRetentionFixture()
	got, stats, err := Manage(context.Background(), nil, "gpt-5.6-terra", fixture, DefaultBudget)
	if err != nil {
		t.Fatal(err)
	}
	if !stats.Compacted {
		t.Fatalf("stats=%+v, want live window compaction", stats)
	}
	if !containsContextToolPair(got, "retention-target") {
		t.Fatalf("live compaction dropped the historical target: %#v", got)
	}
	if containsContextToolPair(TruncateTail(fixture, 18), "retention-target") {
		t.Fatal("recency-only live control unexpectedly retained the historical target")
	}
	assertContextToolPairsComplete(t, got)
}

func liveRetentionFixture() []llm.Message {
	messages := []llm.Message{
		{Role: "system", Content: "system"},
		{Role: "user", Content: "historical constraint"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "retention-target", Name: "inspect", Arguments: `{}`}}},
		{Role: "tool", ToolCallID: "retention-target", Name: "inspect", Content: "historical result"},
		{Role: "assistant", Content: "historical response"},
	}
	for i := 0; i < 20; i++ {
		messages = append(messages,
			llm.Message{Role: "user", Content: fmt.Sprintf("noise request %02d", i)},
			llm.Message{Role: "assistant", Content: fmt.Sprintf("noise response %02d", i)},
		)
	}
	messages = append(messages,
		llm.Message{Role: "user", Content: "latest request"},
		llm.Message{Role: "assistant", Content: "latest response"},
	)
	return messages
}

func containsContextText(messages []llm.Message, want string) bool {
	for _, message := range messages {
		if strings.Contains(message.Content, want) {
			return true
		}
	}
	return false
}

func containsContextCall(messages []llm.Message, id string) bool {
	for _, message := range messages {
		for _, call := range message.ToolCalls {
			if call.ID == id {
				return true
			}
		}
	}
	return false
}

func containsContextToolPair(messages []llm.Message, id string) bool {
	return containsContextCall(messages, id) && func() bool {
		for _, message := range messages {
			if message.Role == "tool" && message.ToolCallID == id {
				return true
			}
		}
		return false
	}()
}

func assertContextToolPairsComplete(t *testing.T, messages []llm.Message) {
	t.Helper()
	for index, message := range messages {
		if message.Role != "tool" {
			continue
		}
		found := false
		for prior := index - 1; prior >= 0; prior-- {
			if messages[prior].Role != "assistant" {
				continue
			}
			for _, call := range messages[prior].ToolCalls {
				if call.ID == message.ToolCallID {
					found = true
				}
			}
			if found {
				break
			}
		}
		if !found {
			t.Fatalf("orphan tool result at %d: %#v", index, messages)
		}
	}
}
