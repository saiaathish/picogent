package agent_test

import (
	"context"
	"encoding/json"
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

type taskRecordingHandler struct {
	allowAll
	ag        *agent.Agent
	mu        sync.Mutex
	snapshots []*taskstate.Task
	errors    []error
}

func (h *taskRecordingHandler) OnTaskState(task *taskstate.Task) {
	// A callback must be able to re-enter read-only task APIs. This would
	// deadlock if Agent invoked it while holding taskMu.
	_ = h.ag.TaskSnapshot()
	h.mu.Lock()
	h.snapshots = append(h.snapshots, task)
	h.mu.Unlock()
}

func (h *taskRecordingHandler) statuses() []taskstate.Status {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]taskstate.Status, 0, len(h.snapshots))
	for _, snapshot := range h.snapshots {
		out = append(out, snapshot.Status)
	}
	return out
}

func (h *taskRecordingHandler) OnError(err error) {
	h.mu.Lock()
	h.errors = append(h.errors, err)
	h.mu.Unlock()
}

func TestDurableTaskPersistsOutsideHistoryAndResumes(t *testing.T) {
	workspace := t.TempDir()
	store := taskstate.NewStore(t.TempDir())
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{Workspace: workspace})
	a := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "done"}}}}, reg, perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = store
	a.SetTaskSession("session-1")

	history, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "audit the auth flow"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Task == nil || result.Task.Status != taskstate.StatusDone {
		t.Fatalf("task=%#v", result.Task)
	}
	for _, msg := range history {
		if msg.Role == "system" && msg.Content == "" {
			t.Fatal("empty internal message persisted")
		}
	}

	resumed := agent.New(cfg, &llm.Scripted{}, reg, perm.New(config.ModeFast, workspace, nil))
	resumed.TaskStore = store
	resumed.SetTaskSession("session-1")
	if got := resumed.TaskSnapshot(); got == nil || got.ID != result.Task.ID || got.Goal != result.Task.Goal {
		t.Fatalf("resumed=%#v want=%#v", got, result.Task)
	}
}

func TestDurableTaskContinuesPastRoutineDeferral(t *testing.T) {
	workspace := t.TempDir()
	args, _ := json.Marshal(map[string]string{"path": "fixed.txt", "content": "fixed"})
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", Content: "I found the issue. Would you like me to fix it?"}},
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "1", Name: "write_file", Arguments: string(args)}}}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}}
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{Workspace: workspace})
	a := agent.New(cfg, fake, reg, perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = taskstate.NewStore(t.TempDir())
	a.SetTaskSession("session-2")

	history, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "fix the broken signup flow"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.Calls) != 3 {
		t.Fatalf("model calls=%d want 3", len(fake.Calls))
	}
	if result.Task == nil || result.Task.Status != taskstate.StatusDone {
		t.Fatalf("task=%#v", result.Task)
	}
	if len(result.FilesChanged) != 1 || filepath.Clean(result.FilesChanged[0]) != "fixed.txt" {
		t.Fatalf("changed=%v", result.FilesChanged)
	}
	for _, msg := range history {
		if msg.Role == "system" && msg.Content == "Internal task-loop instruction: the original request already authorizes the work. Do not ask whether to continue. Take the next obvious safe action with tools. Stop only for permission, a genuine user choice, repeated verification failure, an unavailable resource, or exhausted budget." {
			t.Fatal("internal continuation prompt persisted in chat history")
		}
	}
}

func TestDurableTaskRepairsFailedVerification(t *testing.T) {
	workspace := t.TempDir()
	writeArgs, _ := json.Marshal(map[string]string{"path": "fixed.txt", "content": "first"})
	repairArgs, _ := json.Marshal(map[string]string{"path": "fixed.txt", "content": "repaired"})
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "1", Name: "write_file", Arguments: string(writeArgs)}}}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "2", Name: "write_file", Arguments: string(repairArgs)}}}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}}
	checks := 0
	reg := tools.NewRegistry(tools.Context{
		Workspace: workspace,
		VerifyTargets: func(_ context.Context, targets []string) (string, error) {
			checks++
			if len(targets) != 1 || targets[0] != "fixed.txt" {
				t.Fatalf("targets=%v", targets)
			}
			if checks == 1 {
				return "verify FAIL\ntest failed", nil
			}
			return "verify PASS\n1 passed", nil
		},
	})
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, fake, reg, perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = taskstate.NewStore(t.TempDir())
	a.SetTaskSession("session-repair")

	history, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "fix the broken signup flow"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if checks != 2 || len(fake.Calls) != 4 {
		t.Fatalf("checks=%d calls=%d", checks, len(fake.Calls))
	}
	if result.Task == nil || result.Task.Status != taskstate.StatusDone || len(result.Task.Verification) != 2 {
		t.Fatalf("task=%#v", result.Task)
	}
	for _, msg := range history {
		if msg.Role == "system" && strings.HasPrefix(msg.Content, "Internal verification-repair instruction:") {
			t.Fatal("repair instruction persisted in chat history")
		}
	}
}

func TestDurableTaskStopsAfterThreeVerificationFailures(t *testing.T) {
	workspace := t.TempDir()
	args, _ := json.Marshal(map[string]string{"path": "fixed.txt", "content": "still broken"})
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "1", Name: "write_file", Arguments: string(args)}}}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}}
	checks := 0
	reg := tools.NewRegistry(tools.Context{Workspace: workspace, VerifyTargets: func(context.Context, []string) (string, error) {
		checks++
		return "verify FAIL\nstill failing", nil
	}})
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, fake, reg, perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = taskstate.NewStore(t.TempDir())
	a.SetTaskSession("session-bounded")

	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "fix the broken signup flow"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if checks != 3 || result.Task == nil || result.Task.Status != taskstate.StatusBlocked || result.Task.BlockedBy != "verification repeatedly failed" {
		t.Fatalf("checks=%d task=%#v", checks, result.Task)
	}
}

func TestDurableTaskPublishesPersistedProgressSnapshots(t *testing.T) {
	workspace := t.TempDir()
	args, _ := json.Marshal(map[string]string{"path": "fixed.txt", "content": "fixed"})
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "1", Name: "write_file", Arguments: string(args)}}}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}}
	reg := tools.NewRegistry(tools.Context{
		Workspace: workspace,
		VerifyTargets: func(context.Context, []string) (string, error) {
			return "verify PASS\n1 passed", nil
		},
	})
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, fake, reg, perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = taskstate.NewStore(t.TempDir())
	a.SetTaskSession("session-progress")
	h := &taskRecordingHandler{ag: a}

	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "fix the broken signup flow"}, h)
	if err != nil {
		t.Fatal(err)
	}
	statuses := h.statuses()
	for _, want := range []taskstate.Status{taskstate.StatusWorking, taskstate.StatusVerifying, taskstate.StatusDone} {
		found := false
		for _, got := range statuses {
			found = found || got == want
		}
		if !found {
			t.Fatalf("statuses = %v, missing %s", statuses, want)
		}
	}
	if result.Task == nil || len(result.Task.ChangedFiles) != 1 || result.Task.ChangedFiles[0] != "fixed.txt" {
		t.Fatalf("final task = %#v", result.Task)
	}
	if len(h.snapshots) == 0 || h.snapshots[len(h.snapshots)-1].Status != taskstate.StatusDone {
		t.Fatalf("snapshots = %#v", h.snapshots)
	}
	// Handler-owned snapshots must not alias the Agent's persisted state.
	h.snapshots[len(h.snapshots)-1].ChangedFiles[0] = "tampered.txt"
	if got := a.TaskSnapshot().ChangedFiles[0]; got != "fixed.txt" {
		t.Fatalf("agent snapshot was aliased: %q", got)
	}
}

func TestDurableTaskDoesNotPublishUnsavedState(t *testing.T) {
	workspace := t.TempDir()
	goodStore := taskstate.NewStore(t.TempDir())
	initial, err := taskstate.New("session-persist-fail", "fix the broken signup flow", []string{"work", "verify"})
	if err != nil {
		t.Fatal(err)
	}
	if err := initial.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	if err := goodStore.Save(initial); err != nil {
		t.Fatal(err)
	}

	// Make the store root a regular file so every subsequent atomic Save fails
	// after the agent has already loaded the last good snapshot.
	badRoot := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(badRoot, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{Workspace: workspace})
	fake := &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "done"}}}}
	a := agent.New(cfg, fake, reg, perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = goodStore
	a.SetTaskSession("session-persist-fail")
	lastPersisted := a.TaskSnapshot()
	a.TaskStore = taskstate.NewStore(badRoot)
	h := &taskRecordingHandler{ag: a}

	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "fix the broken signup flow"}, h)
	if err != nil {
		t.Fatal(err)
	}
	if result.Task == nil || result.Task.Status != lastPersisted.Status || result.Task.Attempts != lastPersisted.Attempts {
		t.Fatalf("result task changed despite failed saves: got=%#v last=%#v", result.Task, lastPersisted)
	}
	if got := a.TaskSnapshot(); got == nil || got.Status != lastPersisted.Status || got.Attempts != lastPersisted.Attempts {
		t.Fatalf("agent state changed despite failed saves: got=%#v last=%#v", got, lastPersisted)
	}
	if len(h.snapshots) != 0 {
		t.Fatalf("published unsaved snapshots: %#v", h.snapshots)
	}
	if len(h.errors) == 0 || !strings.Contains(h.errors[0].Error(), "durable task state was not saved") {
		t.Fatalf("persistence failure was not surfaced: %v", h.errors)
	}
	loaded, err := goodStore.Load("session-persist-fail")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != lastPersisted.Status || loaded.Attempts != lastPersisted.Attempts {
		t.Fatalf("last persisted state changed: got=%#v last=%#v", loaded, lastPersisted)
	}
}
