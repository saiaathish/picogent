package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/app"
	"github.com/saiaathish/picogent/internal/attachments"
	"github.com/saiaathish/picogent/internal/codexauth"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/ctxmgr"
	"github.com/saiaathish/picogent/internal/extensions"
	"github.com/saiaathish/picogent/internal/goal"
	"github.com/saiaathish/picogent/internal/learn"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/session"
	"github.com/saiaathish/picogent/internal/setup"
	"github.com/saiaathish/picogent/internal/slash"
	"github.com/saiaathish/picogent/internal/trace"
)

type event struct {
	Type    string  `json:"type"`
	Text    string  `json:"text,omitempty"`
	Summary string  `json:"summary,omitempty"`
	Hint    string  `json:"hint,omitempty"`
	Path    string  `json:"path,omitempty"`
	Line    int     `json:"line,omitempty"`
	LineEnd int     `json:"line_end,omitempty"`
	Added   int     `json:"added,omitempty"`
	Removed int     `json:"removed,omitempty"`
	Count   int     `json:"count,omitempty"`
	Kind    string  `json:"kind,omitempty"`
	Status  string  `json:"status,omitempty"`
	Tokens  int     `json:"tokens,omitempty"`
	Budget  int     `json:"budget,omitempty"`
	Pct     float64 `json:"pct,omitempty"`
	Level   string  `json:"level,omitempty"`
}

type transcriptLine struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type server struct {
	cfg         config.Config
	ag          *agent.Agent
	mu          sync.Mutex
	hist        []llm.Message
	sessionID   string
	permCh      chan perm.Decision
	subs        []chan event
	busy        bool
	cancel      context.CancelFunc
	steerMu     sync.Mutex
	steerPrompt string
	steerParts  []llm.Part
	undoStack   []extensions.UndoEntry
	pendingPerm perm.Request
	liveTask    agent.TaskMode
}

func Run() error {
	cfg, a, err := app.Load(".")
	if err != nil {
		return err
	}
	cfg.Extensions.InstalledSkills = extensions.LoadDeveloperExtensions(cfg.Extensions.InstalledSkills)
	_ = config.Save(cfg)
	if a != nil {
		a.SetTaskMode(agent.TaskAgent)
	}
	sessID, hist := initialSession(cfg.Workspace)
	s := &server{cfg: cfg, ag: a, permCh: make(chan perm.Decision, 1), sessionID: sessID, hist: hist}
	s.attachRouterHook()
	s.ensureProject()
	addr := "127.0.0.1:7420"
	if v := os.Getenv("PICOGENT_GUI_ADDR"); v != "" {
		addr = v
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return err
		}
	}
	path := "/"
	if config.NeedsSetup() {
		path = "/setup.html"
	}
	url := "http://" + ln.Addr().String() + path
	fmt.Println("picogent gui", url)
	if os.Getenv("PICOGENT_NO_BROWSER") == "" {
		go openBrowser(url)
	}
	return http.Serve(ln, s.Handler())
}

func RunSetup() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.SetupComplete = false
	_ = config.Save(cfg)
	return Run()
}

func (s *server) Handler() http.Handler {
	static, err := fs.Sub(webFS, "web")
	if err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/state", s.state)
	mux.HandleFunc("/api/setup", s.setupStatus)
	mux.HandleFunc("/api/setup/install", s.setupInstall)
	mux.HandleFunc("/api/setup/login", s.setupLogin)
	mux.HandleFunc("/api/setup/finish", s.setupFinish)
	mux.HandleFunc("/api/chat", s.chat)
	mux.HandleFunc("/api/permission", s.permission)
	mux.HandleFunc("/api/mode", s.setMode)
	mux.HandleFunc("/api/task-mode", s.setTaskMode)
	mux.HandleFunc("/api/cancel", s.cancelChat)
	mux.HandleFunc("/api/reset", s.reset)
	mux.HandleFunc("/api/sessions", s.sessions)
	mux.HandleFunc("/api/file", s.readFile)
	mux.HandleFunc("/api/settings", s.settings)
	mux.HandleFunc("/api/router", s.routerAPI)
	mux.HandleFunc("/api/projects", s.projectsAPI)
	mux.HandleFunc("/api/folder/pick", s.folderPickAPI)
	mux.HandleFunc("/api/files/pick", s.filesPickAPI)
	mux.HandleFunc("/api/files/read", s.filesReadAPI)
	mux.HandleFunc("/api/overview", s.overviewAPI)
	mux.HandleFunc("/api/test", s.testAPI)
	mux.HandleFunc("/api/diff", s.diffAPI)
	mux.HandleFunc("/api/extensions", s.extensionsAPI)
	mux.HandleFunc("/api/trace", s.traceAPI)
	mux.HandleFunc("/api/events", s.events)
	mux.Handle("/", http.FileServer(http.FS(static)))
	return mux
}

func (s *server) traceAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", 405)
		return
	}
	var events []trace.Event
	if s.ag != nil && s.ag.Trace != nil {
		events = s.ag.Trace.Tail(80)
	}
	if events == nil {
		events = []trace.Event{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"events": events})
}

func (s *server) attachRouterHook() {
	if s.ag == nil {
		return
	}
	r, ok := s.ag.LLM.(*llm.Router)
	if !ok {
		return
	}
	prev := r.OnRoute
	r.OnRoute = func(dec llm.RouteDecision) {
		if prev != nil {
			prev(dec)
		}
		s.mu.Lock()
		s.cfg.Router.LastTier = string(dec.Tier)
		s.cfg.Router.LastModel = dec.Model
		s.cfg.Router.LastReason = dec.Reason
		s.cfg.Router.LastReasoning = string(dec.Reasoning)
		s.cfg.Router.LastTaskKind = string(dec.TaskKind)
		s.cfg.Router.LastRouteMode = string(dec.RouteMode)
		s.mu.Unlock()
		s.emit(event{Type: "route", Text: dec.Label, Summary: dec.Reason})
	}
}

func (s *server) emit(e event) {
	s.mu.Lock()
	subs := append([]chan event(nil), s.subs...)
	s.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- e:
		case <-time.After(2 * time.Second):
		}
	}
}

func initialSession(workspace string) (id string, hist []llm.Message) {
	if prev, err := session.Latest(workspace); err == nil {
		return prev.ID, prev.Messages
	}
	return session.New(workspace).ID, nil
}

func (s *server) snapshot() map[string]any {
	s.mu.Lock()
	cfg := s.cfg
	ag := s.ag
	busy := s.busy
	sessionID := s.sessionID
	hist := s.hist
	pend := s.pendingPerm
	liveTask := s.liveTask
	s.mu.Unlock()

	hint := ""
	if err := cfg.MissingAuth(); err != nil {
		hint = err.Error()
	}
	taskMode := liveTask
	if !taskMode.Valid() && ag != nil && ag.TaskMode.Valid() {
		taskMode = ag.TaskMode
	}
	if !taskMode.Valid() {
		taskMode = agent.ParseTaskMode(cfg.TaskMode)
	}
	out := map[string]any{
		"mode":          cfg.Mode,
		"task_mode":     string(taskMode),
		"model":         cfg.DisplayModel(),
		"workspace":     cfg.Workspace,
		"provider":      cfg.Provider,
		"codex":         cfg.Provider == config.ProviderCodex && codexauth.LoggedIn(),
		"quadcode":      cfg.Provider == config.ProviderQuadCode && cfg.AnthropicKeyResolved() != "",
		"busy":          busy,
		"hint":          hint,
		"setup":         !cfg.SetupComplete,
		"mcp_tools":     mcpToolCount(ag),
		"session_id":    sessionID,
		"router":        s.routerSnapshot(),
		"model_options": llm.ModelChoices(llm.Ecosystem(cfg.RouterEcosystem()), cfg.FableAllowed()),
		"slash":         slash.Catalog(cfg.Workspace),
	}
	if store, err := learn.Load(cfg.Workspace); err == nil {
		out["overview"] = store
	}
	if g, err := goal.Load(cfg.Workspace); err == nil && g != "" {
		out["goal"] = g
	}
	if len(hist) > 0 {
		out["messages"] = messagesToTranscript(hist)
	}
	if pend.Tool != "" {
		kind := pend.Tool
		if strings.HasPrefix(kind, "mcp_") || kind == "mcp_manage" {
			kind = "mcp"
		}
		out["pending_perm"] = map[string]any{
			"tool":    pend.Tool,
			"summary": pend.Summary,
			"hint":    pend.Hint,
			"kind":    kind,
			"status":  permStatus(pend),
		}
	}
	budget := ctxmgr.BudgetForModel(cfg.Model)
	ctxStats := ctxmgr.StatsFor(hist, budget)
	out["context"] = ctxStats
	return out
}

func (s *server) routerSnapshot() map[string]any {
	s.mu.Lock()
	cfg := s.cfg
	ag := s.ag
	s.mu.Unlock()

	eco := cfg.RouterEcosystem()
	cat := llm.CatalogSnapshot()
	tiers := make([]map[string]any, 0)
	for _, m := range cat.ForEcosystem(llm.Ecosystem(eco)) {
		entry := map[string]any{
			"id":          m.ID,
			"display":     m.Display,
			"tier":        m.Tier,
			"description": m.Description,
			"gated":       m.Gated,
		}
		if m.Reasoning != nil {
			entry["reasoning"] = map[string]any{
				"supported": m.Reasoning.Supported,
				"default":   m.Reasoning.Default,
			}
		}
		tiers = append(tiers, entry)
	}
	last := map[string]any{
		"tier":       cfg.Router.LastTier,
		"model":      cfg.Router.LastModel,
		"reason":     cfg.Router.LastReason,
		"reasoning":  cfg.Router.LastReasoning,
		"task_kind":  cfg.Router.LastTaskKind,
		"route_mode": cfg.Router.LastRouteMode,
	}
	if ag != nil {
		if r, ok := ag.LLM.(*llm.Router); ok {
			dec := r.LastDecision()
			if dec.Model != "" {
				last = map[string]any{
					"tier":       dec.Tier,
					"label":      dec.Label,
					"model":      dec.Model,
					"reason":     dec.Reason,
					"score":      dec.Score,
					"advisor":    dec.Advisor,
					"reasoning":  dec.Reasoning,
					"task_kind":  dec.TaskKind,
					"route_mode": dec.RouteMode,
				}
			}
		}
	}
	reasoningScale := llm.ReasoningScaleFor(llm.Ecosystem(eco))
	return map[string]any{
		"enabled":         cfg.Router.Enabled,
		"ecosystem":       eco,
		"use_llm_advisor": cfg.Router.UseLLMAdvisor,
		"allow_fable":     cfg.Router.AllowFable,
		"fable_confirmed": cfg.Router.FableConfirmed,
		"fable_available": cfg.AnthropicKeyResolved() != "",
		"catalog_version": cat.Version,
		"catalog_updated": cat.Updated,
		"tiers":           tiers,
		"reasoning_scale": reasoningScale,
		"last":            last,
	}
}

func (s *server) routerAPI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.routerSnapshot())
	case http.MethodPost:
		var in struct {
			Refresh bool `json:"refresh"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.Refresh {
			llm.InitCatalog(true)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.routerSnapshot())
	default:
		http.Error(w, "GET or POST only", 405)
	}
}

func mcpToolCount(a *agent.Agent) int {
	if a == nil || a.Tools == nil || a.Tools.MCP == nil {
		return 0
	}
	return len(a.Tools.MCP.Tools())
}

func (s *server) setupStatus(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(setup.Snapshot(cfg))
}

func (s *server) setupInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	log, err := setup.InstallCores()
	w.Header().Set("Content-Type", "application/json")
	out := map[string]any{"log": log, "ok": err == nil}
	if err != nil {
		out["error"] = err.Error()
		w.WriteHeader(500)
	}
	s.mu.Lock()
	st := setup.Snapshot(s.cfg)
	s.mu.Unlock()
	out["status"] = st
	_ = json.NewEncoder(w).Encode(out)
}

func (s *server) setupLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	var in struct {
		Target   string `json:"target"`
		ReturnTo string `json:"return_to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch strings.ToLower(in.Target) {
	case "claude":
		if err := setup.StartClaudeLogin(); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "hint": "Finish login in the window that opened, then come back here."})
	default:
		if in.ReturnTo == "" {
			in.ReturnTo = "http://127.0.0.1:7420/setup.html"
		}
		authURL, err := codexauth.BeginBrowserLogin(in.ReturnTo)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "url": authURL})
	}
}

func (s *server) setupFinish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	var in struct {
		Workspace string `json:"workspace"`
		Mode      string `json:"mode"`
		Model     string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	next, err := setup.Apply(cfg, in.Workspace, in.Mode, in.Model)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	a, err := app.Build(next)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	s.mu.Lock()
	s.cfg = next
	s.ag = a
	s.hist = nil
	s.mu.Unlock()
	s.attachRouterHook()
	w.WriteHeader(204)
}

func (s *server) state(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.snapshot())
}

func (s *server) setMode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	m := config.Mode(in.Mode)
	if !m.Valid() {
		http.Error(w, "invalid mode", 400)
		return
	}
	s.mu.Lock()
	s.cfg.Mode = m
	if s.ag != nil {
		s.ag.CFG.Mode = m
		if s.ag.Gate != nil {
			s.ag.Gate.Mode = m
		}
	}
	_ = config.Save(s.cfg)
	s.mu.Unlock()
	w.WriteHeader(204)
}

func (s *server) setTaskMode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TaskMode string `json:"task_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	m := agent.ParseTaskMode(in.TaskMode)
	if !m.Valid() {
		http.Error(w, "invalid task mode", 400)
		return
	}
	s.mu.Lock()
	s.cfg.TaskMode = string(m)
	s.liveTask = m
	if s.ag != nil {
		s.ag.SetTaskMode(m)
	}
	_ = config.Save(s.cfg)
	s.mu.Unlock()
	s.emit(event{Type: "system", Text: "mode: " + strings.ToLower(m.Label())})
	s.emit(event{Type: "task_mode", Text: string(m)})
	w.WriteHeader(204)
}

func (s *server) applyTaskMode(payload string) {
	m := agent.ParseTaskMode(strings.TrimPrefix(payload, "task:"))
	s.mu.Lock()
	s.cfg.TaskMode = string(m)
	s.liveTask = m
	if s.ag != nil {
		s.ag.SetTaskMode(m)
	}
	_ = config.Save(s.cfg)
	s.mu.Unlock()
	s.emit(event{Type: "system", Text: "mode: " + strings.ToLower(m.Label())})
	s.emit(event{Type: "task_mode", Text: string(m)})
}

func (s *server) autoApplyFromUserPrompt(prompt string) {
	if !s.cfg.AutoTaskModeOn() {
		return
	}
	if strings.HasPrefix(strings.TrimSpace(prompt), "/") {
		return
	}
	s.mu.Lock()
	ag := s.ag
	cur := s.liveTask
	goalText := ""
	if !cur.Valid() && ag != nil {
		cur = ag.TaskMode
	}
	if ag != nil {
		goalText = ag.Goal
	}
	s.mu.Unlock()
	if ag == nil {
		return
	}

	dec := agent.InferAuto(prompt, cur, goalText)
	if dec.GoalSet && dec.Goal != goalText {
		_ = s.setGoal(dec.Goal)
	}
	s.mu.Lock()
	s.liveTask = dec.TaskMode
	ag.SetTaskMode(dec.TaskMode)
	s.mu.Unlock()
	if dec.TaskMode != cur {
		s.emit(event{Type: "task_mode", Text: string(dec.TaskMode)})
	}
}

func (s *server) queueSteer(prompt string, parts []llm.Part) {
	s.steerMu.Lock()
	s.steerPrompt = prompt
	s.steerParts = parts
	s.steerMu.Unlock()
}

func (s *server) popSteer() (string, []llm.Part, bool) {
	s.steerMu.Lock()
	defer s.steerMu.Unlock()
	if s.steerPrompt == "" {
		return "", nil, false
	}
	p := s.steerPrompt
	parts := s.steerParts
	s.steerPrompt = ""
	s.steerParts = nil
	return p, parts, true
}

func (s *server) startAgentTurn(prompt string, parts []llm.Part) {
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		return
	}
	s.busy = true
	hist := s.hist
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.mu.Unlock()

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				s.emit(event{Type: "error", Text: fmt.Sprintf("agent panic: %v", rec)})
				fmt.Fprintf(os.Stderr, "picogent: agent panic: %v\n", rec)
			}
			s.mu.Lock()
			s.busy = false
			s.cancel = nil
			s.mu.Unlock()
			s.emit(event{Type: "done"})
			if p, pts, ok := s.popSteer(); ok {
				s.startAgentTurn(p, pts)
			}
		}()
		if s.ag == nil {
			s.emit(event{Type: "error", Text: "agent not ready"})
			return
		}
		if s.ag.Trace == nil {
			if log, err := trace.Open(s.cfg.Workspace); err == nil {
				s.ag.Trace = log
			}
		}
		h := newGUIHandler(s)
		h.beginTurn(prompt)
		s.maybeRecommendExtensions(prompt)
		userMsg := llm.Message{Role: "user", Content: prompt, Parts: parts}
		next, result, err := s.ag.Run(ctx, hist, userMsg, h)
		for i := len(next) - 1; i >= 0; i-- {
			if next[i].Role == "user" && len(next[i].Parts) > 0 {
				next[i].Content = attachments.SummaryLine(next[i].Parts) + next[i].Content
				next[i].Parts = nil
				break
			}
		}
		h.endTurn(result)
		if result.GoalDone {
			_ = s.clearGoal()
		}
		s.mu.Lock()
		if s.liveTask.Valid() && s.ag != nil {
			s.ag.SetTaskMode(s.liveTask)
		}
		s.hist = next
		ws := s.cfg.Workspace
		sid := s.sessionID
		llmClient := s.ag.LLM
		model := s.cfg.Model
		_ = config.Save(s.cfg)
		_ = session.SaveMessages(ws, sid, next)
		s.mu.Unlock()
		if result.Context.Tokens > 0 {
			s.emit(contextEvent(result.Context))
		}
		s.maybeAutoTitle(context.Background(), llmClient, model, ws, sid, next)
		if err != nil && ctx.Err() == nil {
			fmt.Fprintf(os.Stderr, "picogent: turn error: %v\n", err)
			s.emit(event{Type: "error", Text: err.Error()})
		}
	}()
}

func (s *server) setGoal(text string) error {
	text = strings.TrimSpace(text)
	if err := goal.Set(s.cfg.Workspace, text); err != nil {
		return err
	}
	s.mu.Lock()
	if s.ag != nil {
		s.ag.Goal = text
	}
	s.mu.Unlock()
	s.emit(event{Type: "goal", Text: text})
	return nil
}

func (s *server) clearGoal() error {
	if err := goal.Clear(s.cfg.Workspace); err != nil {
		return err
	}
	s.mu.Lock()
	if s.ag != nil {
		s.ag.Goal = ""
	}
	s.mu.Unlock()
	s.emit(event{Type: "goal", Text: ""})
	return nil
}

func (s *server) reset(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	if len(s.hist) > 0 {
		_ = session.SaveMessages(s.cfg.Workspace, s.sessionID, s.hist)
	}
	s.sessionID = session.New(s.cfg.Workspace).ID
	s.hist = nil
	id := s.sessionID
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
}

func (s *server) permission(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Allow  bool `json:"allow"`
		Turn   bool `json:"turn"`
		Always bool `json:"always"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	d := perm.Deny
	if in.Always {
		d = perm.AllowAlways
	} else if in.Turn {
		d = perm.AllowTurn
	} else if in.Allow {
		d = perm.Allow
	}
	select {
	case s.permCh <- d:
	case <-time.After(2 * time.Second):
	}
	s.mu.Lock()
	tool := s.pendingPerm.Tool
	s.pendingPerm = perm.Request{}
	if d == perm.AllowAlways && s.ag != nil && s.ag.Gate != nil && tool != "" {
		s.cfg.Extensions.AlwaysAllowTools = appendUnique(s.cfg.Extensions.AlwaysAllowTools, tool)
		s.ag.Gate.SetAlwaysAllowed(s.cfg.Extensions.AlwaysAllowTools)
		_ = config.Save(s.cfg)
	}
	s.mu.Unlock()
	w.WriteHeader(204)
}

func (s *server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "no flush", 500)
		return
	}
	ch := make(chan event, 32)
	s.mu.Lock()
	s.subs = append(s.subs, ch)
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		for i, c := range s.subs {
			if c == ch {
				s.subs = append(s.subs[:i], s.subs[i+1:]...)
				break
			}
		}
		s.mu.Unlock()
	}()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	fmt.Fprintf(w, "data: {\"type\":\"hello\"}\n\n")
	fl.Flush()
	s.mu.Lock()
	pend := s.pendingPerm
	s.mu.Unlock()
	if pend.Tool != "" {
		b, _ := json.Marshal(permissionEvent(pend))
		fmt.Fprintf(w, "data: %s\n\n", b)
		fl.Flush()
	}
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			fmt.Fprintf(w, ": ping\n\n")
			fl.Flush()
		case e := <-ch:
			b, _ := json.Marshal(e)
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		}
	}
}

func (s *server) cancelChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	w.WriteHeader(204)
}

func (s *server) chat(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Prompt      string              `json:"prompt"`
		Attachments []attachments.Input `json:"attachments"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	prompt := strings.TrimSpace(in.Prompt)
	parts, err := attachments.Decode(in.Attachments)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if prompt == "" && len(parts) == 0 {
		http.Error(w, "empty prompt", 400)
		return
	}

	userPrompt := prompt
	kind, payload := slash.Resolve(s.cfg.Workspace, prompt)
	runAgent := kind != slash.Local
	if kind == slash.Local && strings.HasPrefix(payload, "goal:set:") {
		text := strings.TrimPrefix(payload, "goal:set:")
		if err := s.setGoal(text); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		s.emit(event{Type: "system", Text: "goal set — working…"})
		prompt = goal.WorkPrompt(text)
		runAgent = true
	} else if kind == slash.Local {
		switch payload {
		case "clear":
			s.mu.Lock()
			s.hist = nil
			s.mu.Unlock()
			s.emit(event{Type: "system", Text: "cleared"})
		case "compact":
			s.mu.Lock()
			budget := ctxmgr.BudgetForModel(s.cfg.Model)
			compact, stats, err := ctxmgr.Manage(context.Background(), s.ag.LLM, s.cfg.Model, s.hist, budget)
			if err == nil {
				s.hist = compact
			}
			s.mu.Unlock()
			msg := "context compacted"
			if stats.Compacted && stats.Method != "" {
				msg = fmt.Sprintf("context compacted (%s · %d/%dk tokens)", stats.Method, stats.Tokens/1000, stats.Budget/1000)
			}
			s.emit(event{Type: "system", Text: msg})
			s.emit(contextEvent(stats))
		case "status":
			st := fmt.Sprintf("safe/fast=%s task=%s model=%s workspace=%s", s.cfg.Mode, s.ag.TaskMode.Label(), s.cfg.Model, s.cfg.Workspace)
			if s.ag != nil && s.ag.Goal != "" {
				st += " · goal: " + s.ag.Goal
			}
			if s.ag.Tools != nil && s.ag.Tools.HasMCP() {
				st += fmt.Sprintf(" · %d MCP tools", len(s.ag.Tools.MCP.Tools()))
			}
			s.emit(event{Type: "system", Text: st})
		case "diff":
			s.emit(event{Type: "system", Text: slash.GitDiff()})
		case "goal:show":
			g, _ := goal.Load(s.cfg.Workspace)
			if g == "" {
				s.emit(event{Type: "system", Text: "no active goal"})
			} else {
				s.emit(event{Type: "system", Text: "goal: " + g})
			}
		case "goal:clear":
			if err := s.clearGoal(); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			s.emit(event{Type: "system", Text: "goal cleared"})
		default:
			if strings.HasPrefix(payload, "task:") {
				s.applyTaskMode(payload)
			} else if strings.HasPrefix(payload, "memory:") {
				text := strings.TrimPrefix(payload, "memory:")
				if text == "" {
					text = "(no project rules files)"
				}
				s.emit(event{Type: "system", Text: text})
			}
		}
		if !runAgent {
			w.WriteHeader(204)
			return
		}
	}
	if kind == slash.Prompt {
		prompt = payload
	}

	s.autoApplyFromUserPrompt(userPrompt)

	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		s.queueSteer(prompt, parts)
		s.emit(event{Type: "system", Text: "Follow-up queued for when the current turn finishes"})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "queued": true})
		return
	}
	s.mu.Unlock()

	s.startAgentTurn(prompt, parts)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(202)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

type guiHandler struct {
	s        *server
	learn    learn.Store
	explore  int
	searches int
	reads    int
	edits    int
	added    int
	removed  int
	changes  []event
}

func newGUIHandler(s *server) *guiHandler {
	s.mu.Lock()
	ws := s.cfg.Workspace
	s.mu.Unlock()
	store, _ := learn.Load(ws)
	return &guiHandler{s: s, learn: store}
}

func (h *guiHandler) beginTurn(prompt string) {
	h.learn.RecordTurn()
	h.s.emit(event{Type: "think", Text: summarizePrompt(prompt), Kind: "plan", Status: "start"})
	h.s.emit(event{Type: "activity", Kind: "reset"})
}

func (h *guiHandler) endTurn(result agent.Result) {
	if result.Context.Compacted && result.Context.Method != "" {
		h.s.emit(event{
			Type:   "context",
			Text:   fmt.Sprintf("Auto-compacted context (%s)", result.Context.Method),
			Tokens: result.Context.Tokens,
			Budget: result.Context.Budget,
			Pct:    result.Context.Pct,
			Level:  result.Context.Level,
			Status: result.Context.Method,
		})
	}
	if len(result.FilesChanged) > 0 || h.edits > 0 {
		label := fmt.Sprintf("Edited %d files", h.edits)
		if h.edits == 1 {
			label = "Edited 1 file"
		}
		h.s.emit(event{
			Type:    "changes_summary",
			Text:    label,
			Count:   h.edits,
			Added:   h.added,
			Removed: h.removed,
			Status:  "done",
		})
	}
	h.s.emit(event{Type: "think", Text: "Done", Kind: "plan", Status: "done"})
	if result.Verified != "" {
		failed := strings.Contains(strings.ToLower(result.Verified), "fail")
		h.s.emit(event{
			Type:    "test",
			Text:    result.Verified,
			Summary: clip(result.Verified, 2000),
			Status:  ternary(failed, "fail", "done"),
			Kind:    "test",
		})
	}
	_ = learn.Save(h.learn)
	h.s.emit(event{Type: "overview", Text: "refresh"})
	h.s.cleanupExtensionPool()
}

func summarizePrompt(prompt string) string {
	p := strings.TrimSpace(prompt)
	if len(p) > 120 {
		p = p[:117] + "…"
	}
	if p == "" {
		return "Working on your request…"
	}
	return p
}

func (h *guiHandler) OnText(text string) {
	h.s.emit(event{Type: "assistant", Text: text})
}
func (h *guiHandler) OnToolStart(call llm.ToolCall) {
	h.learn.RecordTool(call.Name)

	switch call.Name {
	case "read_file":
		path := parseToolPath(call.Arguments)
		if path != "" {
			h.reads++
			h.learn.RecordRead(path)
			h.s.emit(event{Type: "review", Path: path})
			h.s.emit(event{Type: "activity", Kind: "read", Count: h.reads, Path: path})
		}
	case "glob", "grep":
		h.searches++
		h.learn.RecordSearch()
		h.s.emit(event{Type: "activity", Kind: "search", Count: h.searches})
	case "write_file", "edit_file":
		path := parseToolPath(call.Arguments)
		h.s.emit(event{Type: "think", Text: "Editing " + path, Kind: "edit", Status: "start", Path: path})
	case "bash":
		var in struct {
			Command string `json:"command"`
		}
		_ = json.Unmarshal([]byte(call.Arguments), &in)
		if isTestCommand(in.Command) {
			h.s.emit(event{Type: "think", Text: "Running tests…", Kind: "test", Status: "start"})
		}
	}
}
func (h *guiHandler) OnToolEnd(call llm.ToolCall, result string, err error) {
	if err != nil {
		h.s.emit(event{Type: "error", Text: err.Error()})
		return
	}

	h.s.mu.Lock()
	ws := h.s.cfg.Workspace
	h.s.mu.Unlock()

	switch call.Name {
	case "write_file":
		path, content := parseWriteContent(call.Arguments)
		added := strings.Count(content, "\n") + 1
		if content == "" {
			added = 0
		}
		removed := 0
		if gitAdded, gitRemoved := diffStats(ws, path); gitAdded > 0 || gitRemoved > 0 {
			added, removed = gitAdded, gitRemoved
		}
		h.recordChange(path, added, removed)
	case "edit_file":
		path, oldStr, newStr := parseEditArgs(call.Arguments)
		added, removed := lineDelta(oldStr, newStr)
		if gitAdded, gitRemoved := diffStats(ws, path); gitAdded+gitRemoved > added+removed {
			added, removed = gitAdded, gitRemoved
		}
		h.recordChange(path, added, removed)
	case "bash":
		var in struct {
			Command string `json:"command"`
		}
		_ = json.Unmarshal([]byte(call.Arguments), &in)
		if isTestCommand(in.Command) {
			passed, failed, skipped := parseTestOutput(result)
			h.learn.RecordTest(passed, failed, skipped, result)
			h.s.emit(event{
				Type:    "test",
				Text:    formatTestSummary(passed, failed, skipped),
				Summary: clip(result, 2000),
				Count:   passed,
				Added:   failed,
				Removed: skipped,
				Status:  ternary(failed > 0, "fail", "done"),
				Kind:    "test",
			})
		}
	}
}

func (h *guiHandler) recordChange(path string, added, removed int) {
	if path == "" {
		return
	}
	h.edits++
	h.added += added
	h.removed += removed
	h.learn.RecordChange(path, added, removed)
	ev := event{Type: "change", Path: path, Added: added, Removed: removed, Status: "done"}
	h.changes = append(h.changes, ev)
	h.s.emit(ev)
	h.s.emit(event{Type: "activity", Kind: "edit", Count: h.edits, Added: h.added, Removed: h.removed})
}

func ternary(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

func (h *guiHandler) OnNeedPermission(ctx context.Context, req perm.Request) (perm.Decision, error) {
	h.s.mu.Lock()
	h.s.pendingPerm = req
	h.s.mu.Unlock()
	if h.s.ag != nil {
		_ = h.s.ag.Trace.Append("perm", req.Tool, req.Summary, nil, 0)
	}
	h.s.emit(permissionEvent(req))
	select {
	case <-ctx.Done():
		return perm.Deny, ctx.Err()
	case d := <-h.s.permCh:
		return d, nil
	}
}

func permissionEvent(req perm.Request) event {
	kind := req.Tool
	if strings.HasPrefix(kind, "mcp_") || kind == "mcp_manage" {
		kind = "mcp"
	}
	return event{Type: "permission", Summary: req.Summary, Hint: req.Hint, Text: req.Tool, Kind: kind, Status: permStatus(req)}
}

func permStatus(req perm.Request) string {
	if req.Destructive {
		return "destructive"
	}
	if req.OutsideWorkspace {
		return "outside"
	}
	if req.Tool == "bash" {
		return "terminal"
	}
	return "risky"
}
func (h *guiHandler) OnError(err error) {
	h.s.emit(event{Type: "error", Text: err.Error()})
}

func contextEvent(st ctxmgr.Stats) event {
	return event{
		Type:   "context",
		Tokens: st.Tokens,
		Budget: st.Budget,
		Pct:    st.Pct,
		Level:  st.Level,
		Status: st.Method,
	}
}

func (s *server) maybeAutoTitle(ctx context.Context, client llm.Client, model, workspace, id string, msgs []llm.Message) {
	if client == nil || id == "" {
		return
	}
	prev, err := session.Load(id)
	if err != nil || !session.NeedsAutoTitle(prev) {
		return
	}
	title, err := session.GenerateTitle(ctx, client, model, msgs)
	if err != nil || title == "" {
		return
	}
	if err := session.SetTitle(id, title); err != nil {
		return
	}
	s.emit(event{Type: "title", Text: title})
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func messagesToTranscript(msgs []llm.Message) []transcriptLine {
	var out []transcriptLine
	for _, m := range msgs {
		switch m.Role {
		case "user":
			if t := strings.TrimSpace(m.Content); t != "" {
				out = append(out, transcriptLine{Role: "user", Text: t})
			}
		case "assistant":
			if t := strings.TrimSpace(m.Content); t != "" {
				out = append(out, transcriptLine{Role: "assistant", Text: t})
			}
		case "tool":
			if t := strings.TrimSpace(m.Content); t != "" {
				out = append(out, transcriptLine{Role: "tool", Text: clip(t, 400)})
			}
		}
	}
	return out
}

func (s *server) sessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.Lock()
		ws := s.cfg.Workspace
		cur := s.sessionID
		s.mu.Unlock()
		metas, err := session.ListMeta(ws)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sessions":   metas,
			"current_id": cur,
		})
	case http.MethodPost:
		var in struct {
			Action string `json:"action"`
			ID     string `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		switch in.Action {
		case "new":
			s.mu.Lock()
			if len(s.hist) > 0 {
				_ = session.SaveMessages(s.cfg.Workspace, s.sessionID, s.hist)
			}
			s.sessionID = session.New(s.cfg.Workspace).ID
			s.hist = nil
			s.liveTask = agent.TaskAgent
			if s.ag != nil {
				s.ag.SetTaskMode(agent.TaskAgent)
			}
			id := s.sessionID
			s.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
		case "load":
			if in.ID == "" {
				http.Error(w, "id required", 400)
				return
			}
			sess, err := session.Load(in.ID)
			if err != nil {
				http.Error(w, err.Error(), 404)
				return
			}
			s.mu.Lock()
			s.sessionID = sess.ID
			s.hist = sess.Messages
			s.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       sess.ID,
				"title":    sess.Title,
				"messages": messagesToTranscript(sess.Messages),
			})
		case "delete":
			if in.ID == "" {
				http.Error(w, "id required", 400)
				return
			}
			s.mu.Lock()
			if s.sessionID == in.ID {
				s.sessionID = session.New(s.cfg.Workspace).ID
				s.hist = nil
			}
			s.mu.Unlock()
			if err := session.Delete(in.ID); err != nil && !os.IsNotExist(err) {
				http.Error(w, err.Error(), 500)
				return
			}
			w.WriteHeader(204)
		default:
			http.Error(w, "action must be new, load, or delete", 400)
		}
	default:
		http.Error(w, "GET or POST only", 405)
	}
}

func (s *server) readFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", 405)
		return
	}
	rel := strings.TrimSpace(r.URL.Query().Get("path"))
	if rel == "" {
		http.Error(w, "path required", 400)
		return
	}
	s.mu.Lock()
	ws := s.cfg.Workspace
	s.mu.Unlock()
	wsAbs, err := filepath.Abs(ws)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	abs := filepath.Join(wsAbs, rel)
	abs, err = filepath.Abs(abs)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if abs != wsAbs && !strings.HasPrefix(abs, wsAbs+string(os.PathSeparator)) {
		http.Error(w, "outside workspace", 403)
		return
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	if !utf8.Valid(data) {
		http.Error(w, "not a utf-8 text file", 415)
		return
	}
	const maxLines = 800
	lines := strings.Split(string(data), "\n")
	total := len(lines)
	if total > maxLines {
		lines = lines[:maxLines]
	}
	type lineRow struct {
		N int    `json:"n"`
		T string `json:"t"`
	}
	rows := make([]lineRow, len(lines))
	for i, t := range lines {
		rows[i] = lineRow{N: i + 1, T: t}
	}
	display, _ := filepath.Rel(wsAbs, abs)
	if display == "." {
		display = rel
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"path":      display,
		"lines":     rows,
		"total":     total,
		"truncated": total > maxLines,
	})
}

func (s *server) settings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.Lock()
		cfg := s.cfg
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"workspace":              cfg.Workspace,
			"mode":                   cfg.Mode,
			"model":                  cfg.DisplayModel(),
			"provider":               cfg.Provider,
			"max_tool_rounds":        cfg.MaxToolRounds,
			"llm_timeout_sec":        cfg.LLMTimeoutSec,
			"bash_timeout_sec":       cfg.BashTimeoutSec,
			"has_api_key":            cfg.APIKeyResolved() != "",
			"has_anthropic_key":      cfg.AnthropicKeyResolved() != "",
			"codex":                  cfg.Provider == config.ProviderCodex && codexauth.LoggedIn(),
			"model_options":          llm.ModelChoices(llm.Ecosystem(cfg.RouterEcosystem()), cfg.FableAllowed()),
			"model_options_codex":    llm.ModelChoices(llm.EcoCodex, false),
			"model_options_quadcode": llm.ModelChoices(llm.EcoQuadCode, cfg.FableAllowed()),
			"router":                 s.routerSnapshot(),
		})
	case http.MethodPost:
		var in struct {
			Workspace      string `json:"workspace"`
			Mode           string `json:"mode"`
			Model          string `json:"model"`
			Provider       string `json:"provider"`
			AnthropicKey   string `json:"anthropic_api_key"`
			MaxToolRounds  int    `json:"max_tool_rounds"`
			LLMTimeoutSec  int    `json:"llm_timeout_sec"`
			BashTimeoutSec int    `json:"bash_timeout_sec"`
			RouterEnabled  *bool  `json:"router_enabled"`
			UseLLMAdvisor  *bool  `json:"use_llm_advisor"`
			AllowFable     *bool  `json:"allow_fable"`
			FableConfirmed *bool  `json:"fable_confirmed"`
			RefreshCatalog bool   `json:"refresh_catalog"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		s.mu.Lock()
		if in.Workspace != "" {
			s.cfg.Workspace = in.Workspace
		}
		if mode := config.Mode(in.Mode); mode.Valid() {
			s.cfg.Mode = mode
			s.ag.CFG.Mode = mode
			s.ag.Gate.Mode = mode
		}
		if in.Model != "" {
			if in.Model == config.ModelAuto {
				s.cfg.Model = config.ModelAuto
				s.cfg.Router.Enabled = true
			} else {
				s.cfg.Model = in.Model
				s.cfg.Router.Enabled = false
			}
			s.ag.CFG.Model = s.cfg.Model
			s.ag.CFG.Router.Enabled = s.cfg.Router.Enabled
		}
		if p := config.Provider(strings.ToLower(in.Provider)); in.Provider != "" {
			switch p {
			case config.ProviderCodex, config.ProviderQuadCode, config.ProviderOpenAI, config.ProviderOllama:
				s.cfg.Provider = p
				s.ag.CFG.Provider = p
			}
		}
		if in.AnthropicKey != "" {
			s.cfg.AnthropicKey = in.AnthropicKey
			s.ag.CFG.AnthropicKey = in.AnthropicKey
		}
		if in.RouterEnabled != nil {
			s.cfg.Router.Enabled = *in.RouterEnabled
			s.ag.CFG.Router.Enabled = *in.RouterEnabled
		}
		if in.UseLLMAdvisor != nil {
			s.cfg.Router.UseLLMAdvisor = *in.UseLLMAdvisor
			s.ag.CFG.Router.UseLLMAdvisor = *in.UseLLMAdvisor
		}
		if in.AllowFable != nil {
			s.cfg.Router.AllowFable = *in.AllowFable
			s.ag.CFG.Router.AllowFable = *in.AllowFable
		}
		if in.FableConfirmed != nil {
			s.cfg.Router.FableConfirmed = *in.FableConfirmed
			s.ag.CFG.Router.FableConfirmed = *in.FableConfirmed
		}
		if in.RefreshCatalog {
			llm.InitCatalog(true)
		}
		if in.MaxToolRounds > 0 {
			s.cfg.MaxToolRounds = in.MaxToolRounds
			s.ag.CFG.MaxToolRounds = in.MaxToolRounds
		}
		if in.LLMTimeoutSec > 0 {
			s.cfg.LLMTimeoutSec = in.LLMTimeoutSec
		}
		if in.BashTimeoutSec > 0 {
			s.cfg.BashTimeoutSec = in.BashTimeoutSec
		}
		cfg := s.cfg
		s.mu.Unlock()
		if err := config.Save(cfg); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if in.Provider != "" || in.RouterEnabled != nil || in.UseLLMAdvisor != nil || in.AllowFable != nil || in.FableConfirmed != nil || in.AnthropicKey != "" {
			a, err := app.Build(cfg)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			s.mu.Lock()
			s.cfg = cfg
			s.ag = a
			s.mu.Unlock()
			s.attachRouterHook()
		}
		w.WriteHeader(204)
	default:
		http.Error(w, "GET or POST only", 405)
	}
}

func openBrowser(url string) {
	time.Sleep(200 * time.Millisecond)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}
	_ = cmd.Start()
}
