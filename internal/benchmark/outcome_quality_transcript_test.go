package benchmark

import (
	"context"
	"testing"

	"github.com/saiaathish/picogent/internal/llm"
)

func TestOutcomeQualityTranscriptTelemetryCountsRepairAndGrowth(t *testing.T) {
	telemetry := &outcomeQualityTranscriptTelemetry{}
	initial := []outcomeQualityTranscriptMessage{{Role: "user", Content: "fix the fixture"}}
	repairRequest := []outcomeQualityTranscriptMessage{
		{Role: "user", Content: "fix the fixture"},
		{Role: "tool", Name: "verify", Content: "verify FAIL\nfixture content mismatch"},
	}
	telemetry.ObserveRequest("fixture-model", initial)
	telemetry.ObserveRequest("fixture-model", repairRequest)
	telemetry.ObserveResponse(repairRequest, []outcomeQualityTranscriptToolCall{{ID: "write-1", Name: "write_file", Arguments: `{"path":"fixture.txt"}`}})

	repairs, growth, reasons := telemetry.Metrics()
	if len(reasons) != 0 {
		t.Fatalf("unexpected transcript reasons: %v", reasons)
	}
	if repairs != 1 {
		t.Fatalf("repair count=%d, want 1", repairs)
	}
	if growth <= 0 {
		t.Fatalf("context growth=%d, want positive growth", growth)
	}
}

func TestOutcomeQualityTranscriptTelemetryDoesNotInventRepairOrGrowth(t *testing.T) {
	telemetry := &outcomeQualityTranscriptTelemetry{}
	request := []outcomeQualityTranscriptMessage{{Role: "user", Content: "fix the fixture"}}
	telemetry.ObserveRequest("fixture-model", request)
	telemetry.ObserveRequest("fixture-model", request)
	telemetry.ObserveResponse(request, []outcomeQualityTranscriptToolCall{{ID: "write-1", Name: "write_file"}})

	repairs, growth, reasons := telemetry.Metrics()
	if len(reasons) != 0 {
		t.Fatalf("unexpected transcript reasons: %v", reasons)
	}
	if repairs != 0 || growth != 0 {
		t.Fatalf("metrics=(repairs=%d growth=%d), want exact zeroes", repairs, growth)
	}
}

func TestOutcomeQualityCountingClientMeasuresRepairTranscript(t *testing.T) {
	client := &outcomeQualityCountingClient{scripted: &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", Content: "checking"}, PromptTokens: 1, CompletionTokens: 1},
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "write-1", Name: "write_file", Arguments: `{"path":"fixture.txt"}`}}}, PromptTokens: 1, CompletionTokens: 1},
	}}}
	if _, err := client.Chat(context.Background(), llm.ChatRequest{
		Model:    "fixture-model",
		Messages: []llm.Message{{Role: "user", Content: "fix the fixture"}},
	}); err != nil {
		t.Fatalf("first chat: %v", err)
	}
	if _, err := client.Chat(context.Background(), llm.ChatRequest{
		Model: "fixture-model",
		Messages: []llm.Message{
			{Role: "user", Content: "fix the fixture"},
			{Role: "tool", Name: "verify", Content: "verify FAIL\nfixture content mismatch"},
		},
	}); err != nil {
		t.Fatalf("repair chat: %v", err)
	}

	repairs, growth, reasons := outcomeQualityClientTranscriptMetrics(client)
	if repairs != 1 || growth <= 0 || len(reasons) != 0 {
		t.Fatalf("client transcript=(repairs=%d growth=%d reasons=%v), want one repair, positive growth, and no reasons", repairs, growth, reasons)
	}
}

func TestOutcomeQualityTranscriptTelemetryFailsClosedForUnsupportedLegacyContent(t *testing.T) {
	_, err := outcomeQualityTranscriptMessagesFromLegacy([]outcomeQualityLegacyProviderMessage{{Role: "user", Content: 17}})
	if err == nil {
		t.Fatal("unsupported legacy content unexpectedly normalized")
	}

	telemetry := &outcomeQualityTranscriptTelemetry{}
	telemetry.MarkUnavailable("legacy v3 request transcript is unsupported: " + err.Error())
	telemetry.ObserveRequest("fixture-model", []outcomeQualityTranscriptMessage{{Role: "user", Content: "fix"}})
	repairs, growth, reasons := telemetry.Metrics()
	if repairs != 0 || growth != 0 {
		t.Fatalf("unsupported transcript metrics=(repairs=%d growth=%d), want zeroes", repairs, growth)
	}
	if len(reasons) != 1 {
		t.Fatalf("reasons=%v, want one explicit unavailable reason", reasons)
	}
}
