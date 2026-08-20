package gui

import (
	"encoding/json"
	"net/http"

	"github.com/saiaathish/picogent/internal/learn"
)

func (s *server) overviewAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", 405)
		return
	}
	s.mu.Lock()
	ws := s.cfg.Workspace
	s.mu.Unlock()
	store, err := learn.Load(ws)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(store)
}
