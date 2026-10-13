package codexauth

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
	"time"
)

func TestUsageNormalizesReportedWindowsAndPreservesMissingValues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer test-token" || r.Header.Get("ChatGPT-Account-ID") != "test-account" {
			t.Error("usage request did not use read-only account auth")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"plan_type":"pro","rate_limit":{"allowed":true,"primary_window":{"used_percent":23,"limit_window_seconds":18000,"reset_at":2000000000},"secondary_window":{"used_percent":105,"limit_window_seconds":604800}},"additional_rate_limits":[{"metered_feature":"astra","limit_name":"Astra","rate_limit":{"primary_window":{"limit_window_seconds":18000,"reset_at":2000000100}}}]}`))
	}))
	defer srv.Close()
	got, err := fetchUsage(context.Background(), srv.Client(), srv.URL, "test-token", "test-account")
	if err != nil {
		t.Fatal(err)
	}
	if got.Plan != "pro" || len(got.Groups) != 2 {
		t.Fatalf("unexpected account limits: %+v", got)
	}
	first := got.Groups[0].Windows[0]
	if first.RemainingPercent == nil || *first.RemainingPercent != 77 || first.ResetsAt == nil || *first.ResetsAt != 2000000000 {
		t.Fatalf("primary quota = %+v", first)
	}
	second := got.Groups[0].Windows[1]
	if second.RemainingPercent == nil || *second.RemainingPercent != 0 || second.ResetsAt != nil {
		t.Fatalf("secondary quota = %+v", second)
	}
	if got.Groups[1].Windows[0].RemainingPercent != nil {
		t.Fatal("missing percentage was fabricated as zero")
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "test-token") || strings.Contains(string(raw), "test-account") {
		t.Fatal("usage snapshot exposed credentials or account ID")
	}
}

func TestUsageErrorsNeverExposeProviderResponse(t *testing.T) {
	for _, status := range []int{401, 403, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte("private-provider-response"))
			}))
			defer srv.Close()
			_, err := fetchUsage(context.Background(), srv.Client(), srv.URL, "token", "account")
			if err == nil || strings.Contains(err.Error(), "private-provider-response") {
				t.Fatalf("unsafe usage error: %v", err)
			}
			if status == 401 && !errors.Is(err, ErrUsageLogin) {
				t.Fatalf("401 = %v", err)
			}
		})
	}
}

func TestUsageNeverFollowsAuthRedirects(t *testing.T) {
	forwarded := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded = true
	}))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer origin.Close()
	client := origin.Client()
	client.CheckRedirect = usageHTTP.CheckRedirect
	_, err := fetchUsage(context.Background(), client, origin.URL, "private-token", "private-account")
	if !errors.Is(err, ErrUsageUnavailable) || forwarded {
		t.Fatalf("quota request followed a redirect: forwarded=%v err=%v", forwarded, err)
	}
}

func TestUsageCacheFollowsLoginIdentity(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PICOGENT_CODEX_HOME", home)
	writeAuth := func(token string) {
		b, _ := json.Marshal(map[string]any{"tokens": map[string]string{"access_token": token, "account_id": "test-account"}})
		if err := os.WriteFile(filepath.Join(home, "auth.json"), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"plan_type":"plus","rate_limit":{"primary_window":{"used_percent":25}}}`))
	}))
	defer srv.Close()
	previous := usageHTTP
	usageHTTP = &http.Client{Transport: usageTestTransport{target: srv.URL, base: srv.Client().Transport}}
	reset := func() {
		usageCache.Lock()
		usageCache.Until = time.Time{}
		usageCache.Unlock()
	}
	reset()
	t.Cleanup(func() { usageHTTP = previous; reset() })
	writeAuth("first-login")
	for i := 0; i < 2; i++ {
		if _, err := ReadUsage(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("same login fetched %d times", calls)
	}
	writeAuth("second-login")
	if _, err := ReadUsage(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("new login reused old account cache: %d calls", calls)
	}
}

type usageTestTransport struct {
	target string
	base   http.RoundTripper
}

func (t usageTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method != http.MethodGet || r.URL.String() != "https://chatgpt.com/backend-api/wham/usage" {
		return nil, errors.New("unexpected request")
	}
	next, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, t.target, nil)
	next.Header = r.Header.Clone()
	return t.base.RoundTrip(next)
}
