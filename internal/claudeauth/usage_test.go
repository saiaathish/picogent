package claudeauth

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

func TestUsageSubscriptionAndModelWindows(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer test-token" || r.Header.Get("anthropic-beta") != "oauth-2025-04-20" {
			t.Error("invalid quota request auth/method")
		}
		_, _ = w.Write([]byte(`{"five_hour":{"utilization":25,"resets_at":"2030-01-01T12:00:00Z"},"seven_day":{"utilization":60,"resets_at":"2030-01-07T12:00:00Z"},"seven_day_opus":{"utilization":100},"seven_day_sonnet":null,"seven_day_oauth_apps":4,"extra_usage":{"utilization":95},"limits":[{"percent":80,"scope":{"model":{"id":"claude-fable","display_name":"Fable"}},"resets_at":"invalid"},{"percent":20,"is_active":false,"scope":{"model":{"id":"disabled"}}}]}`))
	}))
	defer srv.Close()
	v, err := fetchUsage(context.Background(), srv.Client(), srv.URL, Cred{AccessToken: "test-token", SubscriptionType: "max"})
	if err != nil {
		t.Fatal(err)
	}
	if v.Plan != "max" || len(v.Groups) != 3 {
		t.Fatalf("unexpected windows: %+v", v)
	}
	main := v.Groups[0]
	if main.ID != "claude" || len(main.Windows) != 2 || *main.Windows[0].RemainingPercent != 75 || *main.Windows[1].RemainingPercent != 40 {
		t.Fatalf("bad main quotas: %+v", main)
	}
	if main.Windows[0].ResetsAt == nil || *main.Windows[0].WindowSeconds != 18000 {
		t.Fatal("session reset/duration missing")
	}
	if *v.Groups[1].Windows[0].RemainingPercent != 0 || v.Groups[1].Windows[0].ResetsAt != nil {
		t.Fatal("exhausted Opus quota or unknown reset changed")
	}
	if *v.Groups[2].Windows[0].RemainingPercent != 20 || v.Groups[2].Windows[0].WindowSeconds != nil {
		t.Fatal("model quota incorrectly assigned a window length")
	}
	raw, _ := json.Marshal(v)
	if strings.Contains(string(raw), "test-token") || strings.Contains(string(raw), "extra_usage") {
		t.Fatal("credential or paid credit data leaked into quota snapshot")
	}
}

func TestUsageMissingPercentageAndExpiredLogin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"five_hour":{"resets_at":"2030-01-01T12:00:00Z"}}`))
	}))
	defer srv.Close()
	v, err := fetchUsage(context.Background(), srv.Client(), srv.URL, Cred{AccessToken: "test"})
	if err != nil || v.Groups[0].Windows[0].RemainingPercent != nil {
		t.Fatalf("missing percent fabricated: %+v %v", v, err)
	}
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	if _, err := ReadUsage(context.Background()); !errors.Is(err, usage.ErrLogin) {
		t.Fatalf("missing login: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(`{"claudeAiOauth":{"accessToken":"expired","expiresAt":1}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadUsage(context.Background()); !errors.Is(err, usage.ErrLogin) {
		t.Fatalf("expired login: %v", err)
	}
}
