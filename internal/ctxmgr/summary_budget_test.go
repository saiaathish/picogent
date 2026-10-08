package ctxmgr

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/llm"
)

type summaryCaptureClient struct {
	request llm.ChatRequest
}

func (c *summaryCaptureClient) Chat(_ context.Context, request llm.ChatRequest) (llm.ChatResponse, error) {
	c.request = request
	return llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: "bounded summary"}}, nil
}

type cancelAfterFirstErrContext struct {
	context.Context
	cancel context.CancelFunc
	calls  int
}

func (c *cancelAfterFirstErrContext) Err() error {
	c.calls++
	if c.calls == 1 {
		c.cancel()
		return nil
	}
	return c.Context.Err()
}

func TestSummarizeBoundsAggregateInput(t *testing.T) {
	const messages = 40
	conversation := make([]llm.Message, 0, messages)
	for i := 0; i < messages; i++ {
		conversation = append(conversation, llm.Message{
			Role:    "user",
			Content: fmt.Sprintf("message-%02d %s", i, strings.Repeat("context ", 300)),
		})
	}

	client := &summaryCaptureClient{}
	if _, err := Summarize(context.Background(), client, "gpt-5.6-terra", conversation); err != nil {
		t.Fatal(err)
	}
	if len(client.request.Messages) != 2 {
		t.Fatalf("summary request messages = %d, want system plus bounded body", len(client.request.Messages))
	}
	body := client.request.Messages[1].Content
	if len(body) > maxSummaryInputBytes {
		t.Fatalf("summary body length = %d, want <= %d", len(body), maxSummaryInputBytes)
	}
	if !strings.Contains(body, "message-00") || !strings.Contains(body, "message-39") {
		t.Fatalf("summary body lost prefix or suffix context: %q", body)
	}
	if !strings.Contains(body, summaryInputOmission) {
		t.Fatalf("bounded summary body missing omission marker: %q", body)
	}
}

func TestManageCanceledContextDoesNotStartCompactionRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := &summaryCaptureClient{}
	msgs := []llm.Message{{Role: "system", Content: "system"}, {Role: "user", Content: strings.Repeat("context ", 100)}}
	if _, _, err := Manage(ctx, client, "gpt-5.6-terra", msgs, DefaultBudget); err != context.Canceled {
		t.Fatalf("Manage error = %v, want context canceled", err)
	}
	if client.request.Model != "" {
		t.Fatal("Manage started a model-backed compaction request after cancellation")
	}
}

func TestManageCancellationDuringCompactionDoesNotStartSummaryRequest(t *testing.T) {
	base, cancel := context.WithCancel(context.Background())
	ctx := &cancelAfterFirstErrContext{Context: base, cancel: cancel}
	client := &summaryCaptureClient{}
	msgs := make([]llm.Message, 20)
	for i := range msgs {
		msgs[i] = llm.Message{Role: "user", Content: strings.Repeat("x", 2800)}
	}
	if _, _, err := Manage(ctx, client, "gpt-5.6-terra", msgs, 10_000); err != context.Canceled {
		t.Fatalf("Manage error = %v, want context canceled", err)
	}
	if client.request.Model != "" {
		t.Fatal("Manage started a summary request after cancellation during compaction")
	}
}

func TestSummarizeCancellationBeforeProviderCall(t *testing.T) {
	base, cancel := context.WithCancel(context.Background())
	ctx := &cancelAfterFirstErrContext{Context: base, cancel: cancel}
	client := &summaryCaptureClient{}
	msgs := []llm.Message{{Role: "user", Content: strings.Repeat("context ", 100)}}
	if _, err := Summarize(ctx, client, "gpt-5.6-terra", msgs); err != context.Canceled {
		t.Fatalf("Summarize error = %v, want context canceled", err)
	}
	if client.request.Model != "" {
		t.Fatal("Summarize started a provider request after cancellation")
	}
}
