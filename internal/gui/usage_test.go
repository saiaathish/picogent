package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/config"
)

func TestUsageEndpointPreservesUnsupportedAndMissingLoginStates(t *testing.T) {
	t.Setenv("PICOGENT_CODEX_HOME", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("PICOGENT_AGY_HOME", t.TempDir())
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("PICOGENT_ANTHROPIC_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("PICOGENT_GEMINI_API_KEY", "")
	// An expired file avoids accessing the host's macOS Keychain.
	if err := os.WriteFile(filepath.Join(os.Getenv("PICOGENT_AGY_HOME"), "antigravity-oauth-token"), []byte(`{"token":{"access_token":"expired","expiry":"2000-01-01T00:00:00Z"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, provider := range []config.Provider{config.ProviderOpenAI, config.ProviderOpenCode, config.ProviderOllama, config.ProviderCodex, config.ProviderQuadCode, config.ProviderAntigravity} {
		s := newLoopbackAPITestServer(t)
		s.cfg.Provider = provider
		res := httptest.NewRecorder()
		s.Handler().ServeHTTP(res, loopbackAPIRequest(http.MethodGet, "/api/usage", ""))
		if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"available":false`) {
			t.Fatalf("%s quota response = %d %s", provider, res.Code, res.Body.String())
		}
		if strings.Contains(res.Body.String(), `"remaining_percent":0`) {
			t.Fatal("unavailable quota displayed as zero")
		}
		if res.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("account quota may be cached by browser")
		}
		var out struct {
			Supported bool            `json:"supported"`
			Provider  config.Provider `json:"provider"`
			Message   string          `json:"message"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		wantSupported := provider == config.ProviderCodex || provider == config.ProviderQuadCode || provider == config.ProviderAntigravity
		if out.Supported != wantSupported || out.Provider != provider {
			t.Fatalf("incorrect usage support: %+v", out)
		}
		if wantSupported && !strings.HasPrefix(out.Message, "Sign in to ") {
			t.Fatalf("login state lost: %+v", out)
		}
	}
}

func TestUsageOmittedInAPIKeyModes(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("PICOGENT_ANTHROPIC_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("PICOGENT_GEMINI_API_KEY", "")
	t.Setenv("PICOGENT_AGY_HOME", t.TempDir())
	cfg := config.Config{Provider: config.ProviderQuadCode}
	if !supportsUsage(cfg) {
		t.Fatal("Claude OAuth omitted")
	}
	cfg.AnthropicKey = "fixture-key"
	if supportsUsage(cfg) {
		t.Fatal("Claude configured API key shows subscription limits")
	}
	cfg.AnthropicKey = ""
	t.Setenv("ANTHROPIC_API_KEY", "fixture-key")
	if supportsUsage(cfg) {
		t.Fatal("Claude environment API key shows subscription limits")
	}
	cfg.Provider = config.ProviderAntigravity
	if !supportsUsage(cfg) {
		t.Fatal("Antigravity OAuth omitted")
	}
	t.Setenv("GEMINI_API_KEY", "fixture-key")
	if supportsUsage(cfg) {
		t.Fatal("Antigravity API mode shows subscription limits")
	}
	for _, provider := range []config.Provider{config.ProviderQuadCode, config.ProviderAntigravity} {
		s := newLoopbackAPITestServer(t)
		s.cfg.Provider = provider
		res := httptest.NewRecorder()
		s.Handler().ServeHTTP(res, loopbackAPIRequest(http.MethodGet, "/api/usage", ""))
		if !strings.Contains(res.Body.String(), `"supported":false`) || strings.Contains(res.Body.String(), "fixture-key") {
			t.Fatalf("API mode not omitted: %s", res.Body.String())
		}
	}
}
