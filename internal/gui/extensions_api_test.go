package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtensionsAPIErrorSanitizesClaudePluginIdentifier(t *testing.T) {
	const secret = "extensions-api-secret"
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "claude-marketplace.json"), []byte(`{"plugins":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s := newLoopbackAPITestServerAtHome(t, home)
	body, err := json.Marshal(map[string]any{
		"action":  "install",
		"id":      "claude:../access_token=" + secret + "\n\x1b[31m",
		"approve": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := loopbackAPIRequest(http.MethodPost, "/api/extensions", string(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+loopbackTestHost)
	res := httptest.NewRecorder()
	s.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body=%q", res.Code, http.StatusInternalServerError, res.Body.String())
	}
	response := res.Body.String()
	if strings.Contains(response, secret) {
		t.Fatalf("extension API leaked secret: %q", response)
	}
	if strings.Contains(response, "\x1b") || strings.Contains(response, "\naccess_token") {
		t.Fatalf("extension API retained terminal/newline injection: %q", response)
	}
	if !strings.Contains(response, "[REDACTED]") {
		t.Fatalf("extension API error did not record a redaction marker: %q", response)
	}
}
