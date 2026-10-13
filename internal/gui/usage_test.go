package gui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/config"
)

func TestUsageEndpointPreservesUnsupportedAndMissingLoginStates(t *testing.T) {
	t.Setenv("PICOGENT_CODEX_HOME", t.TempDir())
	for _, provider := range []config.Provider{config.ProviderOpenAI, config.ProviderCodex} {
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
	}
}
