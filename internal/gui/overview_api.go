package gui

import (
	"encoding/json"
	"net/http"

	"github.com/saiaathish/picogent/internal/evolve"
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
	ev, _ := evolve.Load(ws)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"workspace":     store.Workspace,
		"files_read":    store.FilesRead,
		"files_changed": store.FilesChanged,
		"tool_counts":   store.ToolCounts,
		"turns":         store.Turns,
		"searches":      store.Searches,
		"last_test":     store.LastTest,
		"knowledge":     store.Knowledge,
		"overview":      store.Overview,
		"updated_at":    store.UpdatedAt,
		"evolve": map[string]any{
			"summary": evolve.Summary(ev),
			"habits":  len(ev.Habits),
			"playbooks": func() int {
				n := 0
				for _, p := range ev.Playbooks {
					if !p.Archived {
						n++
					}
				}
				return n
			}(),
		},
	})
}
