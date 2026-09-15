package benchmark

import (
	"fmt"
	"strings"
	"sync"

	"github.com/saiaathish/picogent/internal/llm"
)

// outcomeQualityTranscriptToolCall is the provider-neutral subset of one
// model tool call that contributes to retained request context.
type outcomeQualityTranscriptToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// outcomeQualityTranscriptMessage is the provider-neutral subset of one
// model request message. It intentionally excludes provider-specific wire
// framing and tool schemas so an in-process v4 request and a v3 request seen
// through the controlled OpenAI-compatible proxy use the same measurement.
type outcomeQualityTranscriptMessage struct {
	Role       string
	Content    string
	ToolCallID string
	Name       string
	ToolCalls  []outcomeQualityTranscriptToolCall
}

// outcomeQualityTranscriptTelemetry measures the shared benchmark context
// boundary. A repair action is one write/edit tool-response plan issued after
// the latest verification result in that request was FAIL. Context growth is
// the peak request-context size minus the first request-context size.
//
// All callers must mark a transcript shape they cannot normalize as
// unavailable. Metrics then return zero values plus named reasons rather than
// silently converting missing telemetry into a passing-looking zero.
type outcomeQualityTranscriptTelemetry struct {
	mu sync.Mutex

	initialSet          bool
	initialContextBytes int64
	peakContextBytes    int64
	repairActions       int
	unverified          []string
}

func (t *outcomeQualityTranscriptTelemetry) ObserveRequest(model string, messages []outcomeQualityTranscriptMessage) {
	if t == nil {
		return
	}
	bytes := outcomeQualityTranscriptRequestBytes(model, messages)
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.initialSet {
		t.initialSet = true
		t.initialContextBytes = bytes
		t.peakContextBytes = bytes
		return
	}
	if bytes > t.peakContextBytes {
		t.peakContextBytes = bytes
	}
}

func (t *outcomeQualityTranscriptTelemetry) ObserveResponse(request []outcomeQualityTranscriptMessage, calls []outcomeQualityTranscriptToolCall) {
	if t == nil || !outcomeQualityLatestVerificationFailed(request) || !outcomeQualityHasRepairWrite(calls) {
		return
	}
	t.mu.Lock()
	t.repairActions++
	t.mu.Unlock()
}

func (t *outcomeQualityTranscriptTelemetry) MarkUnavailable(reason string) {
	if t == nil {
		return
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, existing := range t.unverified {
		if existing == reason {
			return
		}
	}
	if len(t.unverified) < 8 {
		t.unverified = append(t.unverified, reason)
	}
}

func (t *outcomeQualityTranscriptTelemetry) Metrics() (repairCount int, contextGrowthBytes int64, unverified []string) {
	if t == nil {
		return 0, 0, []string{"outcome-quality request transcript observer is unavailable"}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	reasons := append([]string(nil), t.unverified...)
	if !t.initialSet {
		reasons = appendOutcomeQualityReason(reasons, "outcome-quality request transcript recorded no model request", 8)
	}
	if len(reasons) > 0 {
		return 0, 0, reasons
	}
	if t.peakContextBytes > t.initialContextBytes {
		contextGrowthBytes = t.peakContextBytes - t.initialContextBytes
	}
	return t.repairActions, contextGrowthBytes, nil
}

func outcomeQualityTranscriptRequestBytes(model string, messages []outcomeQualityTranscriptMessage) int64 {
	total := int64(len(model))
	for _, message := range messages {
		total += int64(len(message.Role) + len(message.Content) + len(message.ToolCallID) + len(message.Name))
		for _, call := range message.ToolCalls {
			total += int64(len(call.ID) + len(call.Name) + len(call.Arguments))
		}
	}
	return total
}

func outcomeQualityLatestVerificationFailed(messages []outcomeQualityTranscriptMessage) bool {
	for index := len(messages) - 1; index >= 0; index-- {
		message := messages[index]
		if message.Role == "tool" && message.Name == "verify" {
			return outcomeQualityStatus(message.Content) == "FAIL"
		}
	}
	return false
}

func outcomeQualityHasRepairWrite(calls []outcomeQualityTranscriptToolCall) bool {
	for _, call := range calls {
		switch call.Name {
		case "write_file", "edit_file":
			return true
		}
	}
	return false
}

func outcomeQualityTranscriptMessagesFromLLM(messages []llm.Message) ([]outcomeQualityTranscriptMessage, error) {
	result := make([]outcomeQualityTranscriptMessage, 0, len(messages))
	for index, message := range messages {
		converted, err := outcomeQualityTranscriptMessageFromLLM(message)
		if err != nil {
			return nil, fmt.Errorf("message %d: %w", index, err)
		}
		result = append(result, converted)
	}
	return result, nil
}

func outcomeQualityTranscriptMessageFromLLM(message llm.Message) (outcomeQualityTranscriptMessage, error) {
	if len(message.Parts) > 0 {
		return outcomeQualityTranscriptMessage{}, fmt.Errorf("multipart content is unsupported")
	}
	converted := outcomeQualityTranscriptMessage{
		Role:       message.Role,
		Content:    message.Content,
		ToolCallID: message.ToolCallID,
		Name:       message.Name,
		ToolCalls:  make([]outcomeQualityTranscriptToolCall, 0, len(message.ToolCalls)),
	}
	for _, call := range message.ToolCalls {
		converted.ToolCalls = append(converted.ToolCalls, outcomeQualityTranscriptToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: call.Arguments,
		})
	}
	return converted, nil
}
