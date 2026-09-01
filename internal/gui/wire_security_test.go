package gui

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
)

func assertSafeGUIWire(t *testing.T, wire, secret string) {
	t.Helper()
	if strings.Contains(wire, secret) {
		t.Fatalf("GUI wire leaked credential-shaped value %q: %s", secret, wire)
	}
	if strings.ContainsRune(wire, '\x1b') {
		t.Fatalf("GUI wire retained a terminal control: %q", wire)
	}
}

func TestGUIStateHandlerSerializesSafeBoundedTranscriptAndPermission(t *testing.T) {
	const secret = "gui-state-wire-secret"
	s := newLoopbackAPITestServer(t)
	s.hist = []llm.Message{
		{Role: "user", Content: "repository review notes token=" + secret + "\x1b[31m ordinary user text"},
		{Role: "assistant", Content: "provider response api_key=" + secret + "\nordinary assistant text"},
		{Role: "tool", Content: strings.Repeat("ordinary provider output ", 40) + "access_token=" + secret + "\x1b[31m"},
	}
	s.pendingPerm = perm.Request{
		Tool:    "mcp_github_create_issue",
		Summary: "create repository issue password=" + secret,
		Hint:    "\x1b[31mreview authorization=" + secret,
	}

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	res, err := ts.Client().Get(ts.URL + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("state status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if got := res.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("state content type = %q, want application/json", got)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	assertSafeGUIWire(t, string(body), secret)

	var payload struct {
		Messages    []transcriptLine  `json:"messages"`
		PendingPerm map[string]string `json:"pending_perm"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode state response: %v", err)
	}
	if len(payload.Messages) != 3 {
		t.Fatalf("state transcript lines = %#v, want three roles", payload.Messages)
	}
	if !strings.Contains(payload.Messages[0].Text, "ordinary user text") ||
		!strings.Contains(payload.Messages[1].Text, "ordinary assistant text") ||
		!strings.Contains(payload.Messages[2].Text, "ordinary provider output") {
		t.Fatalf("state lost ordinary readable text: %#v", payload.Messages)
	}
	if got := len(payload.Messages[2].Text); got > maxGUIToolTranscriptBytes+len("…") {
		t.Fatalf("tool transcript length = %d, want at most %d", got, maxGUIToolTranscriptBytes+len("…"))
	}
	if payload.PendingPerm["tool"] != "mcp_github_create_issue" ||
		!strings.Contains(payload.PendingPerm["summary"], "[REDACTED]") ||
		!strings.Contains(payload.PendingPerm["hint"], "[REDACTED]") {
		t.Fatalf("pending permission projection = %#v", payload.PendingPerm)
	}
}

func readGUIEventData(t *testing.T, reader *bufio.Reader) []byte {
	t.Helper()
	var data []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE stream: %v", err)
		}
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			if len(data) == 0 {
				continue
			}
			return []byte(strings.Join(data, "\n"))
		}
		if value, ok := strings.CutPrefix(line, "data:"); ok {
			data = append(data, strings.TrimSpace(value))
		}
	}
}

func TestGUIEventsHandlerSerializesSafeReconnectAndLiveWire(t *testing.T) {
	const secret = "gui-events-wire-secret"
	s := newLoopbackAPITestServer(t)
	s.pendingPerm = perm.Request{
		Tool:    "mcp_github_create_issue",
		Summary: "create repository issue password=" + secret,
		Hint:    "\x1b[31mreview authorization=" + secret,
	}

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	client := ts.Client()
	client.Timeout = 5 * time.Second
	res, err := client.Get(ts.URL + "/api/events")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("events status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if got := res.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("events content type = %q, want text/event-stream", got)
	}
	reader := bufio.NewReader(res.Body)

	helloWire := readGUIEventData(t, reader)
	assertSafeGUIWire(t, string(helloWire), secret)
	var hello event
	if err := json.Unmarshal(helloWire, &hello); err != nil {
		t.Fatalf("decode hello event: %v", err)
	}
	if hello.Type != "hello" {
		t.Fatalf("hello event = %#v, want hello", hello)
	}

	reconnectWire := readGUIEventData(t, reader)
	assertSafeGUIWire(t, string(reconnectWire), secret)
	var reconnect event
	if err := json.Unmarshal(reconnectWire, &reconnect); err != nil {
		t.Fatalf("decode reconnect permission: %v", err)
	}
	if reconnect.Type != "permission" || reconnect.Text != "mcp_github_create_issue" ||
		!strings.Contains(reconnect.Summary, "[REDACTED]") ||
		!strings.Contains(reconnect.Hint, "[REDACTED]") {
		t.Fatalf("reconnect permission event = %#v", reconnect)
	}

	s.emit(event{Type: "tool", Text: strings.Repeat("ordinary tool output ", 40) + "access_token=" + secret + "\x1b[31m"})
	toolWire := readGUIEventData(t, reader)
	assertSafeGUIWire(t, string(toolWire), secret)
	var tool event
	if err := json.Unmarshal(toolWire, &tool); err != nil {
		t.Fatalf("decode tool event: %v", err)
	}
	if tool.Type != "tool" || !strings.Contains(tool.Text, "ordinary tool output") {
		t.Fatalf("tool event = %#v", tool)
	}
	if got := len(tool.Text); got > maxGUIToolTranscriptBytes+len("…") {
		t.Fatalf("tool event text length = %d, want at most %d", got, maxGUIToolTranscriptBytes+len("…"))
	}

	s.emit(event{
		Type:    "change",
		Text:    "ordinary changed file",
		Summary: "ordinary change token=" + secret,
		Path:    "repo/README.md\n\x1b[31maccess_token=" + secret,
	})
	pathWire := readGUIEventData(t, reader)
	assertSafeGUIWire(t, string(pathWire), secret)
	var pathEvent event
	if err := json.Unmarshal(pathWire, &pathEvent); err != nil {
		t.Fatalf("decode path event: %v", err)
	}
	if pathEvent.Type != "change" || pathEvent.Text != "ordinary changed file" ||
		!strings.HasPrefix(pathEvent.Path, "repo/README.md") ||
		!strings.Contains(pathEvent.Path, "[REDACTED]") {
		t.Fatalf("path event = %#v", pathEvent)
	}
}
