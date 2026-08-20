package gui

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/app"
	"github.com/saiaathish/picogent/internal/evolve"
)

func (s *server) evolveAPI(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	ws := s.cfg.Workspace
	s.mu.Unlock()
	switch r.Method {
	case http.MethodGet:
		store, err := evolve.Load(ws)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		active := 0
		for _, p := range store.Playbooks {
			if !p.Archived {
				active++
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"habits":    store.Habits,
			"playbooks": store.Playbooks,
			"summary":   evolve.Summary(store),
			"counts": map[string]int{
				"habits":    len(store.Habits),
				"playbooks": active,
			},
		})
	case http.MethodDelete:
		var req struct {
			Kind string `json:"kind"` // habit | playbook
			ID   string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		store, err := evolve.Load(ws)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		switch strings.ToLower(req.Kind) {
		case "habit":
			out := store.Habits[:0]
			for _, h := range store.Habits {
				if h.ID != req.ID {
					out = append(out, h)
				}
			}
			store.Habits = out
		case "playbook":
			for i := range store.Playbooks {
				if store.Playbooks[i].ID == req.ID {
					store.Playbooks[i].Archived = true
				}
			}
		default:
			http.Error(w, "kind must be habit or playbook", 400)
			return
		}
		if err := evolve.Save(store); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		s.mu.Lock()
		ag := s.ag
		s.mu.Unlock()
		app.RefreshMemory(ag, ws)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "summary": evolve.Summary(store)})
	default:
		http.Error(w, "GET or DELETE", 405)
	}
}

// reflectAfterTurn runs quiet self-evolution after a successful agent turn.
func (s *server) reflectAfterTurn(prompt string, result agent.Result) {
	s.mu.Lock()
	ws := s.cfg.Workspace
	s.mu.Unlock()
	sig := evolve.Signal{
		Workspace:     ws,
		UserPrompt:    prompt,
		AssistantText: result.Text,
		FilesChanged:  result.FilesChanged,
		ToolRounds:    result.ToolRounds,
		GoalDone:      result.GoalDone,
		Verified:      result.Verified,
	}
	if !evolve.WorthReflecting(sig) {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()
		client := s.lightClient()
		model := s.lightModelID()
		delta, err := evolve.Reflect(ctx, client, model, sig)
		if err != nil || delta.Message == "" {
			return
		}
		s.mu.Lock()
		ag := s.ag
		workspace := s.cfg.Workspace
		s.mu.Unlock()
		app.RefreshMemory(ag, workspace)
		s.emit(event{
			Type:    "evolve",
			Text:    delta.Message,
			Summary: evolve.Summary(delta.Store),
			Kind:    "memory",
			Status:  "done",
		})
	}()
}
