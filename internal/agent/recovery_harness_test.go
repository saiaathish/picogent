package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
)

// cancelAfterDurableWriteClient makes the provider boundary explicit: the
// write response is released first, then the next model call waits for the
// caller to cancel the turn. This keeps the cancellation point deterministic.
type cancelAfterDurableWriteClient struct {
	args          string
	secondStarted chan struct{}
	startedOnce   sync.Once
	call          int
}

func (c *cancelAfterDurableWriteClient) Chat(ctx context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	c.call++
	if c.call == 1 {
		return toolResponse("write", "write_file", json.RawMessage(c.args)), nil
	}
	c.startedOnce.Do(func() { close(c.secondStarted) })
	<-ctx.Done()
	return llm.ChatResponse{}, ctx.Err()
}

// persistedTaskObserver checks the save-before-publish contract at the
// callback boundary, not only after the whole run has finished.
type persistedTaskObserver struct {
	allowAll
	store      *taskstate.Store
	mu         sync.Mutex
	unsaved    []string
	statusSeen []taskstate.Status
}

func (h *persistedTaskObserver) OnTaskState(task *taskstate.Task) {
	if task == nil {
		return
	}
	persisted, err := h.store.Load(task.SessionID)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.statusSeen = append(h.statusSeen, task.Status)
	if err != nil {
		h.unsaved = append(h.unsaved, fmt.Sprintf("%s: load persisted snapshot: %v", task.Status, err))
		return
	}
	if persisted.ID != task.ID || persisted.Status != task.Status || persisted.Attempts != task.Attempts || persisted.ChangeSeq != task.ChangeSeq || persisted.VerifiedChangeSeq != task.VerifiedChangeSeq || !slices.Equal(persisted.ChangedFiles, task.ChangedFiles) {
		h.unsaved = append(h.unsaved, fmt.Sprintf("published %s snapshot before matching store state", task.Status))
	}
}

func (h *persistedTaskObserver) unsavedSnapshots() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.unsaved...)
}

func (h *persistedTaskObserver) statuses() []taskstate.Status {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]taskstate.Status(nil), h.statusSeen...)
}

func TestDurableTaskCancellationRetainsLastPersistedState(t *testing.T) {
	workspace := t.TempDir()
	store := taskstate.NewStore(t.TempDir())
	args, err := json.Marshal(map[string]string{"path": "note.txt", "content": "after"})
	if err != nil {
		t.Fatal(err)
	}
	client := &cancelAfterDurableWriteClient{
		args:          string(args),
		secondStarted: make(chan struct{}),
	}
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, client, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = store
	const sessionID = "cancel-after-durable-write"
	if err := a.SetTaskSession(sessionID); err != nil {
		t.Fatal(err)
	}
	observer := &persistedTaskObserver{store: store}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type runResult struct {
		result agent.Result
		err    error
	}
	done := make(chan runResult, 1)
	go func() {
		_, result, err := a.Run(ctx, nil, llm.Message{Role: "user", Content: "fix the broken note workflow"}, observer)
		done <- runResult{result: result, err: err}
	}()

	select {
	case <-client.secondStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("provider did not reach the cancellation barrier")
	}
	cancel()

	var run runResult
	select {
	case run = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("canceled durable run did not finish")
	}
	if run.err == nil || !strings.Contains(strings.ToLower(run.err.Error()), "context canceled") {
		t.Fatalf("canceled run error = %v", run.err)
	}
	if unsaved := observer.unsavedSnapshots(); len(unsaved) != 0 {
		t.Fatalf("published snapshots that were not persisted first: %v", unsaved)
	}

	persisted, err := store.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != taskstate.StatusWorking || !slices.Contains(persisted.ChangedFiles, "note.txt") {
		t.Fatalf("persisted cancellation state = %#v, want working with note.txt", persisted)
	}
	last := persisted.LastTurn()
	if last == nil || last.State != taskstate.TurnInterrupted || !slices.Equal(last.ChangedFiles, []string{"note.txt"}) || last.ChangedFilesCapped || last.MutationCount != 1 {
		t.Fatalf("persisted interrupted turn side effects = %#v", last)
	}
	current := a.TaskSnapshot()
	if current == nil || current.Status != persisted.Status || !slices.Equal(current.ChangedFiles, persisted.ChangedFiles) {
		t.Fatalf("live cancellation state = %#v, persisted = %#v", current, persisted)
	}
	for _, status := range observer.statuses() {
		if status == taskstate.StatusDone {
			t.Fatalf("canceled turn published terminal task status: %v", observer.statuses())
		}
	}
	if run.result.Task != nil {
		t.Fatalf("canceled run returned a terminal result task: %#v", run.result.Task)
	}
}
