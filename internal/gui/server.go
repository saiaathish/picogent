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
	"github.com/saiaathish/picogent/internal/codexauth"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/session"
	"github.com/saiaathish/picogent/internal/setup"
	"github.com/saiaathish/picogent/internal/slash"
)

type event struct {
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`
	Summary string `json:"summary,omitempty"`
	Path    string `json:"path,omitempty"`
	Line    int    `json:"line,omitempty"`
	LineEnd int    `json:"line_end,omitempty"`
}

type transcriptLine struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

type server struct {
	cfg       config.Config
	ag        *agent.Agent
	mu        sync.Mutex
	hist      []llm.Message
	sessionID string
	permCh    chan perm.Decision
	subs      []chan event
	busy      bool
	cancel    context.CancelFunc
}

func Run() error {
	cfg, a, err := app.Load(".")
	if err != nil {
		return err
	}
	s := &server{cfg: cfg, ag: a, permCh: make(chan perm.Decision, 1), sessionID: session.New(cfg.Workspace).ID}
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
	mux.HandleFunc("/api/cancel", s.cancelChat)
	mux.HandleFunc("/api/reset", s.reset)
	mux.HandleFunc("/api/sessions", s.sessions)
	mux.HandleFunc("/api/file", s.readFile)
	mux.HandleFunc("/api/settings", s.settings)
	mux.HandleFunc("/api/events", s.events)
	mux.Handle("/", http.FileServer(http.FS(static)))
	return mux
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

func (s *server) snapshot() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	hint := ""
	if err := s.cfg.MissingAuth(); err != nil {
		hint = err.Error()
	}
	return map[string]any{
		"mode":       s.cfg.Mode,
		"model":      s.cfg.Model,
		"workspace":  s.cfg.Workspace,
		"provider":   s.cfg.Provider,
		"codex":      s.cfg.Provider == config.ProviderCodex && codexauth.LoggedIn(),
		"busy":       s.busy,
		"hint":       hint,
		"setup":      !s.cfg.SetupComplete,
		"mcp_tools":  mcpToolCount(s.ag),
		"session_id": s.sessionID,
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
	mode := config.Mode(in.Mode)
	if !mode.Valid() {
		http.Error(w, "mode must be safe or fast", 400)
		return
	}
	s.mu.Lock()
	s.cfg.Mode = mode
	s.ag.CFG.Mode = mode
	s.ag.Gate.Mode = mode
	s.mu.Unlock()
	w.WriteHeader(204)
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
		Allow bool `json:"allow"`
		Turn  bool `json:"turn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	d := perm.Deny
	if in.Turn {
		d = perm.AllowTurn
	} else if in.Allow {
		d = perm.Allow
	}
	select {
	case s.permCh <- d:
	case <-time.After(2 * time.Second):
	}
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
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	prompt := strings.TrimSpace(in.Prompt)
	if prompt == "" {
		http.Error(w, "empty prompt", 400)
		return
	}

	kind, payload := slash.Resolve(s.cfg.Workspace, prompt)
	if kind == slash.Local {
		switch payload {
		case "clear":
			s.mu.Lock()
			s.hist = nil
			s.mu.Unlock()
			s.emit(event{Type: "system", Text: "cleared"})
		case "compact":
			s.mu.Lock()
			if len(s.hist) > 16 {
				head := s.hist[0]
				if head.Role != "system" {
					head = llm.Message{}
				}
				s.hist = append([]llm.Message{head}, s.hist[len(s.hist)-15:]...)
			}
			s.mu.Unlock()
			s.emit(event{Type: "system", Text: "context compacted"})
		case "status":
			st := fmt.Sprintf("mode=%s model=%s workspace=%s", s.cfg.Mode, s.cfg.Model, s.cfg.Workspace)
			if s.ag.Tools != nil && s.ag.Tools.HasMCP() {
				st += fmt.Sprintf(" · %d MCP tools", len(s.ag.Tools.MCP.Tools()))
			}
			s.emit(event{Type: "system", Text: st})
		case "diff":
			s.emit(event{Type: "system", Text: slash.GitDiff()})
		default:
			if strings.HasPrefix(payload, "memory:") {
				text := strings.TrimPrefix(payload, "memory:")
				if text == "" {
					text = "(no project rules files)"
				}
				s.emit(event{Type: "system", Text: text})
			}
		}
		w.WriteHeader(204)
		return
	}
	if kind == slash.Prompt {
		prompt = payload
	}

	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		http.Error(w, "already running", 409)
		return
	}
	s.busy = true
	hist := s.hist
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			s.busy = false
			s.cancel = nil
			s.mu.Unlock()
			s.emit(event{Type: "done"})
		}()
		h := &guiHandler{s: s}
		next, _, err := s.ag.Run(ctx, hist, prompt, h)
		s.mu.Lock()
		s.hist = next
		_ = session.SaveMessages(s.cfg.Workspace, s.sessionID, next)
		s.mu.Unlock()
		if err != nil && ctx.Err() == nil {
			s.emit(event{Type: "error", Text: err.Error()})
		}
	}()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(202)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

type guiHandler struct{ s *server }

func (h *guiHandler) OnText(text string) {
	h.s.emit(event{Type: "assistant", Text: text})
}
func (h *guiHandler) OnToolStart(call llm.ToolCall) {
	h.s.emit(event{Type: "tool", Text: "→ " + call.Name})
	if call.Name == "read_file" {
		var in struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal([]byte(call.Arguments), &in)
		if in.Path != "" {
			h.s.emit(event{Type: "review", Path: in.Path})
		}
	}
}
func (h *guiHandler) OnToolEnd(_ llm.ToolCall, result string, err error) {
	if err != nil {
		h.s.emit(event{Type: "error", Text: err.Error()})
		return
	}
	h.s.emit(event{Type: "tool", Text: clip(result, 400)})
}
func (h *guiHandler) OnNeedPermission(ctx context.Context, req perm.Request) (perm.Decision, error) {
	h.s.emit(event{Type: "permission", Summary: req.Summary})
	select {
	case <-ctx.Done():
		return perm.Deny, ctx.Err()
	case d := <-h.s.permCh:
		return d, nil
	}
}
func (h *guiHandler) OnError(err error) {
	h.s.emit(event{Type: "error", Text: err.Error()})
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
		"path":    display,
		"lines":   rows,
		"total":   total,
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
			"workspace":        cfg.Workspace,
			"mode":             cfg.Mode,
			"model":            cfg.Model,
			"provider":         cfg.Provider,
			"max_tool_rounds":  cfg.MaxToolRounds,
			"llm_timeout_sec":  cfg.LLMTimeoutSec,
			"bash_timeout_sec": cfg.BashTimeoutSec,
			"has_api_key":      cfg.APIKeyResolved() != "",
			"codex":            cfg.Provider == config.ProviderCodex && codexauth.LoggedIn(),
		})
	case http.MethodPost:
		var in struct {
			Workspace      string `json:"workspace"`
			Mode           string `json:"mode"`
			Model          string `json:"model"`
			MaxToolRounds  int    `json:"max_tool_rounds"`
			LLMTimeoutSec  int    `json:"llm_timeout_sec"`
			BashTimeoutSec int    `json:"bash_timeout_sec"`
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
			s.cfg.Model = in.Model
			s.ag.CFG.Model = in.Model
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
	default:
		return
	}
	_ = cmd.Start()
}
