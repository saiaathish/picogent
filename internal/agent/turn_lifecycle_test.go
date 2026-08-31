package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
)

type durableTurnProbeClient struct {
	store       *taskstate.Store
	sessionID   string
	response    llm.ChatResponse
	responses   []llm.ChatResponse
	err         error
	cancel      context.CancelFunc
	observed    chan *taskstate.TurnRecord
	observeOnce sync.Once
}

func (c *durableTurnProbeClient) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	task, err := c.store.Load(c.sessionID)
	if err != nil {
		return llm.ChatResponse{}, err
	}
	c.observeOnce.Do(func() { c.observed <- task.LastTurn() })
	if c.cancel != nil {
		c.cancel()
	}
	if len(c.responses) > 0 {
		response := c.responses[0]
		c.responses = c.responses[1:]
		return response, c.err
	}
	return c.response, c.err
}

func newDurableTurnAgent(t *testing.T, client llm.Client, store *taskstate.Store, sessionID string) *agent.Agent {
	t.Helper()
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, client, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	a.SetTaskStore(store)
	if err := a.SetTaskSession(sessionID); err != nil {
		t.Fatal(err)
	}
	return a
}

type turnCloseFailureHandler struct {
	allowAll
	ag       *agent.Agent
	badStore *taskstate.Store
	switched bool
}

func (h *turnCloseFailureHandler) OnTaskState(task *taskstate.Task) {
	if h.switched || task == nil || task.Status != taskstate.StatusDone {
		return
	}
	last := task.LastTurn()
	if last == nil || last.State != taskstate.TurnActive {
		return
	}
	h.switched = true
	h.ag.SetTaskStore(h.badStore)
}

type providerFailureCloseHandler struct {
	allowAll
	ag       *agent.Agent
	badStore *taskstate.Store
	switched bool
}

func (h *providerFailureCloseHandler) OnTaskState(task *taskstate.Task) {
	if h.switched || task == nil {
		return
	}
	last := task.LastTurn()
	if last == nil || last.State != taskstate.TurnActive {
		return
	}
	h.switched = true
	h.ag.SetTaskStore(h.badStore)
}

func TestRunWithOptionsPersistsActiveTurnBeforeProviderAndClosesCompleted(t *testing.T) {
	store := taskstate.NewStore(t.TempDir())
	const sessionID = "turn-success"
	args, err := json.Marshal(map[string]string{"path": "done.txt", "content": "completed"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := taskstate.New(sessionID, "finish the requested change", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := task.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	client := &durableTurnProbeClient{
		store:     store,
		sessionID: sessionID,
		responses: []llm.ChatResponse{
			toolResponse("write", "write_file", json.RawMessage(args)),
			{Message: llm.Message{Role: "assistant", Content: "status update"}},
		},
		observed: make(chan *taskstate.TurnRecord, 1),
	}
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{
		Workspace: workspace,
		VerifyTargets: func(context.Context, []string) (string, error) {
			return "verify PASS\nrequested checks passed", nil
		},
	})
	a := agent.New(cfg, client, reg, perm.New(config.ModeFast, workspace, nil))
	a.SetTaskStore(store)
	if err := a.SetTaskSession(sessionID); err != nil {
		t.Fatal(err)
	}
	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "finish the requested change"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	active := <-client.observed
	if active == nil || active.State != taskstate.TurnActive || active.Route != string(taskstate.TurnRouteAdmission) {
		t.Fatalf("provider observed turn = %#v, want persisted active admission turn", active)
	}
	if result.Task == nil || result.Task.Status != taskstate.StatusDone {
		t.Fatalf("result task = %#v, want done", result.Task)
	}

	persisted, err := store.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	last := persisted.LastTurn()
	if last == nil || last.State != taskstate.TurnCompleted || last.Route != string(taskstate.TurnRouteComplete) || last.StopReason != taskstate.StopGoalComplete {
		t.Fatalf("persisted completed turn = %#v", last)
	}
	if result.Task.LastTurn() == nil || result.Task.LastTurn().State != taskstate.TurnCompleted {
		t.Fatalf("result did not include closed turn = %#v", result.Task)
	}
}

func TestRunWithOptionsReturnsTurnClosePersistenceFailure(t *testing.T) {
	store := taskstate.NewStore(t.TempDir())
	const sessionID = "turn-close-persist-failure"
	workspace := t.TempDir()
	badRoot := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(badRoot, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	args, err := json.Marshal(map[string]string{"path": "done.txt", "content": "completed"})
	if err != nil {
		t.Fatal(err)
	}
	client := &durableTurnProbeClient{
		store:     store,
		sessionID: sessionID,
		responses: []llm.ChatResponse{
			toolResponse("write", "write_file", json.RawMessage(args)),
			{Message: llm.Message{Role: "assistant", Content: "Goal complete: done"}},
		},
		observed: make(chan *taskstate.TurnRecord, 1),
	}
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{
		Workspace: workspace,
		VerifyTargets: func(context.Context, []string) (string, error) {
			return "verify PASS\nrequested checks passed", nil
		},
	})
	a := agent.New(cfg, client, reg, perm.New(config.ModeFast, workspace, nil))
	a.SetTaskStore(store)
	if err := a.SetTaskSession(sessionID); err != nil {
		t.Fatal(err)
	}
	h := &turnCloseFailureHandler{ag: a, badStore: taskstate.NewStore(badRoot)}

	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "finish the requested change"}, h)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "durable task state") {
		t.Fatalf("turn close persistence failure = %v, want explicit persistence error", err)
	}
	if !h.switched {
		t.Fatal("test did not switch stores after terminal task persistence")
	}
	if result.GoalDone {
		t.Fatal("turn close persistence failure must not report GoalDone")
	}
	persisted, err := store.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status == taskstate.StatusDone || !persisted.NeedsVerification() {
		t.Fatalf("persisted terminal task = %#v, want resumable verification state", persisted)
	}
	if last := persisted.LastTurn(); last == nil || last.State != taskstate.TurnActive {
		t.Fatalf("persisted turn = %#v, want active turn for recovery", last)
	}
}

func TestRunWithOptionsReturnsTurnCloseFailureWithProviderError(t *testing.T) {
	goodStore := taskstate.NewStore(t.TempDir())
	badRoot := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(badRoot, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	const sessionID = "provider-close-persist-failure"
	client := &durableTurnProbeClient{
		store:     goodStore,
		sessionID: sessionID,
		err:       errors.New("provider unavailable"),
		observed:  make(chan *taskstate.TurnRecord, 1),
	}
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, client, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	a.SetTaskStore(goodStore)
	if err := a.SetTaskSession(sessionID); err != nil {
		t.Fatal(err)
	}
	h := &providerFailureCloseHandler{ag: a, badStore: taskstate.NewStore(badRoot)}

	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "investigate the provider failure"}, h)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "provider unavailable") || !strings.Contains(strings.ToLower(err.Error()), "durable task state was not saved") {
		t.Fatalf("provider and turn-close failures = %v, want both causes", err)
	}
	if !h.switched {
		t.Fatal("test did not switch stores after active turn persistence")
	}
	if result.GoalDone || result.Task == nil {
		t.Fatalf("provider failure result = %#v goalDone=%v, want resumable task", result.Task, result.GoalDone)
	}
	if last := result.Task.LastTurn(); last == nil || last.State != taskstate.TurnActive {
		t.Fatalf("provider failure result turn = %#v, want active turn after close failure", last)
	}
	persisted, err := goodStore.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if last := persisted.LastTurn(); last == nil || last.State != taskstate.TurnActive {
		t.Fatalf("persisted provider failure turn = %#v, want active turn for recovery", last)
	}
}

func TestRunWithOptionsInterruptsCanceledTurn(t *testing.T) {
	store := taskstate.NewStore(t.TempDir())
	const sessionID = "turn-canceled"
	ctx, cancel := context.WithCancel(context.Background())
	client := &durableTurnProbeClient{
		store:     store,
		sessionID: sessionID,
		observed:  make(chan *taskstate.TurnRecord, 1),
		cancel:    cancel,
	}
	client.err = context.Canceled
	a := newDurableTurnAgent(t, client, store, sessionID)
	// Cancel after the provider has inspected the active durable checkpoint, so
	// the test proves the close path rather than an admission short circuit.
	_, _, err := a.Run(ctx, nil, llm.Message{Role: "user", Content: "cancel this broken change"}, allowAll{})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "context canceled") {
		t.Fatalf("canceled run error = %v", err)
	}
	active := <-client.observed
	if active == nil || active.State != taskstate.TurnActive {
		t.Fatalf("provider observed turn = %#v, want active", active)
	}
	persisted, err := store.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	last := persisted.LastTurn()
	if last == nil || last.State != taskstate.TurnInterrupted || last.Route != string(taskstate.TurnRouteRecover) || last.StopReason != taskstate.StopCanceled {
		t.Fatalf("persisted canceled turn = %#v", last)
	}
}

func TestRunWithOptionsClosesProviderFailureAsRecovery(t *testing.T) {
	store := taskstate.NewStore(t.TempDir())
	const sessionID = "turn-provider-failure"
	client := &durableTurnProbeClient{
		store:     store,
		sessionID: sessionID,
		err:       errors.New("provider unavailable"),
		observed:  make(chan *taskstate.TurnRecord, 1),
	}
	a := newDurableTurnAgent(t, client, store, sessionID)

	_, _, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "investigate the provider failure"}, allowAll{})
	if err == nil || !strings.Contains(err.Error(), "provider unavailable") {
		t.Fatalf("provider failure = %v", err)
	}
	if active := <-client.observed; active == nil || active.State != taskstate.TurnActive {
		t.Fatalf("provider observed turn = %#v, want active", active)
	}
	persisted, err := store.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	last := persisted.LastTurn()
	if last == nil || last.State != taskstate.TurnCompleted || last.Route != string(taskstate.TurnRouteRecover) || last.StopReason != taskstate.StopResourceUnavailable {
		t.Fatalf("persisted provider failure turn = %#v", last)
	}
}

func TestRunWithOptionsClosesBudgetStopAsBlockedTurn(t *testing.T) {
	store := taskstate.NewStore(t.TempDir())
	const sessionID = "turn-budget"
	client := &durableTurnProbeClient{
		store:     store,
		sessionID: sessionID,
		response:  toolResponse("budget-tool", "unknown_tool", nil),
		observed:  make(chan *taskstate.TurnRecord, 1),
	}
	a := newDurableTurnAgent(t, client, store, sessionID)
	a.UpdateConfig(func(cfg *config.Config) { cfg.MaxToolRounds = 1 })

	_, _, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "finish within one round"}, allowAll{})
	if err == nil || !strings.Contains(err.Error(), "stopped after 1 tool rounds") {
		t.Fatalf("budget stop = %v", err)
	}
	if active := <-client.observed; active == nil || active.State != taskstate.TurnActive {
		t.Fatalf("provider observed turn = %#v, want active", active)
	}
	persisted, err := store.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	last := persisted.LastTurn()
	if last == nil || last.State != taskstate.TurnCompleted || last.Route != string(taskstate.TurnRouteBlocked) || last.StopReason != taskstate.StopBudgetExhausted || last.ToolRounds != 1 {
		t.Fatalf("persisted budget turn = %#v", last)
	}
	if persisted.Status != taskstate.StatusBlocked {
		t.Fatalf("persisted task status = %s, want blocked", persisted.Status)
	}
}
