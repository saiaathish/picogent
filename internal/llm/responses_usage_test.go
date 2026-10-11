package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func responsesUsageTestClient(t *testing.T, stream string) *Codex {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(stream))
	}))
	t.Cleanup(server.Close)
	return &Codex{Model: "fixture-model", BaseURL: server.URL, Originator: "fixture", Tokens: fakeTok{access: "fixture"}, HTTP: server.Client()}
}

func responsesUsageTestEvent(payload string) string {
	return "data: " + payload + "\n\n"
}

func TestCodexRetainsTerminalUsage(t *testing.T) {
	stream := responsesUsageTestEvent(`{"type":"response.output_text.delta","delta":"done"}`) +
		responsesUsageTestEvent(`{"type":"response.completed","response":{"usage":{"input_tokens":12,"output_tokens":5,"total_tokens":17}}}`)
	out, err := responsesUsageTestClient(t, stream).Chat(context.Background(), ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Message.Content != "done" || out.PromptTokens != 12 || out.CompletionTokens != 5 || !out.UsageReported {
		t.Fatalf("response=%+v, want output and reported token counts", out)
	}
}

func TestCodexFailedStreamPreservesPartialUsage(t *testing.T) {
	stream := responsesUsageTestEvent(`{"type":"response.output_text.delta","delta":"partial"}`) +
		responsesUsageTestEvent(`{"type":"response.failed","response":{"error":{"message":"provider failed"},"usage":{"input_tokens":12,"output_tokens":5}}}`)
	out, err := responsesUsageTestClient(t, stream).Chat(context.Background(), ChatRequest{})
	if err == nil || !strings.Contains(err.Error(), "provider failed") {
		t.Fatalf("error=%v, want provider failure", err)
	}
	if out.Message.Content != "partial" || out.PromptTokens != 12 || out.CompletionTokens != 5 || !out.UsageReported {
		t.Fatalf("response=%+v, want partial output and reported token counts", out)
	}
}

func TestCodexIncompleteStreamPreservesPartialUsage(t *testing.T) {
	stream := responsesUsageTestEvent(`{"type":"response.output_text.delta","delta":"partial"}`) +
		responsesUsageTestEvent(`{"type":"response.incomplete","response":{"incomplete_details":{"reason":"max_output_tokens"},"usage":{"input_tokens":12,"output_tokens":5}}}`)
	out, err := responsesUsageTestClient(t, stream).Chat(context.Background(), ChatRequest{})
	if err == nil || !strings.Contains(err.Error(), "max_output_tokens") {
		t.Fatalf("error=%v, want incomplete response reason", err)
	}
	if out.Message.Content != "partial" || out.PromptTokens != 12 || out.CompletionTokens != 5 || !out.UsageReported {
		t.Fatalf("response=%+v, want partial output and reported token counts", out)
	}
}

func TestResponsesStreamUsageAvailability(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	for _, tc := range []struct {
		name, usage   string
		input, output int
		reported      bool
	}{
		{"absent", "", 0, 0, false},
		{"null", `,"usage":null`, 0, 0, false},
		{"empty", `,"usage":{}`, 0, 0, false},
		{"zero", `,"usage":{"input_tokens":0,"output_tokens":0}`, 0, 0, true},
		{"input only", `,"usage":{"input_tokens":12}`, 12, 0, false},
		{"output only", `,"usage":{"output_tokens":5}`, 0, 5, false},
		{"null counter", `,"usage":{"input_tokens":null,"output_tokens":5}`, 0, 5, false},
		{"negative", `,"usage":{"input_tokens":-1,"output_tokens":5}`, 0, 5, false},
		{"fraction", `,"usage":{"input_tokens":1.5,"output_tokens":5}`, 0, 5, false},
		{"string", `,"usage":{"input_tokens":"12","output_tokens":5}`, 0, 5, false},
		{"boolean", `,"usage":{"input_tokens":true,"output_tokens":5}`, 0, 5, false},
		{"counter overflow", `,"usage":{"input_tokens":9223372036854775808,"output_tokens":5}`, 0, 5, false},
		{"sum overflow", fmt.Sprintf(`,"usage":{"input_tokens":%d,"output_tokens":1}`, maxInt), 0, 0, false},
		{"maximum", fmt.Sprintf(`,"usage":{"input_tokens":%d,"output_tokens":0}`, maxInt), maxInt, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream := responsesUsageTestEvent(`{"type":"response.completed","response":{"status":"completed"` + tc.usage + `}}`)
			out, err := readResponsesStream(strings.NewReader(stream), nil)
			if err != nil {
				t.Fatal(err)
			}
			if out.PromptTokens != tc.input || out.CompletionTokens != tc.output || out.UsageReported != tc.reported {
				t.Fatalf("response=%+v, want usage %d/%d reported=%v", out, tc.input, tc.output, tc.reported)
			}
		})
	}
}

func TestResponsesStreamUsageSnapshotsNotAdded(t *testing.T) {
	stream := responsesUsageTestEvent(`{"type":"response.completed","response":{"usage":{"input_tokens":12,"output_tokens":5}}}`) +
		responsesUsageTestEvent(`{"type":"response.completed","response":{"usage":{"input_tokens":12,"output_tokens":6}}}`)
	out, err := readResponsesStream(strings.NewReader(stream), nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.PromptTokens != 12 || out.CompletionTokens != 6 || !out.UsageReported {
		t.Fatalf("response=%+v, want last snapshot, not sum", out)
	}
}

func TestResponsesStreamRejectsBadEventsWithPartialOutput(t *testing.T) {
	for _, payload := range []string{"{", "null", "[]", `{} {}`, `{"type":"response.completed"} junk`} {
		t.Run(payload, func(t *testing.T) {
			stream := responsesUsageTestEvent(`{"type":"response.output_text.delta","delta":"partial"}`) + responsesUsageTestEvent(payload)
			out, err := readResponsesStream(strings.NewReader(stream), nil)
			if err == nil || out.Message.Content != "partial" {
				t.Fatalf("response=%+v error=%v, want preserved output and decode error", out, err)
			}
		})
	}
}

type responsesErrorReader struct{ err error }

func (r responsesErrorReader) Read([]byte) (int, error) { return 0, r.err }

func TestResponsesStreamReaderFailurePreservesEvidence(t *testing.T) {
	wantErr := errors.New("fixture transport failure")
	stream := responsesUsageTestEvent(`{"type":"response.output_text.delta","delta":"partial"}`) +
		responsesUsageTestEvent(`{"type":"response.completed","response":{"usage":{"input_tokens":12,"output_tokens":5}}}`)
	out, err := readResponsesStream(io.MultiReader(strings.NewReader(stream), responsesErrorReader{wantErr}), nil)
	if !errors.Is(err, wantErr) || out.Message.Content != "partial" || out.PromptTokens != 12 || out.CompletionTokens != 5 || !out.UsageReported {
		t.Fatalf("response=%+v error=%v, want retained transport failure and evidence", out, err)
	}
}

func TestResponsesStreamFramingAndErrors(t *testing.T) {
	for _, tc := range []struct{ name, stream, content, errorText string }{
		{"event fallback and CRLF", "event: response.output_text.delta\r\ndata: {\"delta\":\"ok\"}\r\n\r\nevent: response.completed\r\ndata: {}", "ok", ""},
		{"multiline", "data: {\"type\":\"response.completed\",\n" + "data: \"response\":{\"usage\":{\"input_tokens\":0,\"output_tokens\":0}}}\n\n", "", ""},
		{"DONE is not completion", "data: [DONE]\n\n", "", "closed before completion"},
		{"empty", "", "", "closed before completion"},
		{"top level error", responsesUsageTestEvent(`{"type":"error","message":"bad request"}`), "", "bad request"},
		{"nested error", responsesUsageTestEvent(`{"type":"response.error","error":{"message":"nested failure"}}`), "", "nested failure"},
		{"detail error", responsesUsageTestEvent(`{"type":"error","detail":"detail failure"}`), "", "detail failure"},
		{"default error", responsesUsageTestEvent(`{"type":"response.failed"}`), "", "responses stream error"},
		{"default incomplete", responsesUsageTestEvent(`{"type":"response.incomplete"}`), "", "unspecified reason"},
		{"scanner limit", "data: " + strings.Repeat("x", 4<<20) + "\n\n", "", "token too long"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := readResponsesStream(strings.NewReader(tc.stream), nil)
			if out.Message.Content != tc.content {
				t.Fatalf("content=%q, want %q", out.Message.Content, tc.content)
			}
			if tc.errorText == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.errorText) {
				t.Fatalf("error=%v, want %q", err, tc.errorText)
			}
		})
	}
}

type responsesCountingTokens struct{ refreshes int }

func (t *responsesCountingTokens) Token(context.Context) (string, string, error) {
	return "fixture", "", nil
}
func (t *responsesCountingTokens) ForceRefresh(context.Context) error { t.refreshes++; return nil }

func TestCodexStreamAuthenticationErrorsNeverReplay(t *testing.T) {
	for _, prefix := range []string{
		"",
		responsesUsageTestEvent(`{"type":"response.output_text.delta","delta":"partial"}`),
		responsesUsageTestEvent(`{"type":"response.output_item.done","item":{"type":"function_call","call_id":"c1","name":"read_file","arguments":"{}"}}`),
	} {
		t.Run(fmt.Sprintf("prefix bytes %d", len(prefix)), func(t *testing.T) {
			client := responsesUsageTestClient(t, prefix+responsesUsageTestEvent(`{"type":"response.failed","response":{"error":{"message":"unauthorized http 401"},"usage":{"input_tokens":12,"output_tokens":5}}}`))
			tokens := &responsesCountingTokens{}
			client.Tokens = tokens
			out, err := client.Chat(context.Background(), ChatRequest{})
			if err == nil || tokens.refreshes != 0 || out.PromptTokens != 12 || out.CompletionTokens != 5 || !out.UsageReported {
				t.Fatalf("response=%+v error=%v refreshes=%d, want no replay", out, err, tokens.refreshes)
			}
			if strings.Contains(prefix, "partial") && out.Message.Content != "partial" {
				t.Fatalf("partial text discarded: %+v", out.Message)
			}
			if strings.Contains(prefix, "function_call") && (len(out.Message.ToolCalls) != 1 || out.Message.ToolCalls[0].ID != "c1") {
				t.Fatalf("partial tool call discarded: %+v", out.Message)
			}
		})
	}
	client := responsesUsageTestClient(t, responsesUsageTestEvent(`{"type":"error","message":"unauthorized"}`))
	tokens := &responsesCountingTokens{}
	client.Tokens = tokens
	if _, err := client.Chat(context.Background(), ChatRequest{}); err == nil || tokens.refreshes != 0 {
		t.Fatalf("error=%v refreshes=%d, even an empty stream error must not replay", err, tokens.refreshes)
	}
}

func TestCodexHTTP401RefreshesOnce(t *testing.T) {
	for _, alwaysUnauthorized := range []bool{false, true} {
		t.Run(fmt.Sprintf("always unauthorized %v", alwaysUnauthorized), func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if requests == 1 || alwaysUnauthorized {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				_, _ = io.WriteString(w, responsesUsageTestEvent(`{"type":"response.completed","response":{"usage":{"input_tokens":12,"output_tokens":5}}}`))
			}))
			t.Cleanup(server.Close)
			tokens := &responsesCountingTokens{}
			client := &Codex{BaseURL: server.URL, Tokens: tokens, HTTP: server.Client()}
			out, err := client.Chat(context.Background(), ChatRequest{})
			if (err != nil) != alwaysUnauthorized || tokens.refreshes != 1 || requests != 2 {
				t.Fatalf("error=%v refreshes=%d requests=%d", err, tokens.refreshes, requests)
			}
			if !alwaysUnauthorized && (!out.UsageReported || out.PromptTokens != 12 || out.CompletionTokens != 5) {
				t.Fatalf("response=%+v, want usage after successful retry", out)
			}
		})
	}
}

func TestOpenAIResponsesRetainsUsageAndPartialErrors(t *testing.T) {
	for _, terminal := range []string{"response.completed", "response.failed", "response.incomplete"} {
		t.Run(terminal, func(t *testing.T) {
			stream := responsesUsageTestEvent(`{"type":"response.output_text.delta","delta":"output"}`) +
				responsesUsageTestEvent(fmt.Sprintf(`{"type":%q,"response":{"usage":{"input_tokens":12,"output_tokens":5}}}`, terminal))
			client := responsesUsageTestClient(t, stream)
			out, err := openAIResponsesChat(context.Background(), client.HTTP, client.BaseURL+"/responses", "fixture", ChatRequest{})
			if (err != nil) != (terminal != "response.completed") || out.Message.Content != "output" || out.PromptTokens != 12 || out.CompletionTokens != 5 || !out.UsageReported {
				t.Fatalf("response=%+v error=%v, want shared parser evidence", out, err)
			}
		})
	}
}

func TestCodexDisconnectedStreamPreservesPartialResponse(t *testing.T) {
	stream := responsesUsageTestEvent(`{"type":"response.output_text.delta","delta":"partial"}`)
	out, err := responsesUsageTestClient(t, stream).Chat(context.Background(), ChatRequest{})
	if err == nil || !strings.Contains(err.Error(), "closed before completion") {
		t.Fatalf("error=%v, want disconnected stream rejection", err)
	}
	if out.Message.Content != "partial" {
		t.Fatalf("response=%+v, want partial output", out)
	}
}
