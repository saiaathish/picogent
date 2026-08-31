package gui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/agyauth"
	"github.com/saiaathish/picogent/internal/app"
	"github.com/saiaathish/picogent/internal/attachments"
	"github.com/saiaathish/picogent/internal/claudeauth"
	"github.com/saiaathish/picogent/internal/codexauth"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/ctxmgr"
	"github.com/saiaathish/picogent/internal/evolve"
	"github.com/saiaathish/picogent/internal/extensions"
	"github.com/saiaathish/picogent/internal/goal"
	"github.com/saiaathish/picogent/internal/learn"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/opencodeauth"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/projects"
	"github.com/saiaathish/picogent/internal/redact"
	"github.com/saiaathish/picogent/internal/scope"
	"github.com/saiaathish/picogent/internal/session"
	"github.com/saiaathish/picogent/internal/setup"
	"github.com/saiaathish/picogent/internal/slash"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/trace"
	"github.com/saiaathish/picogent/internal/verify"
)

type event struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Summary   string          `json:"summary,omitempty"`
	Hint      string          `json:"hint,omitempty"`
	Path      string          `json:"path,omitempty"`
	Line      int             `json:"line,omitempty"`
	LineEnd   int             `json:"line_end,omitempty"`
	Added     int             `json:"added,omitempty"`
	Removed   int             `json:"removed,omitempty"`
	Count     int             `json:"count,omitempty"`
	Kind      string          `json:"kind,omitempty"`
	Status    string          `json:"status,omitempty"`
	Available bool            `json:"available,omitempty"`
	Tokens    int             `json:"tokens,omitempty"`
	Budget    int             `json:"budget,omitempty"`
	Pct       float64         `json:"pct,omitempty"`
	Level     string          `json:"level,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	Task      *taskstate.Task `json:"task"`
	turnGen   uint64          `json:"-"`
}

type transcriptLine struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

const maxQueuedTurns = 8

type queuedTurn struct {
	prompt         string
	parts          []llm.Part
	display        string
	mode           *agent.TaskMode
	automaticScope bool
	scopeNotice    string
	scopeBoundary  string
}

// turnAdmission is the immutable runtime boundary captured when a turn is
// admitted. A queued follow-up is admitted under the same lock as the
// completion of the previous turn, so it cannot be lost between busy/done
// transitions or accidentally run against a newer session.
type turnAdmission struct {
	hist           []llm.Message
	runAgent       *agent.Agent
	runSession     string
	workspace      string
	myGen          uint64
	beforeAgentRun func()
	ctx            context.Context
	turnPermCh     chan perm.Decision
	temporaryMode  bool
	automaticScope bool
	scopeNotice    string
	scopeBoundary  string
	goalEpoch      uint64
}

type server struct {
	cfg            config.Config
	ag             *agent.Agent
	saveConfig     func(config.Config) error
	configTxMu     sync.Mutex
	mu             sync.Mutex
	hist           []llm.Message
	sessionID      string
	permCh         chan perm.Decision
	subs           []chan event
	busy           bool
	activeTurns    int
	cancel         context.CancelFunc
	steerMu        sync.Mutex
	steerQueue     []queuedTurn
	undoStack      []extensions.UndoEntry
	pendingPerm    perm.Request
	pendingPermGen uint64
	liveTask       agent.TaskMode
	// turnMode is a temporary scope boundary for the admitted turn. It is
	// exposed in state while the turn runs but never replaces liveTask.
	turnMode  *agent.TaskMode
	turnGen   uint64 // bumped on cancel/new chat so stale turns cannot rewrite hist
	goalEpoch uint64 // bumped whenever a user explicitly replaces/clears a goal
	// beforeAgentRun is test-only synchronization for proving that a turn uses
	// the agent/session it captured before a session switch. Production leaves
	// it nil.
	beforeAgentRun func()

	// Side chat companion (Codex-style)
	sideHist     []llm.Message
	sideBusy     bool
	turnStarted  time.Time
	turnPrompt   string
	turnReads    int
	turnSearches int
	turnEdits    int

	// AI prompt recommendations (main hero + side chips)
	mainRecs   []promptRec
	sideRecs   []promptRec
	mainRecsAt time.Time
	sideRecsAt time.Time
	recsKey    string
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
	if a != nil {
		if err := a.SetTaskSession(sessID); err != nil {
			return fmt.Errorf("load durable task state: %w", err)
		}
	}
	s := &server{cfg: cfg, ag: a, permCh: make(chan perm.Decision, 1), sessionID: sessID, hist: hist}
	s.attachRouterHook()
	s.ensureProject()
	addr := "127.0.0.1:7420"
	if v := os.Getenv("PICOGENT_GUI_ADDR"); v != "" {
		if !loopbackListenAddress(v) {
			return fmt.Errorf("PICOGENT_GUI_ADDR must bind to loopback (127.0.0.1, ::1, or localhost); refusing %q", v)
		}
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

func loopbackListenAddress(addr string) bool {
	_, ok := parseLiteralLoopbackHost(addr)
	return ok
}

// literalLoopbackHost is deliberately narrower than net.IP.IsLoopback. The
// browser GUI is a local trust boundary: accepting a name that merely resolves
// to loopback would let DNS rebinding turn a hostile origin into a same-origin
// request. Only the URLs Picogent itself advertises are accepted.
type literalLoopbackHost struct {
	host string
	port string
}

func parseLiteralLoopbackHost(raw string) (literalLoopbackHost, bool) {
	if raw == "" || strings.TrimSpace(raw) != raw || strings.ContainsAny(raw, "@/?#") {
		return literalLoopbackHost{}, false
	}

	host, port := raw, ""
	hasPort := false
	switch {
	case strings.HasPrefix(raw, "["):
		end := strings.IndexByte(raw, ']')
		if end <= 1 {
			return literalLoopbackHost{}, false
		}
		host = raw[1:end]
		rest := raw[end+1:]
		switch {
		case rest == "":
		case strings.HasPrefix(rest, ":"):
			port = rest[1:]
			hasPort = true
		default:
			return literalLoopbackHost{}, false
		}
	case strings.Count(raw, ":") == 1:
		host, port, _ = strings.Cut(raw, ":")
		hasPort = true
	case strings.Count(raw, ":") > 1 && raw != "::1":
		// IPv6 hosts with a port must be bracketed. The one unbracketed literal
		// allowed here is the no-port form required by HTTP Host parsing.
		return literalLoopbackHost{}, false
	}

	if host == "" || (hasPort && port == "") {
		return literalLoopbackHost{}, false
	}
	switch {
	case host == "127.0.0.1", host == "::1":
	case strings.EqualFold(host, "localhost"):
		host = "localhost"
	default:
		return literalLoopbackHost{}, false
	}
	if hasPort {
		for _, r := range port {
			if r < '0' || r > '9' {
				return literalLoopbackHost{}, false
			}
		}
		n, err := strconv.ParseUint(port, 10, 16)
		if err != nil || n > 65535 {
			return literalLoopbackHost{}, false
		}
		port = strconv.FormatUint(n, 10)
	}
	return literalLoopbackHost{host: host, port: port}, true
}

func (s *server) apiRoute(allowed []string, next http.HandlerFunc) http.Handler {
	methods := make(map[string]struct{}, len(allowed))
	for _, method := range allowed {
		methods[method] = struct{}{}
	}
	allow := strings.Join(allowed, ", ")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Report an unsupported verb before evaluating its Origin. This keeps
		// every route's API contract deterministic and prevents a malformed
		// request from masking a method regression as an origin failure.
		if _, ok := methods[r.Method]; !ok {
			w.Header().Set("Allow", allow)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		host, ok := parseLiteralLoopbackHost(r.Host)
		if !ok {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if isUnsafeAPIMethod(r.Method) && !sameLoopbackOrigin(r, host) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	})
}

func isUnsafeAPIMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func sameLoopbackOrigin(r *http.Request, want literalLoopbackHost) bool {
	origins := r.Header.Values("Origin")
	if len(origins) != 1 || origins[0] == "" {
		return false
	}
	u, err := url.ParseRequestURI(origins[0])
	if err != nil || u.Scheme != "http" || u.Host == "" || u.User != nil ||
		u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return false
	}
	got, ok := parseLiteralLoopbackHost(u.Host)
	return ok && got.host == want.host && normalizedHTTPPort(got.port) == normalizedHTTPPort(want.port)
}

func normalizedHTTPPort(port string) string {
	if port == "" {
		return "80"
	}
	return port
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
	api := func(path string, methods []string, handler http.HandlerFunc) {
		mux.Handle(path, s.apiRoute(methods, handler))
	}
	api("/api/state", []string{http.MethodGet}, s.state)
	api("/api/setup", []string{http.MethodGet}, s.setupStatus)
	api("/api/setup/install", []string{http.MethodPost}, s.setupInstall)
	api("/api/setup/login", []string{http.MethodPost}, s.setupLogin)
	api("/api/setup/finish", []string{http.MethodPost}, s.setupFinish)
	api("/api/chat", []string{http.MethodPost}, s.chat)
	api("/api/permission", []string{http.MethodPost}, s.permission)
	api("/api/mode", []string{http.MethodPost}, s.setMode)
	api("/api/task-mode", []string{http.MethodPost}, s.setTaskMode)
	api("/api/cancel", []string{http.MethodPost}, s.cancelChat)
	api("/api/reset", []string{http.MethodPost}, s.reset)
	api("/api/sessions", []string{http.MethodGet, http.MethodPost}, s.sessions)
	api("/api/file", []string{http.MethodGet}, s.readFile)
	api("/api/settings", []string{http.MethodGet, http.MethodPost}, s.settings)
	api("/api/router", []string{http.MethodGet, http.MethodPost}, s.routerAPI)
	api("/api/projects", []string{http.MethodGet, http.MethodPost}, s.projectsAPI)
	api("/api/folder/pick", []string{http.MethodPost}, s.folderPickAPI)
	api("/api/files/pick", []string{http.MethodPost}, s.filesPickAPI)
	api("/api/overview", []string{http.MethodGet}, s.overviewAPI)
	api("/api/evolve", []string{http.MethodGet, http.MethodDelete}, s.evolveAPI)
	api("/api/diff", []string{http.MethodGet}, s.diffAPI)
	api("/api/extensions", []string{http.MethodGet, http.MethodPost}, s.extensionsAPI)
	api("/api/trace", []string{http.MethodGet}, s.traceAPI)
	api("/api/help", []string{http.MethodGet, http.MethodPost}, s.helpAPI)
	api("/api/sidechat", []string{http.MethodGet, http.MethodPost}, s.sidechatAPI)
	api("/api/prompts", []string{http.MethodGet, http.MethodPost}, s.promptsAPI)
	api("/api/events", []string{http.MethodGet}, s.events)
	mux.Handle("/", noCacheStatic(http.FileServer(http.FS(static))))
	return mux
}

func noCacheStatic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, max-age=0")
		next.ServeHTTP(w, r)
	})
}

func (s *server) traceAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", 405)
		return
	}
	var events []trace.Event
	s.mu.Lock()
	ag := s.ag
	s.mu.Unlock()
	if ag != nil {
		if log := ag.TraceSnapshot(); log != nil {
			events = log.Tail(80)
		}
	}
	if events == nil {
		events = []trace.Event{}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"events": events})
}

func (s *server) attachRouterHook() {
	s.mu.Lock()
	ag := s.ag
	s.mu.Unlock()
	if ag == nil {
		return
	}
	r, ok := ag.ClientSnapshot().(*llm.Router)
	if !ok {
		return
	}
	prev := r.LastRouteHook()
	r.SetOnRoute(func(dec llm.RouteDecision) {
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
		s.emit(event{Type: "route", Text: dec.Label, Summary: dec.Reason, Tokens: dec.EstTokens})
	})
}

func (s *server) emit(e event) {
	e = sanitizeEvent(e)
	s.mu.Lock()
	if e.turnGen != 0 && (s.sessionID != e.SessionID || s.turnGen != e.turnGen) {
		s.mu.Unlock()
		return
	}
	subs := append([]chan event(nil), s.subs...)
	s.mu.Unlock()
	for _, ch := range subs {
		if e.turnGen != 0 {
			s.mu.Lock()
			live := s.sessionID == e.SessionID && s.turnGen == e.turnGen
			s.mu.Unlock()
			if !live {
				return
			}
		}
		select {
		case ch <- e:
		case <-time.After(2 * time.Second):
		}
	}
}

const maxGUIErrorBytes = 240

func guiDiagnostic(value string) string {
	return redact.Diagnostic(value, maxGUIErrorBytes)
}

func writeGUIError(w http.ResponseWriter, message string, status int) {
	http.Error(w, guiDiagnostic(message), status)
}

func sanitizeEvent(e event) event {
	switch e.Type {
	case "error":
		e.Text = guiDiagnostic(e.Text)
	case "permission":
		e.Text = guiDiagnostic(e.Text)
		e.Summary = guiDiagnostic(e.Summary)
		e.Hint = guiDiagnostic(e.Hint)
	}
	return e
}

func (s *server) emitTaskSnapshot(sessionID string) {
	s.mu.Lock()
	ag := s.ag
	s.mu.Unlock()
	var task *taskstate.Task
	if ag != nil {
		task = ag.TaskSnapshot()
		if task != nil && task.SessionID != sessionID {
			task = nil
		}
	}
	s.emit(event{Type: "task_progress", SessionID: sessionID, Task: task})
}

func initialSession(workspace string) (id string, hist []llm.Message) {
	if prev, err := session.Latest(workspace); err == nil {
		return prev.ID, prev.Messages
	}
	return session.New(workspace).ID, nil
}

// cloneAgentForSession gives a newly selected chat its own agent state. In
// particular, TaskSession and the in-memory durable task must not be changed
// underneath a turn that is still unwinding after cancellation. The provider,
// tool registry, and trace log are safe to share; the permission gate is
// per-agent because a canceled turn may still be finishing while the next
// session starts.
func cloneAgentForSession(src *agent.Agent, sessionID string) (*agent.Agent, error) {
	if src == nil {
		return nil, nil
	}
	state := src.RuntimeSnapshot()
	var gate *perm.Gate
	if state.Gate != nil {
		gate = perm.New(state.CFG.Mode, state.CFG.Workspace, nil)
		gate.SetAlwaysAllowed(state.Gate.AlwaysAllowedTools())
	} else {
		gate = perm.New(state.CFG.Mode, state.CFG.Workspace, nil)
	}
	clone := agent.New(state.CFG, state.LLM, state.Tools, gate)
	clone.SetProjectRules(state.ProjectRules)
	clone.SetSkillRules(state.SkillRules)
	clone.SetMemory(state.Memory)
	clone.SetGoalState(state.Goal, state.GoalRevision)
	clone.SetTrace(state.Trace)
	clone.SetTaskStore(src.TaskStoreSnapshot())
	clone.SetTaskMode(state.TaskMode)
	if err := clone.SetTaskSession(sessionID); err != nil {
		return nil, fmt.Errorf("load durable task state for session %q: %w", sessionID, err)
	}
	return clone, nil
}

// newSessionLocked rotates the chat and agent together. Callers must hold
// s.mu. A stale turn retains the old agent pointer and therefore can never
// create or update durable task state for the new session.
func (s *server) newSessionLocked() (string, error, error) {
	var saveErr error
	if len(s.hist) > 0 {
		saveErr = session.SaveMessages(s.cfg.Workspace, s.sessionID, s.hist)
	}
	s.abortTurnLocked()
	nextID := session.New(s.cfg.Workspace).ID
	var next *agent.Agent
	if s.ag != nil {
		var err error
		next, err = cloneAgentForSession(s.ag, nextID)
		if err != nil {
			return s.sessionID, saveErr, err
		}
		next.SetTaskMode(agent.TaskAgent)
	}
	s.sessionID = nextID
	s.hist = nil
	s.sideHist = nil
	s.liveTask = agent.TaskAgent
	if next != nil {
		s.ag = next
	}
	return s.sessionID, saveErr, nil
}

func (s *server) snapshot() map[string]any {
	s.mu.Lock()
	cfg := s.cfg
	ag := s.ag
	busy := s.busy
	sessionID := s.sessionID
	hist := append([]llm.Message(nil), s.hist...)
	pend := s.pendingPerm
	liveTask := s.liveTask
	var turnMode *agent.TaskMode
	if s.turnMode != nil {
		copyMode := *s.turnMode
		turnMode = &copyMode
	}
	s.mu.Unlock()
	var task *taskstate.Task
	undoAvailable := false
	if ag != nil {
		task = ag.TaskSnapshot()
		undoAvailable = ag.UndoAvailable()
		if task != nil && task.SessionID != sessionID {
			task = nil
		}
	}

	hint := ""
	if err := cfg.MissingAuth(); err != nil {
		hint = err.Error()
	}
	taskMode := liveTask
	temporaryTaskMode := turnMode != nil && turnMode.Valid()
	if temporaryTaskMode {
		taskMode = *turnMode
	}
	if !taskMode.Valid() && ag != nil && ag.TaskModeSnapshot().Valid() {
		taskMode = ag.TaskModeSnapshot()
	}
	if !taskMode.Valid() {
		taskMode = agent.ParseTaskMode(cfg.TaskMode)
	}
	out := map[string]any{
		"mode":                cfg.Mode,
		"saved_mode":          cfg.PersistentMode(),
		"mode_overridden":     cfg.ModeOverridden(),
		"task_mode":           string(taskMode),
		"model":               cfg.DisplayModel(),
		"workspace":           cfg.Workspace,
		"provider":            cfg.Provider,
		"codex":               cfg.Provider == config.ProviderCodex && codexauth.LoggedIn(),
		"codex_cli":           codexauth.LoggedIn(),
		"quadcode":            cfg.Provider == config.ProviderQuadCode && (cfg.AnthropicKeyResolved() != "" || claudeauth.LoggedIn()),
		"claude_cli":          claudeauth.LoggedIn(),
		"opencode":            cfg.Provider == config.ProviderOpenCode && opencodeauth.LoggedIn(),
		"opencode_cli":        opencodeauth.LoggedIn(),
		"antigravity":         cfg.Provider == config.ProviderAntigravity && agyauth.LoggedIn(),
		"antigravity_cli":     agyauth.LoggedIn(),
		"busy":                busy,
		"task_mode_temporary": temporaryTaskMode,
		"hint":                hint,
		"auth":                setup.ProviderAuthPrompt(cfg),
		"setup":               !cfg.SetupComplete,
		"mcp_tools":           mcpToolCount(ag),
		"session_id":          sessionID,
		"undo_available":      undoAvailable,
		"task":                task,
		"router":              s.routerSnapshot(),
		"model_options":       llm.ModelChoices(llm.Ecosystem(cfg.RouterEcosystem()), cfg.FableAllowed()),
		"slash":               slash.Catalog(cfg.Workspace),
	}
	if store, err := learn.Load(cfg.Workspace); err == nil {
		out["overview"] = store
	}
	if ev, err := evolve.Load(cfg.Workspace); err == nil {
		active := 0
		for _, p := range ev.Playbooks {
			if !p.Archived {
				active++
			}
		}
		out["evolve"] = map[string]any{
			"summary":   evolve.Summary(ev),
			"habits":    len(ev.Habits),
			"playbooks": active,
		}
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
		if r, ok := ag.ClientSnapshot().(*llm.Router); ok {
			dec := r.LastDecision()
			if dec.Model != "" {
				last = map[string]any{
					"tier":         dec.Tier,
					"label":        dec.Label,
					"model":        dec.Model,
					"reason":       dec.Reason,
					"score":        dec.Score,
					"advisor":      dec.Advisor,
					"reasoning":    dec.Reasoning,
					"task_kind":    dec.TaskKind,
					"route_mode":   dec.RouteMode,
					"token_save_x": dec.TokenSaveX,
					"est_tokens":   dec.EstTokens,
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
		writeGUIError(w, "GET or POST only", 405)
	}
}

func mcpToolCount(a *agent.Agent) int {
	if a == nil || a.Tools == nil {
		return 0
	}
	mcp := a.Tools.MCPManagerSnapshot()
	if mcp == nil {
		return 0
	}
	return len(mcp.Tools())
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
		writeGUIError(w, "POST only", 405)
		return
	}
	log, err := setup.InstallCores()
	// Model discovery may invoke installed CLIs and public catalogs, so keep it
	// on the explicit install action rather than the setup-status GET path.
	llm.RefreshCLIModels(true)
	w.Header().Set("Content-Type", "application/json")
	out := map[string]any{"log": log, "ok": err == nil}
	if err != nil {
		out["error"] = guiDiagnostic(err.Error())
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
		writeGUIError(w, "POST only", 405)
		return
	}
	var in struct {
		Target   string `json:"target"`
		ReturnTo string `json:"return_to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeGUIError(w, err.Error(), 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch strings.ToLower(in.Target) {
	case "claude":
		if err := setup.StartClaudeLogin(); err != nil {
			writeGUIError(w, err.Error(), 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":   true,
			"hint": "A Terminal window opened for Claude login. Finish there, then come back — Picogent will detect it automatically.",
		})
	case "opencode":
		if err := setup.StartOpenCodeLogin(); err != nil {
			writeGUIError(w, err.Error(), 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":   true,
			"hint": "A Terminal window opened for OpenCode login. Choose Zen and/or Go, paste your key, then come back here.",
		})
	case "antigravity", "agy":
		if err := setup.StartAntigravityLogin(); err != nil {
			writeGUIError(w, err.Error(), 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":   true,
			"hint": "Antigravity opened in Terminal. Sign in with Google there, then come back here.",
		})
	case "codex-cli":
		if err := setup.StartCodexCLILogin(); err != nil {
			writeGUIError(w, err.Error(), 400)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":   true,
			"hint": "A Terminal window opened for Codex login. Finish there, then come back.",
		})
	default:
		if in.ReturnTo == "" {
			in.ReturnTo = "http://127.0.0.1:7420/"
		}
		authURL, err := codexauth.BeginBrowserLogin(in.ReturnTo)
		if err != nil {
			// Fall back to CLI login in a Terminal window.
			if e2 := setup.StartCodexCLILogin(); e2 != nil {
				writeGUIError(w, err.Error(), 500)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":   true,
				"hint": "Opened Codex login in Terminal (browser OAuth unavailable).",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "url": authURL})
	}
}

func (s *server) setupFinish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeGUIError(w, "POST only", 405)
		return
	}
	var in struct {
		Workspace string `json:"workspace"`
		Mode      string `json:"mode"`
		Model     string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeGUIError(w, err.Error(), 400)
		return
	}
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	next, err := setup.Apply(cfg, in.Workspace, in.Mode, in.Model)
	if err != nil {
		writeGUIError(w, err.Error(), 400)
		return
	}
	a, err := app.Build(next)
	if err != nil {
		writeGUIError(w, err.Error(), 500)
		return
	}
	s.mu.Lock()
	if err := a.SetTaskSession(s.sessionID); err != nil {
		s.mu.Unlock()
		writeGUIError(w, "couldn't load durable task state: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.abortTurnLocked()
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
		writeGUIError(w, err.Error(), 400)
		return
	}
	m := config.Mode(in.Mode)
	if !m.Valid() {
		writeGUIError(w, "invalid mode", 400)
		return
	}
	s.configTxMu.Lock()
	defer s.configTxMu.Unlock()
	s.mu.Lock()
	// Save a prospective config before exposing it to this process. A failed
	// persistence write must not look like a successful mode change in either
	// the GUI state or the permission gate.
	next := s.cfg
	next.SetUserMode(m)
	if err := s.persistConfig(next); err != nil {
		s.mu.Unlock()
		writeGUIError(w, "couldn't save mode: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.cfg = next
	if s.ag != nil {
		s.ag.UpdateConfig(func(cfg *config.Config) { cfg.SetUserMode(m) })
	}
	s.mu.Unlock()
	w.WriteHeader(204)
}

func (s *server) persistConfig(cfg config.Config) error {
	if s.saveConfig != nil {
		return s.saveConfig(cfg)
	}
	return config.Save(cfg)
}

func closeCandidateAgent(a *agent.Agent) {
	if a == nil || a.Tools == nil {
		return
	}
	a.Tools.Close()
}

func (s *server) setTaskMode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TaskMode string `json:"task_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeGUIError(w, err.Error(), 400)
		return
	}
	m := agent.ParseTaskMode(in.TaskMode)
	if !m.Valid() {
		writeGUIError(w, "invalid task mode", 400)
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

func (s *server) autoApplyFromUserPrompt(prompt string, expectedGen uint64) error {
	s.mu.Lock()
	expectedEpoch := s.goalEpoch
	s.mu.Unlock()
	return s.autoApplyInferredFromUserPrompt(prompt, expectedGen, expectedEpoch, false)
}

func (s *server) autoApplyScopedFromUserPrompt(prompt string, expectedGen uint64) error {
	s.mu.Lock()
	expectedEpoch := s.goalEpoch
	s.mu.Unlock()
	return s.autoApplyInferredFromUserPrompt(prompt, expectedGen, expectedEpoch, true)
}

func (s *server) autoApplyInferredFromUserPrompt(prompt string, expectedGen, expectedEpoch uint64, automaticScope bool) error {
	if strings.HasPrefix(strings.TrimSpace(prompt), "/") {
		return nil
	}
	s.mu.Lock()
	ag := s.ag
	sessionID := s.sessionID
	goalEpoch := s.goalEpoch
	cur := s.liveTask
	cfg := s.cfg
	goalText := ""
	if !cur.Valid() && ag != nil {
		cur = ag.TaskModeSnapshot()
	}
	if ag != nil {
		goalText = ag.GoalSnapshot()
	}
	s.mu.Unlock()
	if ag == nil || !cfg.AutoTaskModeOn() {
		return nil
	}

	dec := agent.InferAuto(prompt, cur, goalText)
	if automaticScope {
		dec = agent.InferAutomaticScope(prompt, cur, goalText)
	}
	goalApplied := false
	var goalErr error
	if dec.GoalSet && dec.Goal != goalText {
		// Inference is deterministic but may run across a reset/rebuild. Keep
		// the side effect attached to the admitted agent and session.
		s.mu.Lock()
		if s.ag == ag && s.sessionID == sessionID && s.turnGen == expectedGen && s.goalEpoch == expectedEpoch && s.goalEpoch == goalEpoch {
			if revision, err := goal.SetState(s.cfg.Workspace, dec.Goal); err == nil {
				ag.SetGoalState(dec.Goal, revision)
				goalApplied = true
			} else {
				goalErr = fmt.Errorf("couldn't save inferred goal: %w", err)
				s.goalEpoch++ // fail closed: this turn may not retire the old goal
			}
		}
		s.mu.Unlock()
	}
	s.mu.Lock()
	modeApplied := s.ag == ag && s.sessionID == sessionID && s.turnGen == expectedGen
	if modeApplied {
		s.liveTask = dec.TaskMode
		ag.SetTaskMode(dec.TaskMode)
	}
	s.mu.Unlock()
	if goalApplied {
		s.emit(event{Type: "goal", Text: dec.Goal})
	}
	if goalErr != nil {
		s.emit(event{Type: "error", Text: goalErr.Error()})
		return goalErr
	}
	if modeApplied && dec.TaskMode != cur {
		s.emit(event{Type: "task_mode", Text: string(dec.TaskMode)})
	}
	return nil
}

func (s *server) autoApplyGoalFromUserPrompt(prompt string, expectedGen uint64) error {
	s.mu.Lock()
	expectedEpoch := s.goalEpoch
	s.mu.Unlock()
	return s.autoApplyGoalFromUserPromptAt(prompt, expectedGen, expectedEpoch)
}

func (s *server) autoApplyGoalFromUserPromptAt(prompt string, expectedGen, expectedEpoch uint64) error {
	if strings.HasPrefix(strings.TrimSpace(prompt), "/") {
		return nil
	}
	s.mu.Lock()
	ag := s.ag
	sessionID := s.sessionID
	goalEpoch := s.goalEpoch
	cfg := s.cfg
	cur := s.liveTask
	if !cur.Valid() && ag != nil {
		cur = ag.TaskModeSnapshot()
	}
	goalText := ""
	if ag != nil {
		goalText = ag.GoalSnapshot()
	}
	s.mu.Unlock()
	if ag == nil || !cfg.AutoTaskModeOn() {
		return nil
	}
	dec := agent.InferAuto(prompt, cur, goalText)
	if dec.GoalSet && dec.Goal != goalText {
		var goalErr error
		s.mu.Lock()
		if s.ag == ag && s.sessionID == sessionID && s.turnGen == expectedGen && s.goalEpoch == expectedEpoch && s.goalEpoch == goalEpoch {
			if revision, err := goal.SetState(s.cfg.Workspace, dec.Goal); err == nil {
				ag.SetGoalState(dec.Goal, revision)
				s.mu.Unlock()
				s.emit(event{Type: "goal", Text: dec.Goal})
				return nil
			} else {
				goalErr = fmt.Errorf("couldn't save inferred goal: %w", err)
				s.goalEpoch++ // fail closed: this turn may not retire the old goal
			}
		}
		s.mu.Unlock()
		if goalErr != nil {
			s.emit(event{Type: "error", Text: goalErr.Error()})
			return goalErr
		}
	}
	return nil
}

func (s *server) queueSteer(prompt string, parts []llm.Part, display string) bool {
	return s.queueSteerMode(prompt, parts, display, nil)
}

func (s *server) queueSteerMode(prompt string, parts []llm.Part, display string, mode *agent.TaskMode) bool {
	return s.queueSteerScoped(prompt, parts, display, mode, false, "", "")
}

func (s *server) queueSteerScoped(prompt string, parts []llm.Part, display string, mode *agent.TaskMode, automaticScope bool, scopeNotice, scopeBoundary string) bool {
	s.steerMu.Lock()
	defer s.steerMu.Unlock()
	if len(s.steerQueue) >= maxQueuedTurns {
		return false
	}
	queued := queuedTurn{
		prompt:         prompt,
		parts:          append([]llm.Part(nil), parts...),
		display:        display,
		automaticScope: automaticScope,
		scopeNotice:    scopeNotice,
		scopeBoundary:  scopeBoundary,
	}
	if mode != nil {
		copyMode := *mode
		queued.mode = &copyMode
	}
	s.steerQueue = append(s.steerQueue, queued)
	return true
}

func (s *server) popSteer() (string, []llm.Part, string, *agent.TaskMode, bool) {
	s.steerMu.Lock()
	defer s.steerMu.Unlock()
	if len(s.steerQueue) == 0 {
		return "", nil, "", nil, false
	}
	next := s.steerQueue[0]
	s.steerQueue[0] = queuedTurn{}
	s.steerQueue = s.steerQueue[1:]
	return next.prompt, next.parts, next.display, next.mode, true
}

func (s *server) abortTurnLocked() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.turnGen++
	s.busy = false
	s.turnStarted = time.Time{}
	s.turnPrompt = ""
	s.turnReads, s.turnSearches, s.turnEdits = 0, 0, 0
	s.pendingPerm = perm.Request{}
	s.pendingPermGen = 0
	s.turnMode = nil
	select {
	case s.permCh <- perm.Deny:
	default:
	}
	s.steerMu.Lock()
	s.steerQueue = nil
	s.steerMu.Unlock()
}

func (s *server) startAgentTurn(prompt string, parts []llm.Part) {
	s.startAgentTurnAsMode(prompt, parts, prompt, nil)
}

func (s *server) startAgentTurnAs(prompt string, parts []llm.Part, displayPrompt string) {
	s.startAgentTurnAsMode(prompt, parts, displayPrompt, nil)
}

func (s *server) startAgentTurnAsMode(prompt string, parts []llm.Part, displayPrompt string, mode *agent.TaskMode) {
	if strings.TrimSpace(displayPrompt) == "" {
		displayPrompt = prompt
	}
	s.mu.Lock()
	admitted, ok := s.admitAgentTurnLocked(mode, false)
	s.mu.Unlock()
	if !ok {
		return
	}
	s.runAdmittedTurn(admitted, prompt, parts, displayPrompt, mode)
}

// admitAgentTurnLocked captures all project/session/runtime state for one
// turn. Callers must hold s.mu. allowBusy is used only by the atomic queued
// handoff in runAdmittedTurn; the current turn is still marked busy there.
func (s *server) admitAgentTurnLocked(mode *agent.TaskMode, allowBusy bool) (turnAdmission, bool) {
	if s.busy && !allowBusy {
		return turnAdmission{}, false
	}
	s.busy = true
	s.activeTurns++
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.turnGen++
	turnPermCh := make(chan perm.Decision, 1)
	s.permCh = turnPermCh
	temporaryMode := mode != nil && mode.Valid()
	if temporaryMode {
		copyMode := *mode
		s.turnMode = &copyMode
	} else {
		s.turnMode = nil
	}
	return turnAdmission{
		hist:           s.hist,
		runAgent:       s.ag,
		runSession:     s.sessionID,
		workspace:      s.cfg.Workspace,
		myGen:          s.turnGen,
		beforeAgentRun: s.beforeAgentRun,
		ctx:            ctx,
		turnPermCh:     turnPermCh,
		temporaryMode:  temporaryMode,
		goalEpoch:      s.goalEpoch,
	}, true
}

func (s *server) runAdmittedTurn(admitted turnAdmission, prompt string, parts []llm.Part, displayPrompt string, mode *agent.TaskMode) {
	runAgent := admitted.runAgent
	runSession := admitted.runSession
	workspace := admitted.workspace
	myGen := admitted.myGen
	myGoalEpoch := admitted.goalEpoch
	ctx := admitted.ctx
	turnPermCh := admitted.turnPermCh
	temporaryMode := admitted.temporaryMode
	automaticScope := admitted.automaticScope
	hist := admitted.hist
	beforeAgentRun := admitted.beforeAgentRun
	if admitted.scopeNotice != "" {
		s.emit(event{Type: "system", Text: admitted.scopeNotice, SessionID: runSession, turnGen: myGen})
	}
	// Infer persistent state only after this turn is admitted. Queued
	// follow-ups and canceled requests cannot leave a goal or mode behind.
	var intentErr error
	if temporaryMode {
		intentErr = s.autoApplyGoalFromUserPromptAt(displayPrompt, myGen, myGoalEpoch)
	} else if automaticScope {
		intentErr = s.autoApplyInferredFromUserPrompt(displayPrompt, myGen, myGoalEpoch, true)
	} else {
		intentErr = s.autoApplyInferredFromUserPrompt(displayPrompt, myGen, myGoalEpoch, false)
	}
	if intentErr != nil {
		var next *turnAdmission
		var nextPrompt string
		var nextParts []llm.Part
		var nextDisplay string
		var nextMode *agent.TaskMode
		s.mu.Lock()
		live := s.turnGen == myGen
		if live {
			// An inference failure must not strand turns that were queued behind
			// this admission. Keep the handoff atomic with busy-state cleanup,
			// matching the normal completion path below.
			s.steerMu.Lock()
			if len(s.steerQueue) > 0 {
				queued := s.steerQueue[0]
				s.activeTurns--
				admittedNext, admittedOK := s.admitAgentTurnLocked(queued.mode, true)
				if admittedOK {
					admittedNext.automaticScope = queued.automaticScope
					admittedNext.scopeNotice = queued.scopeNotice
					admittedNext.scopeBoundary = queued.scopeBoundary
					s.steerQueue[0] = queuedTurn{}
					s.steerQueue = s.steerQueue[1:]
					next = &admittedNext
					nextPrompt = queued.prompt
					nextParts = queued.parts
					nextDisplay = queued.display
					nextMode = queued.mode
				} else {
					s.activeTurns++
				}
			}
			s.steerMu.Unlock()
			if next == nil {
				s.busy = false
				s.cancel = nil
				s.turnMode = nil
				s.activeTurns--
			}
		} else {
			s.activeTurns--
		}
		s.mu.Unlock()
		if next != nil {
			s.runAdmittedTurn(*next, nextPrompt, nextParts, nextDisplay, nextMode)
			return
		}
		if live {
			s.emit(event{Type: "done"})
		}
		return
	}
	if temporaryMode {
		if mode != nil {
			s.emit(event{Type: "task_mode", Text: string(*mode), Hint: "this turn"})
		}
	}
	s.noteTurnStart(displayPrompt)
	s.mu.Lock()
	stillLive := s.turnGen == myGen
	if !stillLive {
		s.activeTurns--
	}
	s.mu.Unlock()
	if !stillLive {
		return
	}

	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				s.emit(event{Type: "error", Text: fmt.Sprintf("agent panic: %v", rec)})
				fmt.Fprintln(os.Stderr, guiDiagnostic(fmt.Sprintf("picogent: agent panic: %v", rec)))
			}
			var next *turnAdmission
			var nextPrompt string
			var nextParts []llm.Part
			var nextDisplay string
			var nextMode *agent.TaskMode
			s.mu.Lock()
			live := s.turnGen == myGen
			restoreMode := s.liveTask
			if live {
				// Keep the server busy while handing off. Holding both locks
				// means the queue entry is removed only after the replacement
				// turn has already been admitted, so no request can win the
				// idle window.
				s.steerMu.Lock()
				if len(s.steerQueue) > 0 {
					queued := s.steerQueue[0]
					s.activeTurns--
					admittedNext, admittedOK := s.admitAgentTurnLocked(queued.mode, true)
					if admittedOK {
						admittedNext.automaticScope = queued.automaticScope
						admittedNext.scopeNotice = queued.scopeNotice
						admittedNext.scopeBoundary = queued.scopeBoundary
						s.steerQueue[0] = queuedTurn{}
						s.steerQueue = s.steerQueue[1:]
						next = &admittedNext
						nextPrompt = queued.prompt
						nextParts = queued.parts
						nextDisplay = queued.display
						nextMode = queued.mode
					} else {
						s.activeTurns++
					}
				}
				s.steerMu.Unlock()
				if next == nil {
					s.busy = false
					s.cancel = nil
					s.turnMode = nil
					s.activeTurns--
				}
			}
			s.mu.Unlock()
			if !live {
				s.mu.Lock()
				s.activeTurns--
				s.mu.Unlock()
				return
			}
			s.clearTurnProgress()
			s.invalidatePromptRecs()
			s.emit(event{Type: "prompts_refresh", Text: "all"})
			if next != nil {
				s.runAdmittedTurn(*next, nextPrompt, nextParts, nextDisplay, nextMode)
				return
			}
			s.emit(event{Type: "done"})
			if temporaryMode && restoreMode.Valid() {
				s.emit(event{Type: "task_mode", Text: string(restoreMode)})
			}
		}()
		if runAgent == nil {
			s.emit(event{Type: "error", Text: "agent not ready"})
			return
		}
		if runAgent.TraceSnapshot() == nil {
			if log, err := trace.Open(workspace); err == nil {
				runAgent.SetTrace(log)
			}
		}
		h := newGUIHandlerAtWithPerm(s, runSession, myGen, turnPermCh)
		h.beginTurn(displayPrompt)
		s.maybeRecommendExtensions(displayPrompt)
		// Extension activation can rebuild the live agent while this turn is
		// being prepared. Use that replacement only if this turn still owns the
		// same session; never touch a newer session that won the race with it.
		s.mu.Lock()
		if s.sessionID == runSession && s.turnGen == myGen && s.ag != nil {
			runAgent = s.ag
			if err := runAgent.SetTaskSession(runSession); err != nil {
				s.mu.Unlock()
				h.OnError(fmt.Errorf("load durable task state: %w", err))
				return
			}
		}
		s.mu.Unlock()
		userMsg := llm.Message{Role: "user", Content: prompt, Parts: parts}
		if beforeAgentRun != nil {
			beforeAgentRun()
		}
		runGoal := ""
		var runGoalRevision uint64
		s.mu.Lock()
		if s.turnGen == myGen && s.goalEpoch == myGoalEpoch {
			runGoal, runGoalRevision = runAgent.GoalStateSnapshot()
			runGoal = strings.TrimSpace(runGoal)
		}
		s.mu.Unlock()
		next, result, err := runAgent.RunWithOptions(ctx, hist, userMsg, h, agent.RunOptions{TaskMode: mode, TracePrompt: displayPrompt, DurablePrompt: displayPrompt, ScopeBoundary: admitted.scopeBoundary})
		s.mu.Lock()
		stale := s.turnGen != myGen
		s.mu.Unlock()
		if stale {
			return
		}
		for i := len(next) - 1; i >= 0; i-- {
			if next[i].Role != "user" {
				continue
			}
			scopedPrompt := next[i].Content == prompt
			attachmentSummary := ""
			if len(next[i].Parts) > 0 {
				attachmentSummary = attachments.SummaryLine(next[i].Parts)
				next[i].Content = attachmentSummary + next[i].Content
				next[i].Parts = nil
			}
			if scopedPrompt && displayPrompt != "" {
				next[i].Content = attachmentSummary + displayPrompt
			}
			break
		}
		h.endTurn(result)
		if result.GoalDone && runGoal != "" {
			if err := s.clearGoalIf(runGoal, runGoalRevision, myGoalEpoch, myGen); err != nil {
				s.emit(event{Type: "error", Text: fmt.Sprintf("couldn't clear completed goal: %v", err)})
			}
		}
		s.mu.Lock()
		if s.turnGen != myGen {
			s.mu.Unlock()
			return
		}
		if s.liveTask.Valid() && s.ag != nil {
			s.ag.SetTaskMode(s.liveTask)
		}
		s.hist = next
		ws := s.cfg.Workspace
		sid := s.sessionID
		llmClient := runAgent.ClientSnapshot()
		model := runAgent.ConfigSnapshot().Model
		_ = config.Save(s.cfg)
		saveErr := session.SaveMessages(ws, sid, next)
		s.mu.Unlock()
		if saveErr != nil {
			s.emit(event{Type: "error", Text: fmt.Sprintf("couldn't save session: %v", saveErr)})
		}
		if result.Context.Tokens > 0 {
			s.emit(contextEvent(result.Context))
		}
		s.maybeAutoTitle(context.Background(), llmClient, model, ws, sid, next)
		if err != nil && ctx.Err() == nil {
			fmt.Fprintln(os.Stderr, guiDiagnostic(fmt.Sprintf("picogent: turn error: %v", err)))
			s.emit(event{Type: "error", Text: err.Error()})
		}
	}()
}

func (s *server) setGoal(text string) error {
	text = strings.TrimSpace(text)
	s.mu.Lock()
	workspace := s.cfg.Workspace
	revision, err := goal.SetState(workspace, text)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	if s.ag != nil {
		s.ag.SetGoalState(text, revision)
	}
	s.goalEpoch++
	s.mu.Unlock()
	s.emit(event{Type: "goal", Text: text})
	return nil
}

func (s *server) clearGoal() error {
	s.mu.Lock()
	workspace := s.cfg.Workspace
	if err := goal.Clear(workspace); err != nil {
		s.mu.Unlock()
		return err
	}
	if s.ag != nil {
		s.ag.SetGoalState("", 0)
	}
	s.goalEpoch++
	s.mu.Unlock()
	s.emit(event{Type: "goal", Text: ""})
	return nil
}

// clearGoalIf lets a completed turn retire only the goal it actually ran
// under. A queued or concurrently admitted newer goal must remain intact.
func (s *server) clearGoalIf(expected string, expectedRevision, expectedEpoch, expectedGen uint64) error {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return nil
	}
	s.mu.Lock()
	workspace := s.cfg.Workspace
	ag := s.ag
	if ag == nil || (expectedGen != 0 && s.turnGen != expectedGen) || s.goalEpoch != expectedEpoch {
		s.mu.Unlock()
		return nil
	}
	actualGoal, actualRevision := ag.GoalStateSnapshot()
	if actualGoal != expected || actualRevision != expectedRevision {
		s.mu.Unlock()
		return nil
	}
	cleared, err := goal.ClearIfState(workspace, expected, expectedRevision)
	if err != nil || !cleared {
		s.mu.Unlock()
		return err
	}
	if s.ag == ag {
		actualGoal, actualRevision := ag.GoalStateSnapshot()
		if actualGoal == expected && actualRevision == expectedRevision {
			ag.SetGoalState("", 0)
		}
	}
	s.mu.Unlock()
	s.emit(event{Type: "goal", Text: ""})
	return nil
}

func (s *server) reset(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	id, saveErr, taskErr := s.newSessionLocked()
	s.mu.Unlock()
	if taskErr != nil {
		writeGUIError(w, "couldn't load durable task state: "+taskErr.Error(), http.StatusInternalServerError)
		return
	}
	if saveErr != nil {
		s.emit(event{Type: "error", Text: fmt.Sprintf("couldn't save session: %v", saveErr)})
	}
	s.invalidatePromptRecs()
	s.emitTaskSnapshot(id)
	s.emit(event{Type: "undo", Status: "cleared"})
	s.emit(event{Type: "task_mode", Text: string(agent.TaskAgent)})
	s.emit(event{Type: "prompts_refresh", Text: "main"})
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
	s.mu.Lock()
	permCh := s.permCh
	tool := s.pendingPerm.Tool
	pendingGen := s.pendingPermGen
	turnGen := s.turnGen
	s.mu.Unlock()
	if permCh == nil || tool == "" {
		w.WriteHeader(204)
		return
	}
	select {
	case permCh <- d:
	case <-time.After(2 * time.Second):
	}
	s.mu.Lock()
	// Only the request that was visible when the user clicked may be cleared
	// or promoted to Always. A reset/new turn can replace pendingPerm while the
	// HTTP request is waiting on the old turn's channel.
	if s.pendingPermGen != pendingGen || s.turnGen != turnGen {
		s.mu.Unlock()
		w.WriteHeader(204)
		return
	}
	s.pendingPerm = perm.Request{}
	s.pendingPermGen = 0
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
	s.abortTurnLocked()
	restoreMode := s.liveTask
	s.mu.Unlock()
	s.emit(event{Type: "done"})
	if restoreMode.Valid() {
		s.emit(event{Type: "task_mode", Text: string(restoreMode)})
	}
	w.WriteHeader(204)
}

func (s *server) chat(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Prompt      string              `json:"prompt"`
		ScopeChoice string              `json:"scope_choice"`
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
	s.mu.Lock()
	workspace := s.cfg.Workspace
	s.mu.Unlock()
	kind, payload := slash.Resolve(workspace, prompt)
	scopeNotice := ""
	scopeBoundary := ""
	automaticScope := false
	var scopeMode *agent.TaskMode
	if kind == slash.Unknown {
		if preflight, needed := scope.Analyze(prompt); needed {
			choiceID := strings.TrimSpace(in.ScopeChoice)
			automatic := choiceID == ""
			if automatic {
				choiceID = scope.Recommended(preflight).ID
				automaticScope = true
			}
			choice, selected := scope.Select(preflight, choiceID)
			if !selected {
				http.Error(w, "invalid scope choice", http.StatusBadRequest)
				return
			}
			if automatic {
				scopeNotice = scope.DefaultMessage(choice)
			}
			scopeBoundary = scope.TurnBoundary(choice)
			applied, ok := scope.Apply(prompt, preflight, choiceID)
			if !ok {
				http.Error(w, "invalid scope choice", http.StatusBadRequest)
				return
			}
			prompt = applied
			if !automatic {
				mode := agent.ScopeTaskMode(choiceID)
				scopeMode = &mode
			}
		} else if strings.TrimSpace(in.ScopeChoice) != "" {
			http.Error(w, "scope choice is not needed for this prompt", http.StatusBadRequest)
			return
		}
	}
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
			id, saveErr, taskErr := s.newSessionLocked()
			s.mu.Unlock()
			if taskErr != nil {
				writeGUIError(w, "couldn't load durable task state: "+taskErr.Error(), http.StatusInternalServerError)
				return
			}
			if saveErr != nil {
				s.emit(event{Type: "error", Text: fmt.Sprintf("couldn't save session: %v", saveErr)})
			}
			s.emitTaskSnapshot(id)
			s.emit(event{Type: "undo", Status: "cleared"})
			s.emit(event{Type: "task_mode", Text: string(agent.TaskAgent)})
			s.emit(event{Type: "prompts_refresh", Text: "all"})
			s.emit(event{Type: "system", Text: "cleared"})
		case "compact":
			s.mu.Lock()
			cfg := s.cfg
			ag := s.ag
			budget := ctxmgr.BudgetForModel(cfg.Model)
			var client llm.Client
			if ag != nil {
				client = ag.ClientSnapshot()
			}
			compact, stats, err := ctxmgr.Manage(context.Background(), client, cfg.Model, s.hist, budget)
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
		case "undo":
			s.mu.Lock()
			if s.busy || s.activeTurns > 0 {
				s.mu.Unlock()
				s.emit(event{Type: "error", Text: "cannot undo while a turn is running; wait for it to finish or stop it first"})
				break
			}
			if s.ag == nil {
				s.mu.Unlock()
				s.emit(event{Type: "error", Text: "agent not ready"})
				break
			}
			id := s.sessionID
			text, err := s.ag.UndoLastTurn()
			s.mu.Unlock()
			if err != nil {
				s.emit(event{Type: "error", Text: err.Error()})
			} else {
				s.emitTaskSnapshot(id)
				s.emit(event{Type: "system", Text: text})
				s.emit(event{Type: "undo", Status: "cleared", Text: text})
			}
		case "status":
			s.mu.Lock()
			cfg := s.cfg
			ag := s.ag
			s.mu.Unlock()
			mode := agent.TaskAgent
			goalText := ""
			if ag != nil {
				mode = ag.TaskModeSnapshot()
				goalText = ag.GoalSnapshot()
			}
			st := fmt.Sprintf("safe/fast=%s task=%s model=%s workspace=%s", cfg.Mode, mode.Label(), cfg.Model, cfg.Workspace)
			if goalText != "" {
				st += " · goal: " + goalText
			}
			if ag != nil && ag.Tools != nil && ag.Tools.HasMCP() {
				if mcp := ag.Tools.MCPManagerSnapshot(); mcp != nil {
					st += fmt.Sprintf(" · %d MCP tools", len(mcp.Tools()))
				}
			}
			s.emit(event{Type: "system", Text: st})
		case "diff":
			s.mu.Lock()
			workspace := s.cfg.Workspace
			s.mu.Unlock()
			s.emit(event{Type: "system", Text: slash.GitDiff(workspace)})
		case "goal:show":
			s.mu.Lock()
			workspace := s.cfg.Workspace
			s.mu.Unlock()
			g, _ := goal.Load(workspace)
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

	s.mu.Lock()
	if s.busy {
		queued := s.queueSteerScoped(prompt, parts, userPrompt, scopeMode, automaticScope, scopeNotice, scopeBoundary)
		s.mu.Unlock()
		if !queued {
			s.emit(event{Type: "error", Text: "follow-up queue is full; wait for the current turn to finish"})
			http.Error(w, "follow-up queue is full", http.StatusTooManyRequests)
			return
		}
		s.emit(event{Type: "system", Text: "Follow-up queued for when the current turn finishes"})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		response := map[string]any{"ok": true, "queued": true}
		if scopeNotice != "" {
			response["scope_notice"] = scopeNotice
		}
		_ = json.NewEncoder(w).Encode(response)
		return
	}
	admitted, admittedOK := s.admitAgentTurnLocked(scopeMode, false)
	if admittedOK {
		admitted.automaticScope = automaticScope
		admitted.scopeNotice = scopeNotice
		admitted.scopeBoundary = scopeBoundary
	}
	s.mu.Unlock()
	if !admittedOK {
		http.Error(w, "could not admit agent turn", http.StatusConflict)
		return
	}
	s.runAdmittedTurn(admitted, prompt, parts, userPrompt, scopeMode)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(202)
	response := map[string]any{"ok": true}
	if scopeNotice != "" {
		response["scope_notice"] = scopeNotice
	}
	_ = json.NewEncoder(w).Encode(response)
}

type guiHandler struct {
	s         *server
	prompt    string
	learn     learn.Store
	explore   int
	searches  int
	reads     int
	edits     int
	added     int
	removed   int
	changes   []event
	sessionID string
	turnGen   uint64
	permCh    chan perm.Decision
}

func newGUIHandlerAtWithPerm(s *server, sessionID string, turnGen uint64, permCh chan perm.Decision, workspace ...string) *guiHandler {
	ws := ""
	if len(workspace) > 0 {
		ws = workspace[0]
	}
	if ws == "" {
		s.mu.Lock()
		ws = s.cfg.Workspace
		s.mu.Unlock()
	}
	store, _ := learn.Load(ws)
	return &guiHandler{s: s, learn: store, sessionID: sessionID, turnGen: turnGen, permCh: permCh}
}

func (h *guiHandler) live() bool {
	if h == nil || h.s == nil {
		return false
	}
	// Keep small unit-test handlers useful while every production turn carries
	// both identity fields.  A partially tagged handler is never considered
	// live, which avoids accidentally blessing a stale callback.
	if h.sessionID == "" && h.turnGen == 0 {
		return true
	}
	h.s.mu.Lock()
	live := h.s.sessionID == h.sessionID && h.s.turnGen == h.turnGen
	h.s.mu.Unlock()
	return live
}

func (h *guiHandler) emit(e event) bool {
	if !h.live() {
		return false
	}
	if h.sessionID != "" && h.turnGen != 0 {
		e.SessionID = h.sessionID
		e.turnGen = h.turnGen
	}
	h.s.emit(e)
	return true
}

func (h *guiHandler) OnTaskState(task *taskstate.Task) {
	if task == nil || task.SessionID != h.sessionID {
		return
	}
	h.emit(event{Type: "task_progress", SessionID: h.sessionID, Task: task})
}

func (h *guiHandler) beginTurn(prompt string) {
	if !h.live() {
		return
	}
	h.prompt = prompt
	h.learn.RecordTurn()
	h.emit(event{Type: "think", Text: summarizePrompt(prompt), Kind: "plan", Status: "start"})
	h.emit(event{Type: "activity", Kind: "reset"})
}

func (h *guiHandler) endTurn(result agent.Result) {
	if !h.live() {
		return
	}
	if result.Context.Compacted && result.Context.Method != "" {
		h.emit(event{
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
		h.emit(event{
			Type:    "changes_summary",
			Text:    label,
			Count:   h.edits,
			Added:   h.added,
			Removed: h.removed,
			Status:  "done",
		})
	}
	if result.UndoAvailable {
		h.emit(event{Type: "undo", Status: "available", Available: true})
	} else if result.UndoError != "" {
		h.emit(event{Type: "undo", Status: "unavailable", Text: result.UndoError})
	}
	h.emit(event{Type: "think", Text: "Done", Kind: "plan", Status: "done"})
	if result.Verified != "" {
		status := verify.StatusFromEvidence(result.Verified)
		h.emit(event{
			Type:    "test",
			Text:    result.Verified,
			Summary: clip(result.Verified, 2000),
			Status:  verificationEventStatus(status),
			Kind:    "test",
		})
	}
	if !h.live() {
		return
	}
	_ = learn.Save(h.learn)
	h.emit(event{Type: "overview", Text: "refresh"})
	h.s.cleanupExtensionPool()
	h.s.reflectAfterTurn(h.prompt, result)
}

func verificationEventStatus(status verify.Status) string {
	switch status {
	case verify.StatusPass:
		return "pass"
	case verify.StatusFail:
		return "fail"
	case verify.StatusSkipped:
		return "skipped"
	default:
		return "inconclusive"
	}
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
	h.emit(event{Type: "assistant", Text: text})
}
func (h *guiHandler) OnTextDelta(delta string) {
	h.emit(event{Type: "assistant_delta", Text: delta})
}
func (h *guiHandler) OnTextFinal(text string) {
	h.emit(event{Type: "assistant_final", Text: text})
}
func (h *guiHandler) OnToolStart(call llm.ToolCall) {
	if !h.live() {
		return
	}
	h.learn.RecordTool(call.Name)

	switch call.Name {
	case "read_file":
		path := parseToolPath(call.Arguments)
		if path != "" {
			h.reads++
			h.s.noteTurnActivity("read")
			h.learn.RecordRead(path)
			h.emit(event{Type: "review", Path: path})
			h.emit(event{Type: "activity", Kind: "read", Count: h.reads, Path: path})
		}
	case "glob", "grep":
		h.searches++
		h.s.noteTurnActivity("search")
		h.learn.RecordSearch()
		h.emit(event{Type: "activity", Kind: "search", Count: h.searches})
	case "write_file", "edit_file":
		path := parseToolPath(call.Arguments)
		h.s.noteTurnActivity("edit")
		h.emit(event{Type: "think", Text: "Editing " + path, Kind: "edit", Status: "start", Path: path})
	case "bash":
		var in struct {
			Command string `json:"command"`
		}
		_ = json.Unmarshal([]byte(call.Arguments), &in)
		if isTestCommand(in.Command) {
			h.emit(event{Type: "think", Text: "Running tests…", Kind: "test", Status: "start"})
		}
	}
}
func (h *guiHandler) OnToolEnd(call llm.ToolCall, result string, err error) {
	if !h.live() {
		return
	}
	if err != nil {
		h.emit(event{Type: "error", Text: err.Error()})
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
			h.emit(event{
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
	if path == "" || !h.live() {
		return
	}
	h.edits++
	h.added += added
	h.removed += removed
	h.learn.RecordChange(path, added, removed)
	ev := event{Type: "change", Path: path, Added: added, Removed: removed, Status: "done"}
	h.changes = append(h.changes, ev)
	h.emit(ev)
	h.emit(event{Type: "activity", Kind: "edit", Count: h.edits, Added: h.added, Removed: h.removed})
}

func ternary(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

func (h *guiHandler) OnNeedPermission(ctx context.Context, req perm.Request) (perm.Decision, error) {
	if !h.live() {
		return perm.Deny, context.Canceled
	}
	h.s.mu.Lock()
	h.s.pendingPerm = req
	h.s.pendingPermGen = h.turnGen
	ag := h.s.ag
	h.s.mu.Unlock()
	if ag != nil {
		if traceLog := ag.TraceSnapshot(); traceLog != nil {
			_ = traceLog.Append("perm", req.Tool, req.Summary, nil, 0)
		}
	}
	h.emit(permissionEvent(req))
	permCh := h.permCh
	if permCh == nil {
		h.s.mu.Lock()
		permCh = h.s.permCh
		h.s.mu.Unlock()
	}
	if permCh == nil {
		return perm.Deny, errors.New("permission channel unavailable")
	}
	select {
	case <-ctx.Done():
		h.s.mu.Lock()
		if h.s.pendingPermGen == h.turnGen {
			h.s.pendingPerm = perm.Request{}
			h.s.pendingPermGen = 0
		}
		h.s.mu.Unlock()
		return perm.Deny, ctx.Err()
	case d := <-permCh:
		h.s.mu.Lock()
		if h.s.pendingPermGen == h.turnGen {
			h.s.pendingPerm = perm.Request{}
			h.s.pendingPermGen = 0
		}
		h.s.mu.Unlock()
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
	if err != nil {
		h.emit(event{Type: "error", Text: err.Error()})
	}
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
			id, saveErr, taskErr := s.newSessionLocked()
			s.mu.Unlock()
			if taskErr != nil {
				writeGUIError(w, "couldn't load durable task state: "+taskErr.Error(), http.StatusInternalServerError)
				return
			}
			if saveErr != nil {
				s.emit(event{Type: "error", Text: fmt.Sprintf("couldn't save session: %v", saveErr)})
			}
			s.invalidatePromptRecs()
			s.emitTaskSnapshot(id)
			s.emit(event{Type: "undo", Status: "cleared"})
			s.emit(event{Type: "task_mode", Text: string(agent.TaskAgent)})
			s.emit(event{Type: "prompts_refresh", Text: "all"})
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
			workspace := s.cfg.Workspace
			s.mu.Unlock()
			if !sessionWorkspaceMatches(sess, workspace) {
				http.Error(w, "session belongs to another workspace", http.StatusForbidden)
				return
			}
			s.mu.Lock()
			src := s.ag
			s.mu.Unlock()
			next, err := cloneAgentForSession(src, sess.ID)
			if err != nil {
				writeGUIError(w, err.Error(), http.StatusInternalServerError)
				return
			}
			s.mu.Lock()
			s.abortTurnLocked()
			s.sessionID = sess.ID
			s.hist = sess.Messages
			if next != nil {
				s.ag = next
			}
			ag := s.ag
			s.mu.Unlock()
			s.emitTaskSnapshot(sess.ID)
			s.emit(event{Type: "undo", Status: "cleared"})
			var task *taskstate.Task
			if ag != nil {
				task = ag.TaskSnapshot()
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":       sess.ID,
				"title":    sess.Title,
				"messages": messagesToTranscript(sess.Messages),
				"task":     task,
			})
		case "delete":
			if in.ID == "" {
				http.Error(w, "id required", 400)
				return
			}
			rotated := false
			var saveErr error
			var taskErr error
			s.mu.Lock()
			if s.sessionID == in.ID {
				_, saveErr, taskErr = s.newSessionLocked()
				rotated = taskErr == nil
			}
			currentID := s.sessionID
			s.mu.Unlock()
			if taskErr != nil {
				writeGUIError(w, "couldn't load durable task state: "+taskErr.Error(), http.StatusInternalServerError)
				return
			}
			if saveErr != nil {
				s.emit(event{Type: "error", Text: fmt.Sprintf("couldn't save session: %v", saveErr)})
			}
			s.emitTaskSnapshot(currentID)
			if rotated {
				s.emit(event{Type: "undo", Status: "cleared"})
			}
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

func sessionWorkspaceMatches(sess *session.Session, workspace string) bool {
	if sess == nil || strings.TrimSpace(sess.Workspace) == "" || strings.TrimSpace(workspace) == "" {
		return false
	}
	canonical := func(path string) string {
		abs, err := filepath.Abs(path)
		if err != nil {
			return ""
		}
		abs = filepath.Clean(abs)
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			abs = filepath.Clean(resolved)
		}
		return abs
	}
	return canonical(sess.Workspace) == canonical(workspace)
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
	resolved, err := perm.ResolveWorkspacePath(ws, rel)
	if err != nil || resolved.OutsideWorkspace {
		http.Error(w, "outside workspace", 403)
		return
	}
	wsAbs := resolved.Root
	abs := resolved.Path
	f, err := os.Open(abs)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	defer f.Close()
	const maxPreviewBytes = 256 << 10
	data, err := io.ReadAll(io.LimitReader(f, maxPreviewBytes+utf8.UTFMax))
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	truncatedBytes := len(data) > maxPreviewBytes
	if truncatedBytes {
		data = data[:maxPreviewBytes]
		data = trimIncompleteUTF8Prefix(data)
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
		"truncated": total > maxLines || truncatedBytes,
	})
}

func trimIncompleteUTF8Prefix(data []byte) []byte {
	if utf8.Valid(data) {
		return data
	}
	start := len(data) - utf8.UTFMax + 1
	if start < 0 {
		start = 0
	}
	for n := len(data) - 1; n >= start; n-- {
		if utf8.Valid(data[:n]) {
			return data[:n]
		}
	}
	return data
}

func (s *server) settings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.Lock()
		cfg := s.cfg
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"workspace":                 cfg.Workspace,
			"mode":                      cfg.PersistentMode(),
			"active_mode":               cfg.Mode,
			"mode_overridden":           cfg.ModeOverridden(),
			"model":                     cfg.DisplayModel(),
			"provider":                  cfg.Provider,
			"max_tool_rounds":           cfg.MaxToolRounds,
			"llm_timeout_sec":           cfg.LLMTimeoutSec,
			"bash_timeout_sec":          cfg.BashTimeoutSec,
			"has_api_key":               cfg.APIKeyResolved() != "",
			"has_anthropic_key":         cfg.AnthropicKeyResolved() != "",
			"codex_cli":                 codexauth.LoggedIn(),
			"claude_cli":                claudeauth.LoggedIn(),
			"opencode_cli":              opencodeauth.LoggedIn(),
			"antigravity_cli":           agyauth.LoggedIn(),
			"codex":                     cfg.Provider == config.ProviderCodex && codexauth.LoggedIn(),
			"auth":                      setup.ProviderAuthPrompt(cfg),
			"model_options":             llm.ModelChoices(llm.Ecosystem(cfg.RouterEcosystem()), cfg.FableAllowed()),
			"model_options_codex":       llm.ModelChoices(llm.EcoCodex, false),
			"model_options_quadcode":    llm.ModelChoices(llm.EcoQuadCode, cfg.FableAllowed()),
			"model_options_opencode":    llm.ModelChoices(llm.EcoOpenCode, false),
			"model_options_antigravity": llm.ModelChoices(llm.EcoAntigravity, false),
			"router":                    s.routerSnapshot(),
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
		s.configTxMu.Lock()
		defer s.configTxMu.Unlock()
		s.mu.Lock()
		cfg := s.cfg
		s.mu.Unlock()
		workspaceChanged := in.Workspace != "" && in.Workspace != cfg.Workspace
		if in.Workspace != "" {
			cfg.Workspace = in.Workspace
		}
		if mode := config.Mode(in.Mode); mode.Valid() {
			cfg.SetUserMode(mode)
		}
		if in.Model != "" {
			if in.Model == config.ModelAuto {
				switch cfg.Provider {
				case config.ProviderOpenCode, config.ProviderAntigravity, config.ProviderOllama, config.ProviderOpenAI:
					cfg.Model = ""
					cfg.Model = cfg.BackendModel()
					cfg.Router.Enabled = false
				default:
					cfg.Model = config.ModelAuto
					cfg.Router.Enabled = true
				}
			} else {
				cfg.Model = in.Model
				cfg.Router.Enabled = false
			}
		}
		if p := config.Provider(strings.ToLower(in.Provider)); in.Provider != "" {
			switch p {
			case config.ProviderCodex, config.ProviderQuadCode, config.ProviderOpenCode, config.ProviderAntigravity, config.ProviderOpenAI, config.ProviderOllama:
				cfg.Provider = p
				// Drop Auto when switching to providers without a router.
				if (p == config.ProviderOpenCode || p == config.ProviderAntigravity || p == config.ProviderOllama || p == config.ProviderOpenAI) &&
					(cfg.Model == "" || cfg.Model == config.ModelAuto) {
					cfg.Model = cfg.BackendModel()
					cfg.Router.Enabled = false
				}
			}
		}
		if in.AnthropicKey != "" {
			cfg.AnthropicKey = in.AnthropicKey
		}
		if in.RouterEnabled != nil {
			cfg.Router.Enabled = *in.RouterEnabled
		}
		if in.UseLLMAdvisor != nil {
			cfg.Router.UseLLMAdvisor = *in.UseLLMAdvisor
		}
		if in.AllowFable != nil {
			cfg.Router.AllowFable = *in.AllowFable
		}
		if in.FableConfirmed != nil {
			cfg.Router.FableConfirmed = *in.FableConfirmed
		}
		if in.RefreshCatalog {
			llm.InitCatalog(true)
			llm.RefreshCLIModels(true)
		}
		if in.MaxToolRounds > 0 {
			cfg.MaxToolRounds = in.MaxToolRounds
		}
		if in.LLMTimeoutSec > 0 {
			cfg.LLMTimeoutSec = in.LLMTimeoutSec
		}
		if in.BashTimeoutSec > 0 {
			cfg.BashTimeoutSec = in.BashTimeoutSec
		}
		rebuildAgent := workspaceChanged || in.Provider != "" || in.RouterEnabled != nil || in.UseLLMAdvisor != nil || in.AllowFable != nil || in.FableConfirmed != nil || in.AnthropicKey != ""
		var nextAgent *agent.Agent
		var nextSession string
		var nextHistory []llm.Message
		if rebuildAgent {
			built, err := app.Build(cfg)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			nextAgent = built
			if workspaceChanged {
				nextSession, nextHistory = initialSession(cfg.Workspace)
				if err := nextAgent.SetTaskSession(nextSession); err != nil {
					closeCandidateAgent(nextAgent)
					writeGUIError(w, "couldn't load durable task state: "+err.Error(), http.StatusInternalServerError)
					return
				}
			} else {
				s.mu.Lock()
				currentSession := s.sessionID
				s.mu.Unlock()
				if err := nextAgent.SetTaskSession(currentSession); err != nil {
					closeCandidateAgent(nextAgent)
					writeGUIError(w, "couldn't load durable task state: "+err.Error(), http.StatusInternalServerError)
					return
				}
			}
		}
		// Persist before publishing. The configuration transaction mutex is also
		// held by /api/mode, so Settings and direct mode changes cannot reorder
		// their saved config. A failed write changes neither the live agent nor
		// its permission gate.
		if err := s.persistConfig(cfg); err != nil {
			closeCandidateAgent(nextAgent)
			http.Error(w, "couldn't save settings: "+err.Error(), http.StatusInternalServerError)
			return
		}
		var sessionSaveErr error
		s.mu.Lock()
		if workspaceChanged {
			oldWorkspace := s.cfg.Workspace
			oldSession := s.sessionID
			oldHistory := s.hist
			s.abortTurnLocked()
			if oldSession != "" && len(oldHistory) > 0 {
				sessionSaveErr = session.SaveMessages(oldWorkspace, oldSession, oldHistory)
			}
			s.cfg = cfg
			s.ag = nextAgent
			s.hist = nextHistory
			s.sessionID = nextSession
		} else {
			s.cfg = cfg
			if nextAgent != nil {
				s.ag = nextAgent
			} else if s.ag != nil {
				s.ag.UpdateConfig(func(current *config.Config) { *current = cfg })
			}
		}
		s.mu.Unlock()
		if sessionSaveErr != nil {
			s.emit(event{Type: "error", Text: fmt.Sprintf("couldn't save session: %v", sessionSaveErr)})
		}
		if workspaceChanged {
			s.attachRouterHook()
			_, _, _ = projects.Ensure(cfg.Workspace)
			s.emit(event{Type: "undo", Status: "cleared"})
			s.emit(event{Type: "system", Text: "Opened " + projects.NameFromPath(cfg.Workspace)})
			s.emitTaskSnapshot(nextSession)
			s.emit(event{Type: "overview", Text: "refresh"})
			s.invalidatePromptRecs()
			s.emit(event{Type: "prompts_refresh", Text: "all"})
		} else if nextAgent != nil {
			s.attachRouterHook()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":              true,
			"persisted":       true,
			"model":           cfg.DisplayModel(),
			"mode":            cfg.PersistentMode(),
			"active_mode":     cfg.Mode,
			"mode_overridden": cfg.ModeOverridden(),
		})
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
