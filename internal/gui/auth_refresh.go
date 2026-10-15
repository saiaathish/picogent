package gui

import (
	"context"
	"encoding/json"
	"github.com/saiaathish/picogent/internal/agyauth"
	"github.com/saiaathish/picogent/internal/claudeauth"
	"github.com/saiaathish/picogent/internal/codexauth"
	"net/http"
	"time"
)

func (s *server) refreshLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Provider string `json:"provider"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in) != nil {
		http.Error(w, "Invalid provider", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	var err error
	switch in.Provider {
	case "codex":
		_, _, err = codexauth.Token(ctx)
	case "quadcode":
		_, err = claudeauth.TokenContext(ctx)
	case "antigravity":
		_, err = agyauth.TokenContext(ctx)
	default:
		http.Error(w, "This provider requires reconnecting through its login flow", 400)
		return
	}
	detail := "Login is ready."
	if err != nil {
		detail = "Login could not be refreshed. Use Reconnect to sign in again."
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": err == nil, "detail": detail})
}
