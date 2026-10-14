package agyauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/usage"
)

func TestUsageReadsModelQuotasWithoutInferenceOrOnboarding(t *testing.T) {
	for _, projectJSON := range []string{`"quota-project"`, `{"id":"quota-project"}`} {
		t.Run(projectJSON, func(t *testing.T) {
			calls := []string{}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls = append(calls, r.URL.Path)
				if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-token" {
					t.Error("bad quota status auth/method")
				}
				switch r.URL.Path {
				case "/v1internal:loadCodeAssist":
					_, _ = w.Write([]byte(`{"cloudaicompanionProject":` + projectJSON + `,"currentTier":{"name":"Google AI Pro"}}`))
				case "/v1internal:fetchAvailableModels":
					var body map[string]string
					_ = json.NewDecoder(r.Body).Decode(&body)
					if body["project"] != "quota-project" {
						t.Errorf("wrong project: %+v", body)
					}
					_, _ = w.Write([]byte(`{"models":{"gemini-pro":{"displayName":"Gemini Pro","quotaInfo":{"remainingFraction":0.6,"resetTime":"2030-01-01T12:00:00Z"}},"claude-sonnet":{"displayName":"Claude Sonnet","quotaInfo":{"remainingFraction":0,"isExhausted":true}},"unknown":{"quotaInfo":{"resetTime":"invalid"}},"unreported":{}}}`))
				default:
					t.Errorf("unexpected inference/onboarding request: %s", r.URL.Path)
					w.WriteHeader(400)
				}
			}))
			defer srv.Close()
			v, err := fetchUsage(context.Background(), srv.Client(), srv.URL, "test-token", "")
			if err != nil {
				t.Fatal(err)
			}
			if len(calls) != 2 || len(v.Groups) != 3 || v.Plan != "Google AI Pro" {
				t.Fatalf("bad status: calls=%v value=%+v", calls, v)
			}
			for _, g := range v.Groups {
				q := g.Windows[0]
				if q.WindowSeconds != nil {
					t.Fatal("model quota duration fabricated")
				}
				switch g.ID {
				case "gemini-pro":
					if q.RemainingPercent == nil || *q.RemainingPercent != 60 || q.ResetsAt == nil {
						t.Fatalf("bad Gemini quota: %+v", q)
					}
				case "claude-sonnet":
					if *q.RemainingPercent != 0 || !*g.LimitReached {
						t.Fatal("zero quota not preserved")
					}
				case "unknown":
					if q.RemainingPercent != nil || q.ResetsAt != nil {
						t.Fatal("unknown quota fabricated")
					}
				}
			}
			raw, _ := json.Marshal(v)
			if strings.Contains(string(raw), "test-token") || strings.Contains(string(raw), "quota-project") {
				t.Fatal("account credentials/project exposed")
			}
		})
	}
}

func TestUsageExpiredLoginAndAPIKey(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("PICOGENT_GEMINI_API_KEY", "")
	dir := t.TempDir()
	t.Setenv("PICOGENT_AGY_HOME", dir)
	if err := os.WriteFile(filepath.Join(dir, "antigravity-oauth-token"), []byte(`{"token":{"access_token":"expired","expiry":"2000-01-01T00:00:00Z"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadUsage(context.Background()); !errors.Is(err, usage.ErrLogin) {
		t.Fatalf("expired login = %v", err)
	}
	t.Setenv("GEMINI_API_KEY", "test-key")
	if _, err := ReadUsage(context.Background()); !errors.Is(err, usage.ErrUnavailable) {
		t.Fatalf("API mode fetched quota: %v", err)
	}
}
