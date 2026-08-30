package gui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvolveAPIErrorResponsesSanitizeUntrustedText(t *testing.T) {
	const secret = "evolve-api-secret"

	t.Run("load state", func(t *testing.T) {
		home := filepath.Join(t.TempDir(), "access_token="+secret)
		if err := os.WriteFile(home, []byte("not a directory"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PICOGENT_HOME", home)

		res := httptest.NewRecorder()
		(&server{}).evolveAPI(res, httptest.NewRequest(http.MethodGet, "/api/evolve", nil))
		assertSanitizedEvolveAPIError(t, res, http.StatusInternalServerError, secret)
	})

	t.Run("update state", func(t *testing.T) {
		home := filepath.Join(t.TempDir(), "access_token="+secret)
		if err := os.WriteFile(home, []byte("not a directory"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PICOGENT_HOME", home)

		res := httptest.NewRecorder()
		body := strings.NewReader(`{"kind":"habit","id":"missing"}`)
		(&server{}).evolveAPI(res, httptest.NewRequest(http.MethodDelete, "/api/evolve", body))
		assertSanitizedEvolveAPIError(t, res, http.StatusInternalServerError, secret)
	})
}

func assertSanitizedEvolveAPIError(t *testing.T, res *httptest.ResponseRecorder, wantStatus int, secret string) {
	t.Helper()
	if res.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body=%q", res.Code, wantStatus, res.Body.String())
	}
	body := res.Body.String()
	if strings.Contains(body, secret) {
		t.Fatalf("evolve API error leaked secret: %q", body)
	}
	if strings.Contains(body, "\x1b") || strings.Contains(body, "\naccess_token") {
		t.Fatalf("evolve API error retained terminal/newline injection: %q", body)
	}
	if !strings.Contains(body, "[REDACTED]") {
		t.Fatalf("evolve API error did not record a redaction marker: %q", body)
	}
}
