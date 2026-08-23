package gui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/goal"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/session"
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

func TestGUIOnlyAcceptsLoopbackListenAddresses(t *testing.T) {
	for _, tc := range []struct {
		addr string
		want bool
	}{
		{addr: "127.0.0.1:7420", want: true},
		{addr: "[::1]:7420", want: true},
		{addr: "localhost:7420", want: true},
		{addr: "0.0.0.0:7420", want: false},
		{addr: ":7420", want: false},
		{addr: "192.168.1.20:7420", want: false},
	} {
		if got := loopbackListenAddress(tc.addr); got != tc.want {
			t.Errorf("loopbackListenAddress(%q) = %v, want %v", tc.addr, got, tc.want)
		}
	}
}

func TestFollowUpQueuePreservesFIFOAndBounds(t *testing.T) {
	s := &server{}
	mode := agent.TaskPlan
	if !s.queueSteerMode("first", []llm.Part{{Type: "text", Text: "one"}}, "first display", &mode) {
		t.Fatal("first follow-up was rejected")
	}
	if !s.queueSteer("second", nil, "second display") {
		t.Fatal("second follow-up was rejected")
	}
	for i := 2; i < maxQueuedTurns; i++ {
		if !s.queueSteer(fmt.Sprintf("queued-%d", i), nil, "") {
			t.Fatalf("follow-up %d was rejected before queue bound", i)
		}
	}
	if s.queueSteer("overflow", nil, "") {
		t.Fatal("follow-up queue accepted an entry past its bound")
	}

	prompt, parts, display, gotMode, ok := s.popSteer()
	if !ok || prompt != "first" || display != "first display" || len(parts) != 1 || gotMode == nil || *gotMode != mode {
		t.Fatalf("first queued turn = %q, %#v, %q, %#v, %v", prompt, parts, display, gotMode, ok)
	}
	prompt, _, display, gotMode, ok = s.popSteer()
	if !ok || prompt != "second" || display != "second display" || gotMode != nil {
		t.Fatalf("second queued turn = %q, %q, %#v, %v", prompt, display, gotMode, ok)
	}
}

func TestQueuedFollowUpCannotBeLostDuringDoneHandoff(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	scripted := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", Content: "first done"}},
		{Message: llm.Message{Role: "assistant", Content: "second done"}},
		{Message: llm.Message{Role: "assistant", Content: "third done"}},
	}}
	ag := agent.New(cfg, scripted, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	events := make(chan event)
	s := &server{
		cfg:       cfg,
		ag:        ag,
		sessionID: "handoff-session",
		permCh:    make(chan perm.Decision, 1),
		subs:      []chan event{events},
	}
	if err := (&session.Session{ID: "handoff-session", Title: "Pinned", Workspace: workspace}).Save(); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	s.beforeAgentRun = func() {
		s.mu.Lock()
		if s.ag != nil {
			s.ag.SetClient(scripted)
		}
		s.mu.Unlock()
		first := false
		once.Do(func() {
			first = true
			close(entered)
		})
		if first {
			<-release
		}
	}
	finalDone := make(chan struct{})
	go func() {
		doneCount := 0
		for e := range events {
			if e.Type != "done" {
				continue
			}
			if doneCount == 0 {
				doneCount++
				// A competing request is allowed to arrive exactly when the
				// browser observes done. A correct handoff keeps the queued
				// second turn ahead of this third request.
				go s.startAgentTurn("third", nil)
				continue
			}
			close(finalDone)
			return
		}
	}()

	s.startAgentTurn("first", nil)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first turn did not reach its barrier")
	}
	if !s.queueSteer("second", nil, "second") {
		t.Fatal("failed to queue second turn")
	}
	close(release)

	select {
	case <-finalDone:
	case <-time.After(15 * time.Second):
		s.mu.Lock()
		t.Fatalf("turn handoff did not finish: busy=%v active=%d queue=%d hist=%#v calls=%d", s.busy, s.activeTurns, len(s.steerQueue), s.hist, len(scripted.Calls))
		s.mu.Unlock()
	}
	s.mu.Lock()
	hist := append([]llm.Message(nil), s.hist...)
	queueLen := len(s.steerQueue)
	active := s.activeTurns
	s.mu.Unlock()
	if queueLen != 0 || active != 0 {
		t.Fatalf("handoff state queue=%d active=%d", queueLen, active)
	}
	var users []string
	for _, msg := range hist {
		if msg.Role == "user" {
			users = append(users, msg.Content)
		}
	}
	if !strings.Contains(strings.Join(users, "\n"), "second") || !strings.Contains(strings.Join(users, "\n"), "third") {
		t.Fatalf("queued turns were not preserved: users=%v", users)
	}
}

func TestCanceledTurnCannotPersistInferredState(t *testing.T) {
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	ag := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	s := &server{cfg: cfg, ag: ag, sessionID: "canceled", turnGen: 2}
	s.autoApplyFromUserPrompt("fix all failing tests", 1)
	if got := ag.GoalSnapshot(); got != "" {
		t.Fatalf("stale canceled turn applied goal %q", got)
	}
	if got, _ := goal.Load(workspace); got != "" {
		t.Fatalf("stale canceled turn persisted goal %q", got)
	}
}

func TestSessionLoadRequiresCurrentWorkspace(t *testing.T) {
	current := t.TempDir()
	other := t.TempDir()
	if !sessionWorkspaceMatches(&session.Session{Workspace: current}, current) {
		t.Fatal("matching workspace was rejected")
	}
	if sessionWorkspaceMatches(&session.Session{Workspace: other}, current) {
		t.Fatal("foreign workspace was accepted")
	}
	if sessionWorkspaceMatches(&session.Session{Workspace: ""}, current) {
		t.Fatal("empty session workspace was accepted")
	}
}

func TestScopeAPIExplainsBroadPrompt(t *testing.T) {
	s := &server{}
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/scope", strings.NewReader(`{"prompt":"build something"}`))
	s.scopeAPI(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.Code)
	}
	var body struct {
		Needed   bool             `json:"needed"`
		Question string           `json:"question"`
		Choices  []map[string]any `json:"choices"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Needed || body.Question == "" || len(body.Choices) != 3 {
		t.Fatalf("scope response = %#v", body)
	}
	if got, _ := body.Choices[0]["recommended"].(bool); !got {
		t.Fatal("first choice is not marked recommended")
	}
}

func TestScopeAPILeavesSpecificPromptAlone(t *testing.T) {
	s := &server{}
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/scope", strings.NewReader(`{"prompt":"fix internal/auth/login.go"}`))
	s.scopeAPI(res, req)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), `"needed":false`) {
		t.Fatalf("status/body = %d/%s", res.Code, res.Body.String())
	}
}

func TestChatRequiresScopeChoiceForBroadPrompt(t *testing.T) {
	s := &server{cfg: config.Config{Workspace: t.TempDir()}}
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"prompt":"build something"}`))
	s.chat(res, req)
	if res.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusConflict)
	}
	if !strings.Contains(res.Body.String(), `"error":"scope_required"`) {
		t.Fatalf("body = %s", res.Body.String())
	}
}

func TestChatAppliesScopeAndKeepsTranscriptReadable(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	ag := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "done"}}}}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	s := &server{
		cfg:       cfg,
		ag:        ag,
		sessionID: "scope-session",
		permCh:    make(chan perm.Decision, 1),
	}
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"prompt":"build something","scope_choice":"small"}`))
	s.chat(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusAccepted)
	}
	// The first GUI turn may have to initialize trace/extension state on a
	// slower Windows or macOS runner; allow startup variance without changing
	// the actual turn contract.
	deadline := time.Now().Add(10 * time.Second)
	for {
		s.mu.Lock()
		active := s.activeTurns
		hist := append([]llm.Message(nil), s.hist...)
		s.mu.Unlock()
		if active == 0 {
			if len(hist) < 2 {
				t.Fatalf("history = %#v", hist)
			}
			if hist[len(hist)-2].Role != "user" || hist[len(hist)-2].Content != "build something" {
				t.Fatalf("saved user message leaked scope guidance: %#v", hist[len(hist)-2])
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("scoped turn did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestScopedTurnReportsTemporaryTaskModeAndRestoresIt(t *testing.T) {
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	events := make(chan event, 32)
	ag := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "done"}}}}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	s := &server{
		cfg:       cfg,
		ag:        ag,
		liveTask:  agent.TaskAgent,
		sessionID: "temporary-mode",
		permCh:    make(chan perm.Decision, 1),
		subs:      []chan event{events},
	}
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"prompt":"build something","scope_choice":"plan"}`))
	s.chat(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusAccepted)
	}
	// Keep this asynchronous boundary tolerant of first-run runtime startup;
	// the test still fails if the turn genuinely never unwinds.
	deadline := time.Now().Add(10 * time.Second)
	for {
		s.mu.Lock()
		active := s.activeTurns
		s.mu.Unlock()
		if active == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("scoped turn did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
	foundPlan, foundAgent := false, false
	for {
		select {
		case e := <-events:
			if e.Type != "task_mode" {
				continue
			}
			foundPlan = foundPlan || e.Text == string(agent.TaskPlan)
			foundAgent = foundAgent || e.Text == string(agent.TaskAgent)
		default:
			if !foundPlan || !foundAgent {
				t.Fatalf("task mode events did not show temporary plan and restore: plan=%v agent=%v", foundPlan, foundAgent)
			}
			return
		}
	}
}

func TestQueuedScopedTurnDoesNotPersistGoalBeforeAdmission(t *testing.T) {
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	ag := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	s := &server{
		cfg:         cfg,
		ag:          ag,
		busy:        true,
		activeTurns: 1,
		sessionID:   "queued-scope",
		permCh:      make(chan perm.Decision, 1),
	}
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"prompt":"fix all","scope_choice":"focused"}`))
	s.chat(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusAccepted)
	}
	if got := ag.GoalSnapshot(); got != "" {
		t.Fatalf("queued scope inferred goal %q before admission", got)
	}
	if got, _ := goal.Load(workspace); got != "" {
		t.Fatalf("queued scope persisted goal %q before admission", got)
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

func TestReadFileRejectsOutsideSymlink(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "escape")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("symlink creation requires privileges on Windows")
		}
		t.Fatal(err)
	}
	s := &server{cfg: config.Config{Workspace: workspace}}
	res := httptest.NewRecorder()
	s.readFile(res, httptest.NewRequest(http.MethodGet, "/api/file?path=escape%2Fsecret.txt", nil))
	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusForbidden)
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

func TestGUIHandlerDropsStaleTurnCallbacks(t *testing.T) {
	events := make(chan event, 8)
	s := &server{subs: []chan event{events}, sessionID: "session-live", turnGen: 2}
	stale := &guiHandler{s: s, sessionID: "session-live", turnGen: 1, permCh: make(chan perm.Decision, 1)}
	stale.OnText("old assistant text")
	stale.OnTextDelta("old delta")
	stale.OnTextFinal("old final")
	stale.OnToolStart(llm.ToolCall{Name: "read_file", Arguments: `{"path":"old.go"}`})
	stale.OnError(errors.New("old error"))
	if decision, err := stale.OnNeedPermission(context.Background(), perm.Request{Tool: "write_file"}); decision != perm.Deny || !errors.Is(err, context.Canceled) {
		t.Fatalf("stale permission = %s, %v; want canceled denial", decision, err)
	}
	select {
	case got := <-events:
		t.Fatalf("stale callback emitted %#v", got)
	default:
	}

	live := &guiHandler{s: s, sessionID: "session-live", turnGen: 2, permCh: make(chan perm.Decision, 1)}
	live.OnText("current assistant text")
	select {
	case got := <-events:
		if got.Type != "assistant" || got.Text != "current assistant text" || got.SessionID != "session-live" || got.turnGen != 2 {
			t.Fatalf("live callback event = %#v", got)
		}
	default:
		t.Fatal("live callback did not emit")
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
	// The full cross-platform package run can spend several seconds starting a
	// provider-backed turn on Windows before it reaches this test barrier.
	select {
	case <-beforeRun:
	case <-time.After(10 * time.Second):
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

	deadline := time.Now().Add(10 * time.Second)
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

func TestSettingsWorkspaceChangeRebuildsProjectRuntime(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	oldWorkspace := t.TempDir()
	newWorkspace := t.TempDir()
	store := taskstate.NewStore(t.TempDir())
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = oldWorkspace
	ag := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "old turn"}}}}, tools.NewRegistry(tools.Context{Workspace: oldWorkspace}), perm.New(config.ModeFast, oldWorkspace, nil))
	ag.SetTaskStore(store)
	ag.SetTaskSession("old-session")
	oldTask, err := taskstate.New("old-session", "finish the old workspace task", []string{"implement"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(oldTask); err != nil {
		t.Fatal(err)
	}
	ag.SetTaskSession("old-session")
	entered := make(chan struct{})
	release := make(chan struct{})
	s := &server{
		cfg:       cfg,
		ag:        ag,
		sessionID: "old-session",
		permCh:    make(chan perm.Decision, 1),
		beforeAgentRun: func() {
			close(entered)
			<-release
		},
	}
	s.startAgentTurn("work in the old workspace", nil)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("old turn did not reach its barrier")
	}

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(fmt.Sprintf(`{"workspace":%q}`, newWorkspace)))
	s.settings(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("settings status = %d, body=%s", res.Code, res.Body.String())
	}
	s.mu.Lock()
	currentCfg := s.cfg
	currentAgent := s.ag
	currentSession := s.sessionID
	s.mu.Unlock()
	if currentCfg.Workspace != newWorkspace {
		t.Fatalf("workspace = %q, want %q", currentCfg.Workspace, newWorkspace)
	}
	if currentAgent == nil || currentAgent == ag {
		t.Fatal("settings workspace change did not replace the agent")
	}
	if currentSession == "old-session" || currentAgent.TaskSession == "old-session" {
		t.Fatalf("workspace switch retained old session: server=%q agent=%q", currentSession, currentAgent.TaskSession)
	}
	if got := currentAgent.TaskSnapshot(); got != nil {
		t.Fatalf("new workspace inherited old durable task: %#v", got)
	}

	close(release)
	deadline := time.Now().Add(10 * time.Second)
	for {
		s.mu.Lock()
		active := s.activeTurns
		s.mu.Unlock()
		if active == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("stale old-workspace turn did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got, err := store.Load("old-session"); err != nil || got == nil {
		t.Fatalf("old durable task was lost: task=%#v err=%v", got, err)
	}
}
