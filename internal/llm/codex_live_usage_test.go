package llm

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/codexauth"
)

// This probe consumes subscription allowance, never an API key. Normal test
// runs and CI do not contact the signed-in provider without explicit opt-in.
func TestCodexLiveUsageExplicitOptIn(t *testing.T) {
	if os.Getenv("PICOGENT_CODEX_USAGE_LIVE") != "1" {
		t.Skip("set PICOGENT_CODEX_USAGE_LIVE=1 to probe the existing Codex login")
	}
	if !codexauth.LoggedIn() {
		t.Fatal("live probe requires an existing Codex OAuth login")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client := NewCodex("")
	out, err := client.Chat(ctx, ChatRequest{
		Reasoning: ReasonLow,
		Messages: []Message{
			{Role: "system", Content: "Reply with exactly OK. Do not explain or use tools."},
			{Role: "user", Content: "Reply OK."},
		},
	})
	if err != nil {
		t.Fatalf("signed-in Codex request failed: %v", err)
	}
	if strings.TrimSpace(out.Message.Content) != "OK" || len(out.Message.ToolCalls) != 0 {
		t.Fatal("signed-in Codex did not return the expected short text response")
	}
	if !out.UsageReported || out.PromptTokens <= 0 || out.CompletionTokens <= 0 {
		t.Fatalf("usage unavailable or unexpected: reported=%v input=%d output=%d", out.UsageReported, out.PromptTokens, out.CompletionTokens)
	}
	t.Logf("signed-in Codex: model=%s usage_reported=%v input_tokens=%d output_tokens=%d", client.Model, out.UsageReported, out.PromptTokens, out.CompletionTokens)
}
