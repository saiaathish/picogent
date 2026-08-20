package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeTok struct{ access, account string }

func (f fakeTok) Token(context.Context) (string, string, error) { return f.access, f.account, nil }
func (f fakeTok) ForceRefresh(context.Context) error            { return nil }

func TestToResponsesInput(t *testing.T) {
	inst, input := toResponsesInput([]Message{
		{Role: "system", Content: "be small"},
		{Role: "user", Content: "read it"},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "call_1", ItemID: "fc_1", Name: "read_file", Arguments: `{"path":"a"}`}}},
		{Role: "tool", ToolCallID: "call_1", Content: "hi"},
	})
	if inst != "be small" {
		t.Fatalf("instructions %q", inst)
	}
	if len(input) != 3 {
		t.Fatalf("len=%d", len(input))
	}
	fc := input[1].(map[string]any)
	if fc["type"] != "function_call" || fc["call_id"] != "call_1" {
		t.Fatalf("%v", fc)
	}
	out := input[2].(map[string]any)
	if out["type"] != "function_call_output" || out["call_id"] != "call_1" {
		t.Fatalf("%v", out)
	}
}

func TestCodexStreamParsesTextAndTools(t *testing.T) {
	stream := strings.Join([]string{
		"event: response.output_text.delta",
		`data: {"type":"response.output_text.delta","delta":"hi"}`,
		"",
		"event: response.output_item.done",
		`data: {"type":"response.output_item.done","item":{"id":"fc_1","type":"function_call","call_id":"call_9","name":"read_file","arguments":"{\"path\":\"README.md\"}"}}`,
		"",
		"event: response.completed",
		`data: {"type":"response.completed"}`,
		"",
	}, "\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("path %s", r.URL.Path)
		}
		if r.Header.Get("originator") == "" {
			t.Fatal("missing originator")
		}
		if r.Header.Get("chatgpt-account-id") != "acct" {
			t.Fatal("missing account")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(stream))
	}))
	defer srv.Close()
	c := &Codex{
		Model:      "gpt-5.6-luna",
		BaseURL:    srv.URL,
		Originator: "codex_cli_rs",
		Tokens:     fakeTok{access: "tok", account: "acct"},
		HTTP:       srv.Client(),
	}
	out, err := c.Chat(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
		Tools:    []ToolSpec{{Name: "read_file", Description: "r", Parameters: map[string]any{"type": "object"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Message.Content != "hi" {
		t.Fatalf("content %q", out.Message.Content)
	}
	if len(out.Message.ToolCalls) != 1 || out.Message.ToolCalls[0].ID != "call_9" {
		t.Fatalf("%+v", out.Message.ToolCalls)
	}
}

func TestCodexRequiresStreamTrue(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Stream bool `json:"stream"`
			Store  bool `json:"store"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if !body.Stream || body.Store {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"detail":"Stream must be set to true"}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"pong\"}\n\n"))
	}))
	defer srv.Close()
	c := &Codex{BaseURL: srv.URL, Tokens: fakeTok{access: "t"}, HTTP: srv.Client(), Originator: "x"}
	out, err := c.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "ping"}}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Message.Content != "pong" {
		t.Fatalf("%q", out.Message.Content)
	}
}
