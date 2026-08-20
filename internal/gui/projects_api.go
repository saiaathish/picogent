package gui

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/saiaathish/picogent/internal/app"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/projects"
	"github.com/saiaathish/picogent/internal/session"
)

func (s *server) projectsAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, current, err := projects.List()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		s.mu.Lock()
		ws := s.cfg.Workspace
		s.mu.Unlock()
		_, _, _ = projects.Ensure(ws)
		list, current, _ = projects.List()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"projects":   list,
			"current_id": current,
			"workspace":  ws,
		})
	case http.MethodPost:
		var in struct {
			Action string `json:"action"`
			Name   string `json:"name"`
			Path   string `json:"path"`
			ID     string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		switch in.Action {
		case "add":
			p, err := projects.Add(in.Name, in.Path)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			if err := s.switchWorkspace(p.Path); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(p)
		case "switch":
			p, err := projects.Switch(in.ID)
			if err != nil {
				http.Error(w, err.Error(), 404)
				return
			}
			if err := s.switchWorkspace(p.Path); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(p)
		case "remove":
			if err := projects.Remove(in.ID); err != nil {
				http.Error(w, err.Error(), 404)
				return
			}
			w.WriteHeader(204)
		default:
			http.Error(w, "action must be add, switch, or remove", 400)
		}
	default:
		http.Error(w, "GET or POST only", 405)
	}
}

func (s *server) switchWorkspace(path string) error {
	s.mu.Lock()
	cfg := s.cfg
	cfg.Workspace = path
	s.mu.Unlock()
	a, err := app.Build(cfg)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg = cfg
	s.ag = a
	s.hist = nil
	s.sessionID = session.New(path).ID
	s.mu.Unlock()
	s.attachRouterHook()
	_ = config.Save(cfg)
	_, _, _ = projects.Ensure(path)
	s.emit(event{Type: "system", Text: "Switched to " + path})
	return nil
}

func (s *server) ensureProject() {
	s.mu.Lock()
	ws := s.cfg.Workspace
	s.mu.Unlock()
	if ws == "" {
		return
	}
	if _, err := os.Stat(ws); err != nil {
		return
	}
	_, _, _ = projects.Ensure(ws)
}
