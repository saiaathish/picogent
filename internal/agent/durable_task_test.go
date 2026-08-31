package agent_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
	workspacepkg "github.com/saiaathish/picogent/internal/workspace"
)

type taskRecordingHandler struct {
	allowAll
	ag        *agent.Agent
	mu        sync.Mutex
	snapshots []*taskstate.Task
	errors    []error
}

type terminalSaveFailureHandler struct {
	allowAll
	ag       *agent.Agent
	badStore *taskstate.Store
	switched bool
	mu       sync.Mutex
	errors   []error
}

func (h *terminalSaveFailureHandler) OnTaskState(task *taskstate.Task) {
	if task == nil {
		return
	}
	if !h.switched && task.Status == taskstate.StatusVerifying && len(task.Verification) > 0 {
		h.switched = true
		h.ag.SetTaskStore(h.badStore)
	}
}

func (h *terminalSaveFailureHandler) OnError(err error) {
	if err == nil {
		return
	}
	h.mu.Lock()
	h.errors = append(h.errors, err)
	h.mu.Unlock()
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
	if err := os.WriteFile(filepath.Join(workspace, "signup.go"), []byte("package signup\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := taskstate.NewStore(t.TempDir())
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "verify", Name: "verify", Arguments: `{"targets":["signup.go"]}`}}}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}}
	reg := tools.NewRegistry(tools.Context{
		Workspace: workspace,
		VerifyTargets: func(_ context.Context, targets []string) (string, error) {
			if strings.Join(targets, ",") != "signup.go" {
				t.Fatalf("verification targets = %v, want signup.go", targets)
			}
			return "verify PASS\naudit checks passed", nil
		},
	})
	a := agent.New(cfg, fake, reg, perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = store
	a.SetTaskSession("session-1")

	history, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "audit the signup flow"}, allowAll{})
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

func TestDurableTaskLoadFailureIsSurfaced(t *testing.T) {
	workspace := t.TempDir()
	store := taskstate.NewStore(t.TempDir())
	path, err := store.Path("corrupt-session")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	a.SetTaskStore(store)
	if err := a.SetTaskSession("corrupt-session"); err == nil {
		t.Fatal("corrupt task state should be reported")
	}
	if got := a.TaskSnapshot(); got != nil {
		t.Fatalf("corrupt task state was accepted: %#v", got)
	}
}

func TestDurableTaskLoadFailureStopsBeforeProvider(t *testing.T) {
	workspace := t.TempDir()
	store := taskstate.NewStore(t.TempDir())
	path, err := store.Path("corrupt-session")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(workspace, "must-not-change.txt")
	fake := &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "write", Name: "write_file", Arguments: `{"path":"must-not-change.txt","content":"unsafe"}`}}}}}}
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, fake, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	a.SetTaskStore(store)
	if err := a.SetTaskSession("corrupt-session"); err == nil {
		t.Fatal("corrupt task state should be reported")
	}

	_, result, err := a.RunWithOptions(context.Background(), nil, llm.Message{Role: "user", Content: "write the file"}, allowAll{}, agent.RunOptions{})
	if err == nil {
		t.Fatal("run should fail closed when durable task loading failed")
	}
	if len(fake.Calls) != 0 {
		t.Fatalf("provider calls = %d, want 0", len(fake.Calls))
	}
	if result.Text != "" || result.ToolRounds != 0 {
		t.Fatalf("failed run produced work: %#v", result)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("target file changed despite failed durable load: %v", err)
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
	reg := tools.NewRegistry(tools.Context{
		Workspace: workspace,
		Verify:    func(context.Context) (string, error) { return "verify PASS\nfixed", nil },
	})
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

func TestDurableTaskWriteWithoutVerifierRemainsVerifying(t *testing.T) {
	workspace := t.TempDir()
	args, _ := json.Marshal(map[string]string{"path": "fixed.txt", "content": "fixed"})
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "1", Name: "write_file", Arguments: string(args)}}}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}}
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, fake, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = taskstate.NewStore(t.TempDir())
	a.SetTaskSession("session-unverified-write")

	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "fix the broken signup flow"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Task == nil || result.Task.Status != taskstate.StatusVerifying {
		t.Fatalf("task=%#v, want verifying", result.Task)
	}
	if !result.Task.NeedsVerification() || result.Task.ChangeSeq != 1 {
		t.Fatalf("task evidence=%#v", result.Task)
	}
}

func TestDurableTaskResumesUnverifiedWriteWithAutomaticVerification(t *testing.T) {
	workspace := t.TempDir()
	store := taskstate.NewStore(t.TempDir())
	args, _ := json.Marshal(map[string]string{"path": "fixed.txt", "content": "fixed"})
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama

	initial := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "write", Name: "write_file", Arguments: string(args)}}}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	initial.TaskStore = store
	initial.SetTaskSession("session-resume-unverified-write")
	_, first, err := initial.Run(context.Background(), nil, llm.Message{Role: "user", Content: "fix the broken signup flow"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if first.Task == nil || !first.Task.NeedsVerification() {
		t.Fatalf("initial task=%#v", first.Task)
	}

	checks := 0
	var gotTargets []string
	resumed := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", Content: "Goal complete: the fix is verified"}},
	}}, tools.NewRegistry(tools.Context{
		Workspace: workspace,
		VerifyTargets: func(_ context.Context, targets []string) (string, error) {
			checks++
			gotTargets = append([]string(nil), targets...)
			return "verify PASS\n1 passed", nil
		},
	}), perm.New(config.ModeFast, workspace, nil))
	resumed.TaskStore = store
	resumed.SetTaskSession("session-resume-unverified-write")
	_, result, err := resumed.Run(context.Background(), nil, llm.Message{Role: "user", Content: "finish the task"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if checks != 1 {
		t.Fatalf("automatic verification calls=%d, want 1", checks)
	}
	if got := strings.Join(gotTargets, ","); got != "fixed.txt" {
		t.Fatalf("automatic verification targets=%v, want [fixed.txt]", gotTargets)
	}
	if result.Task == nil || result.Task.Status != taskstate.StatusDone || result.Task.NeedsVerification() {
		t.Fatalf("resumed task=%#v", result.Task)
	}
	if !result.GoalDone {
		t.Fatal("verified resumed completion should complete the result goal")
	}
}

func TestDurableTaskAutomaticVerificationIncludesRetainedChanges(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "first.txt"), []byte("first\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := taskstate.NewStore(t.TempDir())
	const sessionID = "session-cumulative-verification-targets"
	task, err := taskstate.New(sessionID, "fix both files", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.RecordChanged("first.txt")
	if err := task.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	if err := task.SetStatus(taskstate.StatusVerifying); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}

	secondArgs, err := json.Marshal(map[string]string{"path": "second.txt", "content": "second"})
	if err != nil {
		t.Fatal(err)
	}
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "second", Name: "write_file", Arguments: string(secondArgs)}}}},
		{Message: llm.Message{Role: "assistant", Content: "Goal complete: both files are verified"}},
	}}
	var gotTargets []string
	reg := tools.NewRegistry(tools.Context{
		Workspace: workspace,
		VerifyTargets: func(_ context.Context, targets []string) (string, error) {
			gotTargets = append([]string(nil), targets...)
			return "verify PASS\n2 files passed", nil
		},
	})
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, fake, reg, perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = store
	if err := a.SetTaskSession(sessionID); err != nil {
		t.Fatal(err)
	}

	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "continue fixing the task"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(gotTargets, ",") != "second.txt,first.txt" {
		t.Fatalf("automatic verification targets=%v, want current and retained changes", gotTargets)
	}
	if result.Task == nil || result.Task.Status != taskstate.StatusDone || result.Task.ChangeSeq != 2 || result.Task.VerifiedChangeSeq != 2 {
		t.Fatalf("cumulative verification task=%#v", result.Task)
	}
}

func TestDurableTaskResumesLegacyCompletedChangesForVerification(t *testing.T) {
	workspace := t.TempDir()
	store := taskstate.NewStore(t.TempDir())
	legacy, err := taskstate.New("session-legacy-completed-changes", "fix the broken signup flow", nil)
	if err != nil {
		t.Fatal(err)
	}
	legacy.Status = taskstate.StatusDone
	legacy.ChangedFiles = []string{"fixed.txt"}
	if err := store.Save(legacy); err != nil {
		t.Fatal(err)
	}

	checks := 0
	var gotTargets []string
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", Content: "Goal complete: the legacy fix is verified"}},
	}}, tools.NewRegistry(tools.Context{
		Workspace: workspace,
		VerifyTargets: func(_ context.Context, targets []string) (string, error) {
			checks++
			gotTargets = append([]string(nil), targets...)
			return "verify PASS\n1 passed", nil
		},
	}), perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = store
	a.SetTaskSession(legacy.SessionID)

	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "finish the task"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if checks != 1 || strings.Join(gotTargets, ",") != "fixed.txt" {
		t.Fatalf("legacy verification checks=%d targets=%v", checks, gotTargets)
	}
	if result.Task == nil || result.Task.ID != legacy.ID || result.Task.Status != taskstate.StatusDone || result.Task.NeedsVerification() {
		t.Fatalf("legacy result task=%#v", result.Task)
	}
	if result.Task.ChangeSeq != 1 || result.Task.VerifiedChangeSeq != 1 || !result.GoalDone {
		t.Fatalf("legacy evidence=%#v goalDone=%v", result.Task, result.GoalDone)
	}
}

func TestDurableTaskExplicitVerificationIncludesAllChangedFiles(t *testing.T) {
	workspace := t.TempDir()
	first, _ := json.Marshal(map[string]string{"path": "first.txt", "content": "first"})
	second, _ := json.Marshal(map[string]string{"path": "second.txt", "content": "second"})
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "first", Name: "write_file", Arguments: string(first)},
			{ID: "second", Name: "write_file", Arguments: string(second)},
		}}},
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "verify", Name: "verify", Arguments: `{"targets":["first.txt"]}`}}}},
		{Message: llm.Message{Role: "assistant", Content: "Goal complete: both files are verified"}},
	}}
	checks := 0
	var gotTargets []string
	reg := tools.NewRegistry(tools.Context{
		Workspace: workspace,
		VerifyTargets: func(_ context.Context, targets []string) (string, error) {
			checks++
			gotTargets = append([]string(nil), targets...)
			return "verify PASS\n1 passed", nil
		},
	})
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, fake, reg, perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = taskstate.NewStore(t.TempDir())
	a.SetTaskSession("session-explicit-full-targets")

	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "fix both files"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if checks != 1 || strings.Join(gotTargets, ",") != "first.txt,second.txt" {
		t.Fatalf("explicit verification checks=%d targets=%v", checks, gotTargets)
	}
	if result.Task == nil || result.Task.Status != taskstate.StatusDone || result.Task.NeedsVerification() {
		t.Fatalf("task=%#v", result.Task)
	}
	if result.Task.ChangeSeq != 2 || result.Task.VerifiedChangeSeq != 2 || !result.GoalDone {
		t.Fatalf("task evidence=%#v goalDone=%v", result.Task, result.GoalDone)
	}
}

func TestDurableTaskPassingVerificationCompletes(t *testing.T) {
	workspace := t.TempDir()
	args, _ := json.Marshal(map[string]string{"path": "fixed.txt", "content": "fixed"})
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "1", Name: "write_file", Arguments: string(args)}}}},
		{Message: llm.Message{Role: "assistant", Content: "Goal complete: all tests pass"}},
	}}
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{
		Workspace: workspace,
		Verify:    func(context.Context) (string, error) { return "verify PASS\n1 passed", nil },
	})
	a := agent.New(cfg, fake, reg, perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = taskstate.NewStore(t.TempDir())
	a.SetTaskSession("session-verified-write")

	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "fix the broken signup flow"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Task == nil || result.Task.Status != taskstate.StatusDone || result.Task.NeedsVerification() {
		t.Fatalf("task=%#v", result.Task)
	}
	if result.Task.ChangeSeq != 1 || result.Task.VerifiedChangeSeq != 1 || len(result.Task.Verification) != 1 {
		t.Fatalf("task evidence=%#v", result.Task)
	}
	if !result.GoalDone {
		t.Fatal("verified durable completion should complete the result goal")
	}
}

func TestDurableTaskCoBatchedWriteAndVerifyForcesFinalAutoVerify(t *testing.T) {
	workspace := t.TempDir()
	args, _ := json.Marshal(map[string]string{"path": "fixed.txt", "content": "fixed"})
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "write", Name: "write_file", Arguments: string(args)},
			{ID: "verify", Name: "verify", Arguments: `{}`},
		}}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}}
	var mu sync.Mutex
	checks := 0
	reg := tools.NewRegistry(tools.Context{
		Workspace: workspace,
		Verify: func(context.Context) (string, error) {
			mu.Lock()
			checks++
			mu.Unlock()
			return "verify PASS\n1 passed", nil
		},
	})
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, fake, reg, perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = taskstate.NewStore(t.TempDir())
	a.SetTaskSession("session-co-batched-verify")

	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "fix the broken signup flow"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	gotChecks := checks
	mu.Unlock()
	if gotChecks != 2 {
		t.Fatalf("verify calls=%d, want explicit verify plus final auto verify", gotChecks)
	}
	if result.Task == nil || result.Task.Status != taskstate.StatusDone || result.Task.NeedsVerification() {
		t.Fatalf("task=%#v", result.Task)
	}
	if result.Task.ChangeSeq != 1 || result.Task.VerifiedChangeSeq != 1 || len(result.Task.Verification) != 2 {
		t.Fatalf("task evidence=%#v", result.Task)
	}
}

func TestDurableTaskUnverifiedCompletionMarkerDoesNotCompleteGoal(t *testing.T) {
	workspace := t.TempDir()
	args, _ := json.Marshal(map[string]string{"path": "fixed.txt", "content": "fixed"})
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "1", Name: "write_file", Arguments: string(args)}}}},
		{Message: llm.Message{Role: "assistant", Content: "Goal complete: all tests pass"}},
	}}
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, fake, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = taskstate.NewStore(t.TempDir())
	a.SetTaskSession("session-unverified-marker")

	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "fix the broken signup flow"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Task == nil || result.Task.Status != taskstate.StatusVerifying {
		t.Fatalf("task=%#v", result.Task)
	}
	if result.GoalDone {
		t.Fatal("an unverified durable task must not complete the result goal")
	}
}

func TestDurableTaskTruncatedVerificationCannotCompleteGoal(t *testing.T) {
	workspace := t.TempDir()
	args, _ := json.Marshal(map[string]string{"path": "fixed.txt", "content": "fixed"})
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "1", Name: "write_file", Arguments: string(args)}}}},
		{Message: llm.Message{Role: "assistant", Content: "Goal complete: all tests pass"}},
	}}
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{
		Workspace: workspace,
		VerifyTargets: func(context.Context, []string) (string, error) {
			return "verify INCONCLUSIVE\nverification output was truncated", nil
		},
	})
	a := agent.New(cfg, fake, reg, perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = taskstate.NewStore(t.TempDir())
	a.SetTaskSession("session-truncated-verification")

	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "fix the broken signup flow"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if result.GoalDone {
		t.Fatal("truncated verification must not complete the result goal")
	}
	if result.Task == nil || !result.Task.NeedsVerification() {
		t.Fatalf("truncated verification task = %#v", result.Task)
	}
	if len(result.Task.Verification) == 0 || result.Task.Verification[len(result.Task.Verification)-1].Passed || !strings.HasPrefix(result.Task.Verification[len(result.Task.Verification)-1].Summary, "verify INCONCLUSIVE") {
		t.Fatalf("truncated verification evidence = %#v", result.Task)
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
	diversityGateSeen := false
	for _, call := range fake.Calls {
		for _, msg := range call.Messages {
			if strings.Contains(msg.Content, "same verification failure repeated") {
				diversityGateSeen = true
			}
		}
	}
	if !diversityGateSeen {
		t.Fatal("repeated verification failure did not request a different repair route")
	}
}

func TestBlockedDurableTaskRerunPreservesCheckpoint(t *testing.T) {
	workspace := t.TempDir()
	store := taskstate.NewStore(t.TempDir())
	if err := os.WriteFile(filepath.Join(workspace, "signup.go"), []byte("package signup\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	observation, err := workspacepkg.Capture(context.Background(), workspace, []string{"signup.go"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := taskstate.New("session-blocked-resume", "fix the broken signup flow", []string{"inspect", "verify"})
	if err != nil {
		t.Fatal(err)
	}
	if err := task.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	task.RecordChanged("signup.go")
	task.AddVerificationForCriteria([]int{0, 1}, "go test ./...", true, "verify PASS", &observation)
	task.Block("permission needed")
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "done"}}}}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = store
	a.SetTaskSession(task.SessionID)
	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: task.Goal}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Task == nil || result.Task.ID != task.ID || result.Task.Status != taskstate.StatusDone {
		t.Fatalf("rerun replaced or failed to finish checkpoint: %#v", result.Task)
	}
	if len(result.Task.ChangedFiles) != 1 || result.Task.ChangedFiles[0] != "signup.go" || result.Task.Attempts != task.Attempts+1 {
		t.Fatalf("rerun lost checkpoint details: %#v", result.Task)
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
	if err == nil {
		t.Fatal("run should fail closed when durable task state cannot be saved")
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
	if len(fake.Calls) != 0 {
		t.Fatalf("provider calls = %d, want 0", len(fake.Calls))
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

func TestDurableTaskTerminalSaveFailureIsReturned(t *testing.T) {
	workspace := t.TempDir()
	goodStore := taskstate.NewStore(t.TempDir())
	badRoot := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(badRoot, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]string{"path": "fixed.txt", "content": "fixed"})
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "1", Name: "write_file", Arguments: string(args)}}}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}}
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{
		Workspace: workspace,
		VerifyTargets: func(context.Context, []string) (string, error) {
			return "verify PASS\n1 passed", nil
		},
	})
	a := agent.New(cfg, fake, reg, perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = goodStore
	if err := a.SetTaskSession("terminal-save-failure"); err != nil {
		t.Fatal(err)
	}
	h := &terminalSaveFailureHandler{ag: a, badStore: taskstate.NewStore(badRoot)}

	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "fix the broken signup flow"}, h)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "durable task state") {
		t.Fatalf("terminal save failure = %v, want explicit persistence error", err)
	}
	if !h.switched {
		t.Fatal("test did not switch stores after persisted verification")
	}
	if result.GoalDone {
		t.Fatal("terminal save failure must not report GoalDone")
	}
	loaded, err := goodStore.Load("terminal-save-failure")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status == taskstate.StatusDone || loaded.NeedsVerification() == false {
		t.Fatalf("last persisted task = %#v, want non-terminal verification state", loaded)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.errors) == 0 || !strings.Contains(strings.ToLower(h.errors[len(h.errors)-1].Error()), "durable task state was not saved") {
		t.Fatalf("persistence error events = %v", h.errors)
	}
}

func TestDurableTaskResumesActiveCompletionIntentAfterMissingEvidence(t *testing.T) {
	workspace := t.TempDir()
	store := taskstate.NewStore(t.TempDir())
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama

	initial := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", Content: "Goal complete: the project is finished"}},
	}}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	initial.SetGoal("finish this project")
	initial.TaskStore = store
	initial.SetTaskSession("resume-completion-intent")
	_, first, err := initial.Run(context.Background(), nil, llm.Message{Role: "user", Content: "finish this project"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if first.GoalDone || first.Task == nil || first.Task.Status != taskstate.StatusVerifying {
		t.Fatalf("initial completion = %#v goalDone=%v, want verifying/unresolved", first.Task, first.GoalDone)
	}

	checks := 0
	resumed := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", Content: "Goal complete: the project is finished"}},
	}}, tools.NewRegistry(tools.Context{
		Workspace: workspace,
		VerifyTargets: func(_ context.Context, targets []string) (string, error) {
			checks++
			if len(targets) != 0 {
				t.Fatalf("resume completion targets = %v, want none", targets)
			}
			return "verify PASS\nproject checks passed", nil
		},
	}), perm.New(config.ModeFast, workspace, nil))
	resumed.SetGoal("finish this project")
	resumed.TaskStore = store
	resumed.SetTaskSession("resume-completion-intent")
	_, result, err := resumed.Run(context.Background(), nil, llm.Message{Role: "user", Content: "finish this project"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if checks != 1 {
		t.Fatalf("resume verification calls = %d, want 1", checks)
	}
	if result.Task == nil || result.Task.Status != taskstate.StatusBlocked || !result.Task.NeedsVerification() || result.GoalDone {
		t.Fatalf("resumed completion = %#v goalDone=%v, want blocked on unbound evidence", result.Task, result.GoalDone)
	}
	if len(result.Task.Verification) == 0 || result.Task.Verification[len(result.Task.Verification)-1].Passed || !strings.HasPrefix(result.Task.Verification[len(result.Task.Verification)-1].Summary, "verify INCONCLUSIVE") {
		t.Fatalf("resumed evidence = %#v", result.Task.Verification)
	}
}

func TestTaskSnapshotDoesNotAliasTurnLedger(t *testing.T) {
	workspace := t.TempDir()
	store := taskstate.NewStore(t.TempDir())
	task, err := taskstate.New("turn-snapshot", "finish the requested change", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := task.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	sequence, ok := task.BeginTurn(taskstate.TurnRouteImplement)
	if !ok {
		t.Fatal("turn did not start")
	}
	task.RecordChanged("internal/turn.go")
	if !task.FinishTurn(sequence, taskstate.TurnRouteImplement, "implement the requested change", "UNVERIFIED", taskstate.StopNone, 1, 1) {
		t.Fatal("turn did not finish")
	}
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = store
	if err := a.SetTaskSession(task.SessionID); err != nil {
		t.Fatal(err)
	}

	snapshot := a.TaskSnapshot()
	if snapshot == nil || len(snapshot.Turns) != 1 || snapshot.Turns[0].FinishedAt == nil {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	snapshot.Turns[0].Route = string(taskstate.TurnRouteRecover)
	snapshot.Turns[0].ChangedFiles[0] = "tampered-turn.go"
	*snapshot.Turns[0].FinishedAt = snapshot.Turns[0].FinishedAt.Add(24 * time.Hour)

	current := a.TaskSnapshot()
	if current == nil || current.Turns[0].Route != string(taskstate.TurnRouteImplement) || current.Turns[0].ChangedFiles[0] != "internal/turn.go" {
		t.Fatalf("agent turn ledger was aliased: %#v", current)
	}
	if current.Turns[0].FinishedAt.Equal(*snapshot.Turns[0].FinishedAt) {
		t.Fatal("agent turn finish time was aliased")
	}
	reloaded, err := store.Load(task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Turns[0].Route != string(taskstate.TurnRouteImplement) || reloaded.Turns[0].FinishedAt.Equal(*snapshot.Turns[0].FinishedAt) {
		t.Fatalf("persisted turn ledger was aliased: %#v", reloaded.Turns[0])
	}
}
