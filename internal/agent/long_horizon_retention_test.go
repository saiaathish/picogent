package agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/saiaathish/picogent/internal/ctxmgr"
	"github.com/saiaathish/picogent/internal/llm"
)

func TestLongHorizonValueAwareLiveContextRetention(t *testing.T) {
	fixture := newLongHorizonFixture(t)
	advanceLongHorizon(t, fixture, longHorizonTurns)

	managed, stats, err := ctxmgr.Manage(
		context.Background(),
		nil,
		"gpt-5.6-terra",
		append([]llm.Message(nil), fixture.messages...),
		ctxmgr.DefaultBudget,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !stats.Compacted {
		t.Fatalf("stats=%+v, want live context compaction", stats)
	}
	if len(managed) > 19 {
		t.Fatalf("managed messages=%d, want <= 19 including one tool-pair backfill", len(managed))
	}
	if !longHorizonContainsText(managed, fmt.Sprintf("continue turn %d", longHorizonTurns-1)) {
		t.Fatalf("latest request was dropped from managed context: %#v", managed)
	}
	const historicalTarget = "read-088"
	if !longHorizonContainsToolPair(managed, historicalTarget) {
		t.Fatalf("value-aware live compaction dropped historical target %q", historicalTarget)
	}
	if longHorizonContainsToolPair(ctxmgr.TruncateTail(fixture.messages, 18), historicalTarget) {
		t.Fatalf("recency-only control unexpectedly retained historical target %q", historicalTarget)
	}
	assertLongHorizonToolPairsComplete(t, managed)

	t.Logf("live retention: raw_messages=%d managed_messages=%d managed_tokens=%d target=%s target_retained=true recency_control_target=false", len(fixture.messages), len(managed), stats.Tokens, historicalTarget)
}

func longHorizonContainsText(messages []llm.Message, want string) bool {
	for _, message := range messages {
		if message.Content == want {
			return true
		}
	}
	return false
}

func longHorizonContainsToolPair(messages []llm.Message, id string) bool {
	assistant, tool := false, false
	for _, message := range messages {
		for _, call := range message.ToolCalls {
			if call.ID == id {
				assistant = true
			}
		}
		if message.Role == "tool" && message.ToolCallID == id {
			tool = true
		}
	}
	return assistant && tool
}

func assertLongHorizonToolPairsComplete(t *testing.T, messages []llm.Message) {
	t.Helper()
	for index, message := range messages {
		if message.Role != "tool" {
			continue
		}
		owner := -1
		for prior := index - 1; prior >= 0; prior-- {
			if messages[prior].Role != "assistant" {
				continue
			}
			for _, call := range messages[prior].ToolCalls {
				if call.ID == message.ToolCallID {
					owner = prior
					break
				}
			}
			if owner >= 0 {
				break
			}
		}
		if owner < 0 {
			t.Fatalf("managed context contains orphan tool result at %d: %#v", index, messages)
		}
	}
}
