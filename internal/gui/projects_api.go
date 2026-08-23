package gui

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"

	"github.com/saiaathish/picogent/internal/app"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/folderpick"
	"github.com/saiaathish/picogent/internal/projects"
	"github.com/saiaathish/picogent/internal/session"
	"github.com/saiaathish/picogent/internal/taskstate"
)

type projectSwitchResult struct {
	Project   projects.Project `json:"project"`
	SessionID string           `json:"session_id"`
	Messages  []transcriptLine `json:"messages"`
	Task      *taskstate.Task  `json:"task"`
}

func (s *server) writeProjectSwitch(w http.ResponseWriter, p projects.Project, res projectSwitchResult) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":         p.ID,
		"name":       p.Name,
		"path":       p.Path,
		"session_id": res.SessionID,
		"messages":   res.Messages,
		"task":       res.Task,
	})
}

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
		case "pick":
			path, err := folderpick.Choose("Select a project folder")
			if errors.Is(err, folderpick.ErrCancelled) {
				w.WriteHeader(204)
				return
			}
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			p, err := projects.Add("", path)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			res, err := s.switchWorkspace(p.Path)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			s.writeProjectSwitch(w, p, res)
		case "add":
			p, err := projects.Add(in.Name, in.Path)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			res, err := s.switchWorkspace(p.Path)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			s.writeProjectSwitch(w, p, res)
		case "switch":
			p, err := projects.Switch(in.ID)
			if err != nil {
				http.Error(w, err.Error(), 404)
				return
			}
			res, err := s.switchWorkspace(p.Path)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			s.writeProjectSwitch(w, p, res)
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

func (s *server) switchWorkspace(path string) (projectSwitchResult, error) {
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	cfg.Workspace = path
	res, err := s.replaceWorkspace(cfg)
	if err != nil {
		return projectSwitchResult{}, err
	}
	_ = config.Save(cfg)
	return res, nil
}

// replaceWorkspace rebuilds every project-scoped runtime resource together:
// task store, project rules, memory, goal, MCP wiring, trace, and session
// history. The old generation is invalidated at the swap, so a turn that was
// already running can only finish against its captured old agent/session.
func (s *server) replaceWorkspace(cfg config.Config) (projectSwitchResult, error) {
	a, err := app.Build(cfg)
	if err != nil {
		return projectSwitchResult{}, err
	}

	path := cfg.Workspace
	sessID, hist := initialSession(path)
	a.SetTaskSession(sessID)

	s.mu.Lock()
	oldWorkspace := s.cfg.Workspace
	oldSession := s.sessionID
	oldHist := s.hist
	s.abortTurnLocked()
	if oldSession != "" && len(oldHist) > 0 {
		_ = session.SaveMessages(oldWorkspace, oldSession, oldHist)
	}
	s.cfg = cfg
	s.ag = a
	s.hist = hist
	s.sessionID = sessID
	s.mu.Unlock()
	s.attachRouterHook()
	_, _, _ = projects.Ensure(path)
	s.emit(event{Type: "system", Text: "Opened " + projects.NameFromPath(path)})
	s.emitTaskSnapshot(sessID)
	s.emit(event{Type: "overview", Text: "refresh"})
	s.invalidatePromptRecs()
	s.emit(event{Type: "prompts_refresh", Text: "all"})
	return projectSwitchResult{
		SessionID: sessID,
		Messages:  messagesToTranscript(hist),
		Task:      a.TaskSnapshot(),
	}, nil
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
