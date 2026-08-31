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
