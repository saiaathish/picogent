package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/llm"
)

func TestOpenAIChatParsesToolCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{
				map[string]any{
					"message": map[string]any{
						"role": "assistant",
						"tool_calls": []any{
							map[string]any{
								"id":   "call_1",
								"type": "function",
								"function": map[string]any{
									"name":      "read_file",
									"arguments": `{"path":"README.md"}`,
								},
							},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()
	c := llm.NewOpenAI(srv.URL, "sk-test", "demo", 5*time.Second)
	out, err := c.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Message.ToolCalls) != 1 || out.Message.ToolCalls[0].Name != "read_file" {
		t.Fatalf("%+v", out.Message)
	}
}
