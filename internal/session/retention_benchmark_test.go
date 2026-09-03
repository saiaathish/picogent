package session

import (
	"fmt"
	"testing"

	"github.com/saiaathish/picogent/internal/llm"
)

// BenchmarkSessionRetention compares the M-lane selector with the exact
// recency-only control used before integration. The fixture is deterministic:
// an older complete tool exchange is followed by enough plain assistant noise
// to exceed the old whole-turn admission rule, then by a newest complete turn.
// target-coverage/op is a structural fixture outcome, not a model-quality
// claim.
func BenchmarkSessionRetention(b *testing.B) {
	base := Session{ID: "retention-benchmark", Title: "chat", Workspace: "workspace"}
	fixture := retentionComparisonFixture()
	inputUnits := len(messageUnits(fixture))

	for _, tc := range []struct {
		name string
		fn   func([]llm.Message, Session) []llm.Message
	}{
		{name: "value-aware", fn: retainMessagesByValue},
		{name: "recency-only", fn: retainMessagesByRecency},
	} {
		b.Run(tc.name, func(b *testing.B) {
			var retained []llm.Message
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				retained = tc.fn(fixture, base)
			}
			b.StopTimer()
			b.ReportMetric(float64(inputUnits), "input-units/op")
			b.ReportMetric(float64(len(messageUnits(retained))), "retained-units/op")
			coverage := 0
			if containsRetentionTarget(retained) {
				coverage = 1
			}
			b.ReportMetric(float64(coverage), "target-coverage/op")
		})
	}
}

func TestSessionRetentionControlComparison(t *testing.T) {
	base := Session{ID: "retention-comparison", Title: "chat", Workspace: "workspace"}
	fixture := retentionComparisonFixture()
	valueAware := retainMessagesByValue(fixture, base)
	recencyOnly := retainMessagesByRecency(fixture, base)

	valueCoverage := containsRetentionTarget(valueAware)
	recencyCoverage := containsRetentionTarget(recencyOnly)
	t.Logf("retention comparison: input-units=%d value-aware-retained-units=%d recency-only-retained-units=%d value-aware-target=%t recency-only-target=%t", len(messageUnits(fixture)), len(messageUnits(valueAware)), len(messageUnits(recencyOnly)), valueCoverage, recencyCoverage)
	if !valueCoverage {
		t.Fatal("value-aware selector did not retain the complete target exchange")
	}
	if recencyCoverage {
		t.Fatal("recency-only control unexpectedly retained the target exchange")
	}
	if len(valueAware) > MaxSessionMessages || !sessionFits(base, valueAware) {
		t.Fatalf("value-aware output exceeded persistence bounds: messages=%d", len(valueAware))
	}
	if len(recencyOnly) > MaxSessionMessages || !sessionFits(base, recencyOnly) {
		t.Fatalf("recency-only control exceeded persistence bounds: messages=%d", len(recencyOnly))
	}
}

func retentionComparisonFixture() []llm.Message {
	messages := []llm.Message{
		{Role: "user", Content: "older request with useful tool result"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "retention-target", Name: "inspect", Arguments: `{}`}}},
		{Role: "tool", ToolCallID: "retention-target", Name: "inspect", Content: "useful result"},
	}
	for i := 0; i < 160; i++ {
		messages = append(messages, llm.Message{Role: "assistant", Content: fmt.Sprintf("plain noise %03d", i)})
	}
	messages = append(messages,
		llm.Message{Role: "user", Content: "newest request"},
		llm.Message{Role: "assistant", Content: "newest response"},
	)
	normalized := make([]llm.Message, 0, len(messages))
	for _, message := range messages {
		normalized = append(normalized, boundMessage(message))
	}
	return normalized
}

func containsRetentionTarget(messages []llm.Message) bool {
	assistant, tool := false, false
	for _, message := range messages {
		for _, call := range message.ToolCalls {
			if call.ID == "retention-target" {
				assistant = true
			}
		}
		if message.ToolCallID == "retention-target" {
			tool = true
		}
	}
	return assistant && tool
}
