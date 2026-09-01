package gui

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
