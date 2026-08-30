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

func TestProjectsAPIErrorResponsesSanitizeUntrustedText(t *testing.T) {
	const secret = "projects-api-secret"

	t.Run("registry load", func(t *testing.T) {
		home := t.TempDir()
		if err := os.WriteFile(filepath.Join(home, "projects.yaml"), []byte("projects: access_token="+secret+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PICOGENT_HOME", home)

		res := httptest.NewRecorder()
		(&server{}).projectsAPI(res, httptest.NewRequest(http.MethodGet, "/api/projects", nil))
		assertSanitizedProjectsAPIError(t, res, http.StatusInternalServerError, secret)
	})

	t.Run("invalid project path", func(t *testing.T) {
		t.Setenv("PICOGENT_HOME", t.TempDir())
		path := filepath.Join(t.TempDir(), "access_token="+secret)
		body, err := json.Marshal(map[string]string{
			"action": "add",
			"path":   path,
		})
		if err != nil {
			t.Fatal(err)
		}

		res := httptest.NewRecorder()
		(&server{}).projectsAPI(res, httptest.NewRequest(http.MethodPost, "/api/projects", strings.NewReader(string(body))))
		assertSanitizedProjectsAPIError(t, res, http.StatusBadRequest, secret)
		if !strings.Contains(res.Body.String(), "[REDACTED]") {
			t.Fatalf("project API error did not record a redaction marker: %q", res.Body.String())
		}
	})
}

func assertSanitizedProjectsAPIError(t *testing.T, res *httptest.ResponseRecorder, wantStatus int, secret string) {
	t.Helper()
	if res.Code != wantStatus {
		t.Fatalf("status = %d, want %d; body=%q", res.Code, wantStatus, res.Body.String())
	}
	body := res.Body.String()
	if strings.Contains(body, secret) {
		t.Fatalf("project API error leaked secret: %q", body)
	}
	if strings.Contains(body, "\x1b") || strings.Contains(body, "\naccess_token") {
		t.Fatalf("project API error retained terminal/newline injection: %q", body)
	}
}
