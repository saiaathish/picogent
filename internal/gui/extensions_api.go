package gui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/app"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/extensions"
	"github.com/saiaathish/picogent/internal/taskstate"
)

func (s *server) extensionsAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.extensionsList(w, r)
	case http.MethodPost:
		s.extensionsAction(w, r)
	default:
		writeGUIError(w, "method not allowed", 405)
	}
}

func (s *server) extensionsList(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	cfg := s.cfg
	ws := cfg.Workspace
	s.mu.Unlock()

	installed, _ := extensions.InstalledSet(ws, cfg.Extensions.InstalledSkills)
	items := extensions.Search("", installed)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"items":     items,
		"installed": len(installedKeys(installed)),
		"dismissed": cfg.Extensions.Dismissed,
	})
}

func (s *server) extensionsAction(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Action  string `json:"action"`
		ID      string `json:"id"`
		Query   string `json:"query"`
		Kind    string `json:"kind"`
		Page    int    `json:"page"`
		UndoID  string `json:"undo_id"`
		Approve bool   `json:"approve"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeGUIError(w, err.Error(), 400)
		return
	}

	switch in.Action {
	case "search":
		s.extensionsSearch(w, in.Query)
	case "browse":
		s.extensionsBrowse(w, r, in.Query, in.Kind, in.Page)
	case "assistant":
		s.extensionsAssistant(w, in.Query)
	case "recommend":
		s.extensionsRecommend(w, in.Query)
	case "install":
		s.extensionsInstall(w, in.ID, in.Approve)
	case "undo":
		s.extensionsUndo(w, in.UndoID)
	case "dismiss":
		s.extensionsDismiss(w, in.ID)
	case "auth_done":
		s.extensionsAuthDone(w, in.ID)
	case "activate":
		s.extensionsActivate(w, in.ID)
	case "essential":
		s.extensionsEssential(w, in.ID)
	case "cleanup":
		s.extensionsCleanup(w)
	default:
		writeGUIError(w, "unknown action", 400)
	}
}

func (s *server) extensionsSearch(w http.ResponseWriter, query string) {
	s.extensionsBrowse(w, nil, query, "", 1)
}

func (s *server) extensionsBrowse(w http.ResponseWriter, _ *http.Request, query, kind string, page int) {
	s.mu.Lock()
	cfg := s.cfg
	ws := cfg.Workspace
	s.mu.Unlock()

	installed, _ := extensions.InstalledSet(ws, cfg.Extensions.InstalledSkills)
	items, stats, err := extensions.Browse(extensions.Kind(kind), query, page, installed)
	if err != nil {
		writeGUIError(w, err.Error(), 500)
		return
	}
	items = extensions.ActiveStatus(items, cfg.Extensions.EssentialPlugins, cfg.Extensions.ActiveTransient)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"items":   items,
		"stats":   stats,
		"page":    page,
		"library": "claude-official",
	})
}

func (s *server) extensionsAssistant(w http.ResponseWriter, query string) {
	s.mu.Lock()
	cfg := s.cfg
	ws := cfg.Workspace
	s.mu.Unlock()

	installed, _ := extensions.InstalledSet(ws, cfg.Extensions.InstalledSkills)
	msg, items := extensions.AssistantFind(query, installed)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"message": msg,
		"items":   items,
	})
}

func (s *server) extensionsRecommend(w http.ResponseWriter, prompt string) {
	s.mu.Lock()
	cfg := s.cfg
	ws := cfg.Workspace
	s.mu.Unlock()

	installed, _ := extensions.InstalledSet(ws, cfg.Extensions.InstalledSkills)
	recs := extensions.Recommend(prompt, installed, dismissedSet(cfg.Extensions.Dismissed))
	out := extensions.ToSearchResults(recs, installed)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": out})
}

func (s *server) extensionsInstall(w http.ResponseWriter, id string, approved bool) {
	var it *extensions.Item
	if strings.HasPrefix(id, "claude:") {
		name := strings.TrimPrefix(id, "claude:")
		it = extensions.ByClaudeName(name)
	} else {
		it = extensions.ByID(id)
	}
	if it == nil {
		writeGUIError(w, "extension not found", 404)
		return
	}

	s.configTxMu.Lock()
	defer s.configTxMu.Unlock()
	s.mu.Lock()
	cfg := s.cfg
	mode := cfg.Mode
	ws := cfg.Workspace
	s.mu.Unlock()

	if mode == config.ModeSafe && !approved {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"needs_approval": true,
			"item":           extensions.ToSearchResults([]extensions.Item{*it}, nil)[0],
		})
		return
	}

	res, entry, err := extensions.Install(*it, ws)
	if err != nil {
		// Claude plugins: activate on-demand from local cache.
		if strings.HasPrefix(id, "claude:") {
			if actErr := extensions.ActivateClaudePlugin(strings.TrimPrefix(id, "claude:")); actErr != nil {
				writeGUIError(w, actErr.Error(), 500)
				return
			}
			if err := s.persistExtensionUpdate(func(ext *config.ExtensionsConfig) {
				ext.ActiveTransient = appendUnique(ext.ActiveTransient, id)
			}); err != nil {
				writeGUIError(w, "extension activated, but couldn't save settings: "+err.Error(), 500)
				return
			}
			_ = s.rebuildAgent()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok": true, "transient": true,
				"result": map[string]string{"message": "Loaded " + it.Name + " for this session"},
			})
			return
		}
		writeGUIError(w, err.Error(), 500)
		return
	}

	s.mu.Lock()
	s.undoStack = append(s.undoStack, entry)
	s.mu.Unlock()
	if err := s.persistExtensionUpdate(func(ext *config.ExtensionsConfig) {
		switch it.Kind {
		case extensions.KindSkill:
			ext.InstalledSkills = appendUnique(ext.InstalledSkills, it.SkillPath)
		case extensions.KindPlugin:
			ext.InstalledPlugins = appendUnique(ext.InstalledPlugins, it.ID)
		}
	}); err != nil {
		writeGUIError(w, "extension installed, but couldn't save settings: "+err.Error(), 500)
		return
	}

	if err := s.rebuildAgent(); err != nil {
		writeGUIError(w, err.Error(), 500)
		return
	}

	auto := mode == config.ModeFast && it.Kind == extensions.KindMCP
	s.emit(event{
		Type:    "extension_installed",
		Text:    res.Message,
		Summary: res.UndoID,
		Kind:    string(it.Kind),
		Status:  ternary(auto, "auto", "manual"),
		Path:    res.MCPName,
	})
	if res.AuthNeeded {
		s.emit(event{
			Type:    "extension_auth",
			Text:    it.Name,
			Summary: res.AuthHint,
			Kind:    "auth",
			Path:    it.ID,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"result":  res,
		"auto":    auto,
		"undo_id": res.UndoID,
	})
}

func (s *server) extensionsUndo(w http.ResponseWriter, undoID string) {
	s.configTxMu.Lock()
	defer s.configTxMu.Unlock()
	s.mu.Lock()
	var entry *extensions.UndoEntry
	rest := s.undoStack[:0]
	for _, e := range s.undoStack {
		if e.ID == undoID {
			copy := e
			entry = &copy
			continue
		}
		rest = append(rest, e)
	}
	s.undoStack = rest
	s.mu.Unlock()

	if entry == nil {
		writeGUIError(w, "undo entry not found", 404)
		return
	}
	if err := extensions.Undo(*entry); err != nil {
		writeGUIError(w, err.Error(), 500)
		return
	}

	if err := s.persistExtensionUpdate(func(ext *config.ExtensionsConfig) {
		if entry.Kind == extensions.KindSkill && entry.SkillPath != "" {
			ext.InstalledSkills = removeString(ext.InstalledSkills, filepathBase(entry.SkillPath))
		}
		if entry.Kind == extensions.KindPlugin {
			ext.InstalledPlugins = removeString(ext.InstalledPlugins, entry.ExtID)
		}
	}); err != nil {
		writeGUIError(w, "extension removed, but couldn't save settings: "+err.Error(), 500)
		return
	}

	_ = s.rebuildAgent()
	s.emit(event{Type: "extension_undo", Text: "Removed " + entry.ExtID, Summary: undoID})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *server) extensionsDismiss(w http.ResponseWriter, id string) {
	if id == "" {
		writeGUIError(w, "id required", 400)
		return
	}
	s.configTxMu.Lock()
	defer s.configTxMu.Unlock()
	if err := s.persistExtensionUpdate(func(ext *config.ExtensionsConfig) {
		ext.Dismissed = appendUnique(ext.Dismissed, id)
	}); err != nil {
		writeGUIError(w, "couldn't save extension preference: "+err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *server) extensionsAuthDone(w http.ResponseWriter, id string) {
	s.emit(event{Type: "system", Text: "Extension authorized: " + id})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *server) rebuildAgent() error {
	s.mu.Lock()
	cfg := s.cfg
	sessionID := s.sessionID
	src := s.ag
	var inheritedAlwaysAllowed []string
	if src != nil && src.Gate != nil {
		inheritedAlwaysAllowed = src.Gate.AlwaysAllowedTools()
	}
	s.mu.Unlock()

	var runtime agent.RuntimeState
	var taskStoreSnapshot *taskstate.Store
	if src != nil {
		runtime = src.RuntimeSnapshot()
		taskStoreSnapshot = src.TaskStoreSnapshot()
	}
	a, err := app.Build(cfg)
	if err != nil {
		return err
	}
	allowed := append([]string(nil), cfg.Extensions.AlwaysAllowTools...)
	for _, tool := range inheritedAlwaysAllowed {
		allowed = appendUnique(allowed, tool)
	}
	if a.Gate != nil {
		a.Gate.SetAlwaysAllowed(allowed)
	}
	if src != nil {
		// Rebuilding the provider/tool registry must not detach the live task
		// contract. In particular, cleanup after a turn used to replace the
		// agent with an empty TaskSession, hiding both the durable task and the
		// checkpoint-backed undo action from the GUI until the next turn.
		if taskStoreSnapshot != nil {
			a.SetTaskStore(taskStoreSnapshot)
		}
		a.SetProjectRules(runtime.ProjectRules)
		a.SetSkillRules(runtime.SkillRules)
		a.SetMemory(runtime.Memory)
		a.SetGoalState(runtime.Goal, runtime.GoalRevision)
		a.SetTrace(runtime.Trace)
		a.SetTaskMode(runtime.TaskMode)
		if sessionID != "" {
			if err := a.SetTaskSession(sessionID); err != nil {
				a.Close()
				return fmt.Errorf("restore durable task session %q after agent rebuild: %w", sessionID, err)
			}
		}
	}
	s.mu.Lock()
	s.ag = a
	s.mu.Unlock()
	s.attachRouterHook()
	return nil
}

func (s *server) maybeRecommendExtensions(prompt string) {
	s.mu.Lock()
	cfg := s.cfg
	ws := cfg.Workspace
	s.mu.Unlock()

	installed, _ := extensions.InstalledSet(ws, cfg.Extensions.InstalledSkills)
	recs := extensions.Recommend(prompt, installed, dismissedSet(cfg.Extensions.Dismissed))

	// Include Claude library matches.
	claudeItems, _ := extensions.LoadClaudeLibrary()
	lower := strings.ToLower(prompt)
	for _, sr := range claudeItems {
		if extensions.MatchScore(lower, sr.Keywords) >= 8 && !installed[sr.ID] {
			recs = append(recs, extensions.Item{
				ID: sr.ID, Name: sr.Name, Kind: sr.Kind,
				Description: sr.Description, Keywords: sr.Keywords,
			})
		}
	}

	if len(recs) > 1 {
		recs = recs[:1]
	}
	for _, it := range recs {
		s.emit(event{
			Type:    "extension_recommend",
			Text:    it.Name,
			Summary: it.Description,
			Kind:    string(it.Kind),
			Path:    it.ID,
			Status:  ternary(it.AuthRequired, "auth", "on-demand"),
		})
	}

}

func (s *server) cleanupExtensionPool() error {
	s.configTxMu.Lock()
	defer s.configTxMu.Unlock()
	s.mu.Lock()
	suppressRebuild := s.suppressExtensionRebuild
	cfg := s.cfg
	ws := cfg.Workspace
	s.mu.Unlock()
	if suppressRebuild {
		return nil
	}

	pool := extensions.NewPool(ws, cfg.Extensions.EssentialPlugins, cfg.Extensions.ActiveTransient)
	if err := pool.CleanupTransient(); err != nil {
		return fmt.Errorf("clean up extensions: %w", err)
	}
	if err := s.persistExtensionUpdate(func(ext *config.ExtensionsConfig) {
		ext.ActiveTransient = append([]string(nil), pool.Transient...)
	}); err != nil {
		return fmt.Errorf("extensions cleaned up, but couldn't save settings: %w", err)
	}
	return s.rebuildAgent()
}

func (s *server) extensionsActivate(w http.ResponseWriter, id string) {
	if id == "" {
		writeGUIError(w, "id required", 400)
		return
	}
	s.configTxMu.Lock()
	defer s.configTxMu.Unlock()
	if strings.HasPrefix(id, "claude:") {
		if err := extensions.ActivateClaudePlugin(strings.TrimPrefix(id, "claude:")); err != nil {
			writeGUIError(w, err.Error(), 500)
			return
		}
	} else if it := extensions.ByID(id); it != nil && it.MCP != nil {
		if err := extensions.ActivateMCPCatalog(*it); err != nil {
			writeGUIError(w, err.Error(), 500)
			return
		}
	}
	if err := s.persistExtensionUpdate(func(ext *config.ExtensionsConfig) {
		ext.ActiveTransient = appendUnique(ext.ActiveTransient, id)
	}); err != nil {
		writeGUIError(w, "extension activated, but couldn't save settings: "+err.Error(), 500)
		return
	}
	_ = s.rebuildAgent()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *server) extensionsEssential(w http.ResponseWriter, id string) {
	if id == "" {
		writeGUIError(w, "id required", 400)
		return
	}
	s.configTxMu.Lock()
	defer s.configTxMu.Unlock()
	if err := s.persistExtensionUpdate(func(ext *config.ExtensionsConfig) {
		ext.EssentialPlugins = extensions.MarkEssential(ext.EssentialPlugins, id)
		ext.ActiveTransient = appendUnique(ext.ActiveTransient, id)
	}); err != nil {
		writeGUIError(w, "couldn't save extension preference: "+err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (s *server) extensionsCleanup(w http.ResponseWriter) {
	if err := s.cleanupExtensionPool(); err != nil {
		writeGUIError(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// persistExtensionUpdate requires configTxMu for the surrounding operation.
// Clone slice storage before prospective mutations: removing a value in-place
// must not mutate the live config when persistence fails.
func (s *server) persistExtensionUpdate(mutate func(*config.ExtensionsConfig)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.cfg
	next.Extensions.AlwaysAllowTools = append([]string(nil), next.Extensions.AlwaysAllowTools...)
	next.Extensions.Dismissed = append([]string(nil), next.Extensions.Dismissed...)
	next.Extensions.InstalledSkills = append([]string(nil), next.Extensions.InstalledSkills...)
	next.Extensions.InstalledPlugins = append([]string(nil), next.Extensions.InstalledPlugins...)
	next.Extensions.EssentialPlugins = append([]string(nil), next.Extensions.EssentialPlugins...)
	next.Extensions.ActiveTransient = append([]string(nil), next.Extensions.ActiveTransient...)
	mutate(&next.Extensions)
	if err := s.persistConfig(next); err != nil {
		return err
	}
	s.cfg = next
	return nil
}

func dismissedSet(list []string) map[string]bool {
	m := map[string]bool{}
	for _, id := range list {
		m[id] = true
	}
	return m
}

func installedKeys(m map[string]bool) []string {
	var out []string
	for k, v := range m {
		if v {
			out = append(out, k)
		}
	}
	return out
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

func removeString(list []string, v string) []string {
	out := list[:0]
	for _, x := range list {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

func filepathBase(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}
