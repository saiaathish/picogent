package gui

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/tools"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/taskstate"
)

func TestTaskProgressClearEventKeepsNullTaskEnvelope(t *testing.T) {
	raw, err := json.Marshal(event{Type: "task_progress", SessionID: "session-current"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"task":null`) {
		t.Fatalf("clear event = %s", raw)
	}
}

func TestChatRejectsUndoWhileAgentTurnIsActive(t *testing.T) {
	tests := []struct {
		name        string
		busy        bool
		activeTurns int
	}{
		{name: "busy", busy: true, activeTurns: 1},
		{name: "cancelled but still exiting", activeTurns: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := make(chan event, 1)
			s := &server{
				busy:        tt.busy,
				activeTurns: tt.activeTurns,
				subs:        []chan event{events},
			}
			req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"prompt":"/undo"}`))
			res := httptest.NewRecorder()

			s.chat(res, req)

			if res.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d", res.Code, http.StatusNoContent)
			}
			select {
			case got := <-events:
				if got.Type != "error" {
					t.Fatalf("event type = %q, want error", got.Type)
				}
				if !strings.Contains(got.Text, "cannot undo while a turn is running") {
					t.Fatalf("event text = %q, want active-turn explanation", got.Text)
				}
			default:
				t.Fatal("expected active-turn error event")
			}
		})
	}
}

func TestGUIHandlerEmitsCanonicalFinalTextReplacement(t *testing.T) {
	events := make(chan event, 1)
	s := &server{subs: []chan event{events}}
	h := &guiHandler{s: s}
	h.OnTextFinal("Undo: /undo")

	got := <-events
	if got.Type != "assistant_final" || got.Text != "Undo: /undo" {
		t.Fatalf("event = %+v", got)
	}
}

func TestSnapshotIncludesDurableTaskForCurrentSession(t *testing.T) {
	store := taskstate.NewStore(t.TempDir())
	task, err := taskstate.New("session-current", "finish the loop", []string{"implement", "verify"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}
	ag := &agent.Agent{TaskStore: store}
	ag.SetTaskSession("session-current")
	s := &server{ag: ag, sessionID: "session-current"}

	got, ok := s.snapshot()["task"].(*taskstate.Task)
	if !ok || got == nil || got.ID != task.ID {
		t.Fatalf("snapshot task = %#v", s.snapshot()["task"])
	}
}

func TestGUIHandlerDropsStaleTaskProgress(t *testing.T) {
	events := make(chan event, 1)
	s := &server{subs: []chan event{events}, sessionID: "session-live", turnGen: 2}
	task, err := taskstate.New("session-live", "finish the loop", []string{"implement"})
	if err != nil {
		t.Fatal(err)
	}
	stale := &guiHandler{s: s, sessionID: "session-live", turnGen: 1}
	stale.OnTaskState(task)
	select {
	case got := <-events:
		t.Fatalf("stale handler emitted %#v", got)
	default:
	}

	live := &guiHandler{s: s, sessionID: "session-live", turnGen: 2}
	live.OnTaskState(task)
	got := <-events
	if got.Type != "task_progress" || got.SessionID != "session-live" || got.Task == nil || got.Task.ID != task.ID {
		t.Fatalf("event = %#v", got)
	}
}

func TestClearRotatesDurableTaskSession(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	store := taskstate.NewStore(t.TempDir())
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	reg := tools.NewRegistry(tools.Context{Workspace: workspace})
	ag := agent.New(cfg, &llm.Scripted{}, reg, perm.New(config.ModeFast, workspace, nil))
	ag.TaskStore = store
	ag.SetTaskSession("session-before-clear")
	task, err := taskstate.New("session-before-clear", "finish the loop", []string{"implement"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}
	s := &server{
		cfg:       cfg,
		ag:        ag,
		sessionID: "session-before-clear",
		hist:      []llm.Message{{Role: "user", Content: "old"}},
		liveTask:  agent.TaskAgent,
		permCh:    make(chan perm.Decision, 1),
	}

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"prompt":"/clear"}`))
	s.chat(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusNoContent)
	}
	s.mu.Lock()
	newID := s.sessionID
	newAgent := s.ag
	histLen := len(s.hist)
	s.mu.Unlock()
	if newID == "session-before-clear" {
		t.Fatal("clear reused the durable task session id")
	}
	if newAgent == nil || newAgent == ag {
		t.Fatal("clear did not replace the agent instance")
	}
	if newAgent.TaskSession != newID {
		t.Fatalf("agent task session = %q, want %q", newAgent.TaskSession, newID)
	}
	if histLen != 0 {
		t.Fatalf("history length after clear = %d, want 0", histLen)
	}
	if got := newAgent.TaskSnapshot(); got != nil {
		t.Fatalf("new session inherited task state: %#v", got)
	}
	if got, err := store.Load("session-before-clear"); err != nil || got == nil {
		t.Fatalf("old session task was lost: task=%#v err=%v", got, err)
	}
}

func TestStaleTurnUsesCapturedAgentSessionAfterReset(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	store := taskstate.NewStore(t.TempDir())
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	reg := tools.NewRegistry(tools.Context{Workspace: workspace})
	ag := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "done"}}}}, reg, perm.New(config.ModeFast, workspace, nil))
	ag.TaskStore = store
	ag.SetTaskSession("session-before-reset")
	beforeRun := make(chan struct{})
	release := make(chan struct{})
	s := &server{
		cfg:       cfg,
		ag:        ag,
		sessionID: "session-before-reset",
		liveTask:  agent.TaskAgent,
		permCh:    make(chan perm.Decision, 1),
		beforeAgentRun: func() {
			close(beforeRun)
			<-release
		},
	}

	s.startAgentTurn("fix the broken signup flow", nil)
	select {
	case <-beforeRun:
	case <-time.After(2 * time.Second):
		t.Fatal("turn did not reach the pre-run barrier")
	}
	res := httptest.NewRecorder()
	s.reset(res, httptest.NewRequest(http.MethodPost, "/api/reset", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("reset status = %d, want %d", res.Code, http.StatusOK)
	}
	var resetBody map[string]string
	if err := json.Unmarshal(res.Body.Bytes(), &resetBody); err != nil {
		t.Fatal(err)
	}
	newID := resetBody["id"]
	if newID == "" || newID == "session-before-reset" {
		t.Fatalf("reset id = %q", newID)
	}
	close(release)

	deadline := time.Now().Add(2 * time.Second)
	for {
		s.mu.Lock()
		active := s.activeTurns
		s.mu.Unlock()
		if active == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("stale turn did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, err := store.Load(newID); !errors.Is(err, taskstate.ErrNotFound) {
		t.Fatalf("new session received stale task state: err=%v", err)
	}
	if got, err := store.Load("session-before-reset"); err != nil || got == nil {
		t.Fatalf("captured session task was not persisted: task=%#v err=%v", got, err)
	}
	s.mu.Lock()
	currentAgent := s.ag
	s.mu.Unlock()
	if currentAgent == ag {
		t.Fatal("reset did not replace the agent used by the stale turn")
	}
	if currentAgent.TaskSession != newID {
		t.Fatalf("current agent task session = %q, want %q", currentAgent.TaskSession, newID)
	}
}
