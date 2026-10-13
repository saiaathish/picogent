package gui

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/saiaathish/picogent/internal/codexauth"
	"github.com/saiaathish/picogent/internal/config"
)

func (s *server) usageAPI(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	provider := s.cfg.Provider
	s.mu.Unlock()
	out := map[string]any{"provider": provider, "available": false}
	if provider != config.ProviderCodex {
		out["message"] = "Subscription usage limits are currently available for Codex. This provider does not expose limits to Picogent."
	} else {
		usage, err := codexauth.ReadUsage(r.Context())
		if err != nil {
			out["message"] = "Codex usage limits are unavailable right now. Try refreshing."
			if errors.Is(err, codexauth.ErrUsageLogin) {
				out["message"] = "Sign in to Codex to see usage limits."
			}
		} else {
			out["available"] = true
			out["usage"] = usage
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(out)
}
