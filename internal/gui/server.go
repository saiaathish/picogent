package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/app"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
)

type event struct {
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type server struct {
	cfg    config.Config
	ag     *agent.Agent
	mu     sync.Mutex
	hist   []llm.Message
	permCh chan perm.Decision
	subs   []chan event
}

func Run() error {
	cfg, a, err := app.Load(".")
	if err != nil {
		return err
	}
	s := &server{cfg: cfg, ag: a, permCh: make(chan perm.Decision, 1)}
	static, err := fs.Sub(webFS, "web")
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(static)))
	mux.HandleFunc("/api/state", s.state)
	mux.HandleFunc("/api/chat", s.chat)
	mux.HandleFunc("/api/permission", s.permission)
	mux.HandleFunc("/api/mode", s.setMode)
	mux.HandleFunc("/api/events", s.events)

	ln, err := net.Listen("tcp", "127.0.0.1:7420")
	if err != nil {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return err
		}
	}
	url := "http://" + ln.Addr().String() + "/"
	fmt.Println("picogent gui", url)
	go openBrowser(url)
	return http.Serve(ln, mux)
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

func (s *server) state(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"mode":      s.cfg.Mode,
		"model":     s.cfg.Model,
		"workspace": s.cfg.Workspace,
		"provider":  s.cfg.Provider,
	})
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

func (s *server) permission(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Allow bool   `json:"allow"`
		Turn  bool   `json:"turn"`
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
	ch := make(chan event, 16)
	s.mu.Lock()
	s.subs = append(s.subs, ch)
	s.mu.Unlock()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			b, _ := json.Marshal(e)
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		}
	}
}

func (s *server) chat(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.mu.Lock()
	hist := s.hist
	s.mu.Unlock()
	h := &guiHandler{s: s}
	next, _, err := s.ag.Run(r.Context(), hist, in.Prompt, h)
	s.mu.Lock()
	s.hist = next
	s.mu.Unlock()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.WriteHeader(204)
}

type guiHandler struct{ s *server }

func (h *guiHandler) OnText(text string) {
	h.s.emit(event{Type: "assistant", Text: text})
}
func (h *guiHandler) OnToolStart(call llm.ToolCall) {
	h.s.emit(event{Type: "tool", Text: "→ " + call.Name})
}
func (h *guiHandler) OnToolEnd(_ llm.ToolCall, result string, err error) {
	if err != nil {
		h.s.emit(event{Type: "error", Text: err.Error()})
		return
	}
	h.s.emit(event{Type: "tool", Text: result})
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
