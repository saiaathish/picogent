package gui

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

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
			writeGUIError(w, err.Error(), 500)
			return
		}
		s.mu.Lock()
		ws := s.cfg.Workspace
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"projects":   list,
			"current_id": current,
			"workspace":  ws,
		})
	case http.MethodPost:
		s.configTxMu.Lock()
		defer s.configTxMu.Unlock()
		var in struct {
			Action string `json:"action"`
			Name   string `json:"name"`
			Path   string `json:"path"`
			ID     string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeGUIError(w, err.Error(), 400)
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
				writeGUIError(w, err.Error(), 500)
				return
			}
			expected, next, p, err := projects.PrepareAdd("", path)
			if err != nil {
				writeGUIError(w, err.Error(), 400)
				return
			}
			res, err := s.switchPreparedProject(expected, next, p)
			if err != nil {
				writeGUIError(w, err.Error(), 500)
				return
			}
			s.writeProjectSwitch(w, p, res)
		case "add":
			expected, next, p, err := projects.PrepareAdd(in.Name, in.Path)
			if err != nil {
				writeGUIError(w, err.Error(), 400)
				return
			}
			res, err := s.switchPreparedProject(expected, next, p)
			if err != nil {
				writeGUIError(w, err.Error(), 500)
				return
			}
			s.writeProjectSwitch(w, p, res)
		case "switch":
			expected, next, p, err := projects.PrepareSwitch(in.ID)
			if err != nil {
				writeGUIError(w, err.Error(), 404)
				return
			}
			res, err := s.switchPreparedProject(expected, next, p)
			if err != nil {
				writeGUIError(w, err.Error(), 500)
				return
			}
			s.writeProjectSwitch(w, p, res)
		case "remove":
			s.mu.Lock()
			activeWorkspace := s.cfg.Workspace
			s.mu.Unlock()
			reg, err := projects.Load()
			if err != nil {
				writeGUIError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			for _, p := range reg.Projects {
				if p.ID == in.ID && (p.ID == reg.Current || filepath.Clean(p.Path) == filepath.Clean(activeWorkspace)) {
					writeGUIError(w, "switch away from the active project before removing it", http.StatusConflict)
					return
				}
			}
			if err := projects.Remove(in.ID); err != nil {
				writeGUIError(w, err.Error(), 404)
				return
			}
			w.WriteHeader(204)
		default:
			writeGUIError(w, "action must be add, switch, or remove", 400)
		}
	default:
		writeGUIError(w, "GET or POST only", 405)
	}
}

func (s *server) switchPreparedProject(expected, next projects.Registry, p projects.Project) (projectSwitchResult, error) {
	s.mu.Lock()
	previousPath := s.cfg.Workspace
	s.mu.Unlock()
	res, err := s.switchWorkspace(p.Path)
	if err != nil {
		s.mu.Lock()
		currentPath := s.cfg.Workspace
		s.mu.Unlock()
		if filepath.Clean(currentPath) != filepath.Clean(previousPath) {
			if _, restoreErr := s.switchWorkspace(previousPath); restoreErr != nil {
				return projectSwitchResult{}, fmt.Errorf("switch project: %w; restore previous workspace: %v", err, restoreErr)
			}
		}
		return projectSwitchResult{}, err
	}
	if err := projects.SaveIfCurrent(expected, next); err != nil {
		if filepath.Clean(previousPath) != filepath.Clean(p.Path) {
			if _, restoreErr := s.switchWorkspace(previousPath); restoreErr != nil {
				return projectSwitchResult{}, fmt.Errorf("commit project selection: %w; restore previous workspace: %v", err, restoreErr)
			}
		}
		return projectSwitchResult{}, fmt.Errorf("commit project selection: %w", err)
	}
	return res, nil
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
	if err := s.persistConfig(cfg); err != nil {
		return projectSwitchResult{}, fmt.Errorf("save workspace config: %w", err)
	}
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
	if err := a.SetTaskSession(sessID); err != nil {
		closeCandidateAgent(a)
		return projectSwitchResult{}, fmt.Errorf("load durable task state: %w", err)
	}

	s.mu.Lock()
	oldWorkspace := s.cfg.Workspace
	oldSession := s.sessionID
	oldHist := s.hist
	s.abortTurnLocked()
	var saveErr error
	if oldSession != "" && len(oldHist) > 0 {
		saveErr = session.SaveMessages(oldWorkspace, oldSession, oldHist)
	}
	s.cfg = cfg
	s.ag = a
	s.hist = hist
	s.sessionID = sessID
	s.mu.Unlock()
	if saveErr != nil {
		s.emit(event{Type: "error", Text: fmt.Sprintf("couldn't save session: %v", saveErr)})
	}
	s.attachRouterHook()
	s.emit(event{Type: "undo", Status: "cleared"})
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
