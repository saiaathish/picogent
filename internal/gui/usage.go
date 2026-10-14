package gui

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/saiaathish/picogent/internal/agyauth"
	"github.com/saiaathish/picogent/internal/claudeauth"
	"github.com/saiaathish/picogent/internal/codexauth"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/usage"
)

func supportsUsage(cfg config.Config) bool {
	switch cfg.Provider {
	case config.ProviderCodex:
		return true
	case config.ProviderQuadCode:
		return cfg.AnthropicKeyResolved() == ""
	case config.ProviderAntigravity:
		return !agyauth.UsesGeminiAPIKey()
	default:
		return false
	}
}

func (s *server) usageAPI(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	provider := cfg.Provider
	supported := supportsUsage(cfg)
	out := map[string]any{"provider": provider, "available": false, "supported": supported}
	if !supported {
		out["message"] = "Usage limits are omitted for this provider or API key mode."
	} else {
		var value usage.Snapshot
		var err error
		var label, summary string
		switch provider {
		case config.ProviderCodex:
			label, summary = "Codex", "codex"
			value, err = codexauth.ReadUsage(r.Context())
		case config.ProviderQuadCode:
			label, summary = "Claude Code", "claude"
			value, err = claudeauth.ReadUsage(r.Context())
		case config.ProviderAntigravity:
			label, summary = "Antigravity", cfg.Model
			value, err = agyauth.ReadUsage(r.Context())
		}
		out["summary_id"] = summary
		if err != nil {
			out["message"] = label + " usage limits are unavailable right now. Try refreshing."
			if errors.Is(err, codexauth.ErrUsageLogin) || errors.Is(err, usage.ErrLogin) {
				out["message"] = "Sign in to " + label + " to see usage limits."
			}
		} else {
			out["available"] = true
			out["usage"] = value
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(out)
}
