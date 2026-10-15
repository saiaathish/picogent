package gui

import (
	"encoding/json"
	"github.com/saiaathish/picogent/internal/agyauth"
	"net/http"
)

func (s *server) setupCheck(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Provider string `json:"provider"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&input) != nil || input.Provider != "antigravity" {
		http.Error(w, "Choose Antigravity to check this connection.", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(agyauth.CheckConnection(r.Context()))
}
