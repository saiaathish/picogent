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
	workspacepkg "github.com/saiaathish/picogent/internal/workspace"
)

func TestDurableVerificationPersistsWorkspaceObservation(t *testing.T) {
	root := t.TempDir()
	args, err := json.Marshal(map[string]string{"path": "fixed.txt", "content": "fixed"})
	if err != nil {
		t.Fatal(err)
	}
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "write", Name: "write_file", Arguments: string(args)}}}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}}
	cfg := config.Default()
	cfg.Workspace = root
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{
		Workspace: root,
		VerifyTargets: func(_ context.Context, targets []string) (string, error) {
			if strings.Join(targets, ",") != "fixed.txt" {
				t.Fatalf("verification targets = %v, want fixed.txt", targets)
			}
			return "verify PASS\n1 passed", nil
		},
	})
	store := taskstate.NewStore(t.TempDir())
	a := agent.New(cfg, fake, reg, perm.New(config.ModeFast, root, nil))
	a.TaskStore = store
	a.SetTaskSession("observation-persisted")
	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "fix the broken file"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Task == nil || len(result.Task.Verification) == 0 {
		t.Fatalf("task verification = %#v", result.Task)
	}
	latest := result.Task.Verification[len(result.Task.Verification)-1]
	if !latest.Passed || latest.Observation == nil || len(latest.Observation.Files) != 1 || latest.Observation.Files[0].Path != "fixed.txt" || !latest.Observation.Files[0].Known {
		t.Fatalf("workspace-bound verification = %#v", latest)
	}
	loaded, err := store.Load(result.Task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Verification[len(loaded.Verification)-1].Observation == nil {
		t.Fatal("persisted verification lost workspace observation")
	}
}

func TestResumeInvalidatesExternallyRewrittenWorkspaceEvidence(t *testing.T) {
	root := t.TempDir()
	store := seedBoundPassingTask(t, root, "resume-rewrite")
	if err := os.WriteFile(filepath.Join(root, "fixed.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Workspace = root
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: root}), perm.New(config.ModeFast, root, nil))
	a.TaskStore = store
	if err := a.SetTaskSession("resume-rewrite"); err != nil {
		t.Fatal(err)
	}
	got := a.TaskSnapshot()
	if got == nil || got.Status != taskstate.StatusVerifying || !got.NeedsVerification() {
		t.Fatalf("stale task = %#v", got)
	}
	latest := got.Verification[len(got.Verification)-1]
	if latest.Passed || !strings.HasPrefix(latest.Summary, "verify INCONCLUSIVE") {
		t.Fatalf("stale verification = %#v", latest)
	}
	loaded, err := store.Load("resume-rewrite")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Verification[len(loaded.Verification)-1].Passed {
		t.Fatal("stale PASS remained persisted after resume")
	}
}

func TestResumeKeepsSameContentWorkspaceEvidenceFresh(t *testing.T) {
	root := t.TempDir()
	store := seedBoundPassingTask(t, root, "resume-same-content")
	if err := os.WriteFile(filepath.Join(root, "fixed.txt"), []byte("fixed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Workspace = root
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: root}), perm.New(config.ModeFast, root, nil))
	a.TaskStore = store
	if err := a.SetTaskSession("resume-same-content"); err != nil {
		t.Fatal(err)
	}
	got := a.TaskSnapshot()
	if got == nil || got.Status != taskstate.StatusDone || got.NeedsVerification() || !got.Verification[len(got.Verification)-1].Passed {
		t.Fatalf("same-content task = %#v", got)
	}
}

func TestResumeInvalidatesWorkspaceEvidenceAfterRootReplacement(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "fixed.txt"), []byte("fixed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := taskstate.NewStore(t.TempDir())
	task := seedBoundPassingTaskInStore(t, root, store, "resume-root-replacement")
	oldRoot := filepath.Join(parent, "old-workspace")
	if err := os.Rename(root, oldRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "fixed.txt"), []byte("fixed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Workspace = root
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: root}), perm.New(config.ModeFast, root, nil))
	a.TaskStore = store
	if err := a.SetTaskSession(task.SessionID); err != nil {
		t.Fatal(err)
	}
	got := a.TaskSnapshot()
	if got == nil || got.Status != taskstate.StatusVerifying || got.Verification[len(got.Verification)-1].Passed {
		t.Fatalf("root-replaced task = %#v", got)
	}
}

func TestCompletionRejectsMutationAfterVerificationObservation(t *testing.T) {
	root := t.TempDir()
	args, err := json.Marshal(map[string]string{"path": "fixed.txt", "content": "fixed"})
	if err != nil {
		t.Fatal(err)
	}
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "write", Name: "write_file", Arguments: string(args)}}}},
		{Message: llm.Message{Role: "assistant", Content: "Goal complete: the fix is verified"}},
	}}
	cfg := config.Default()
	cfg.Workspace = root
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{
		Workspace: root,
		VerifyTargets: func(context.Context, []string) (string, error) {
			return "verify PASS\n1 passed", nil
		},
	})
	a := agent.New(cfg, fake, reg, perm.New(config.ModeFast, root, nil))
	a.SetGoal("finish this project")
	a.TaskStore = taskstate.NewStore(t.TempDir())
	a.SetTaskSession("completion-event-mutation")
	h := &rewriteAfterVerificationHandler{root: root}
	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "fix the broken file"}, h)
	if err != nil {
		t.Fatal(err)
	}
	if result.GoalDone || result.Task == nil || result.Task.Status != taskstate.StatusVerifying || !result.Task.NeedsVerification() {
		t.Fatalf("completion after event mutation = %#v goalDone=%v", result.Task, result.GoalDone)
	}
	latest := result.Task.Verification[len(result.Task.Verification)-1]
	if latest.Passed || !strings.HasPrefix(latest.Summary, "verify INCONCLUSIVE") {
		t.Fatalf("event-mutated verification = %#v", latest)
	}
}

func TestVerificationMutationDuringCheckBecomesInconclusive(t *testing.T) {
	root := t.TempDir()
	args, err := json.Marshal(map[string]string{"path": "fixed.txt", "content": "fixed"})
	if err != nil {
		t.Fatal(err)
	}
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "write", Name: "write_file", Arguments: string(args)}}}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}}
	cfg := config.Default()
	cfg.Workspace = root
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{
		Workspace: root,
		VerifyTargets: func(context.Context, []string) (string, error) {
			if err := os.WriteFile(filepath.Join(root, "fixed.txt"), []byte("changed during verify\n"), 0o644); err != nil {
				return "", err
			}
			return "verify PASS\n1 passed", nil
		},
	})
	a := agent.New(cfg, fake, reg, perm.New(config.ModeFast, root, nil))
	a.TaskStore = taskstate.NewStore(t.TempDir())
	a.SetTaskSession("verification-event-mutation")
	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "fix the broken file"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Task == nil || result.Task.Status != taskstate.StatusBlocked || !result.Task.NeedsVerification() {
		t.Fatalf("during-check mutation task = %#v", result.Task)
	}
	latest := result.Task.Verification[len(result.Task.Verification)-1]
	if latest.Passed || !strings.HasPrefix(latest.Summary, "verify INCONCLUSIVE") {
		t.Fatalf("during-check verification = %#v", latest)
	}
}

func TestStaleAgentTaskRevisionCannotPublishCandidate(t *testing.T) {
	root := t.TempDir()
	store := taskstate.NewStore(t.TempDir())
	task, err := taskstate.New("stale-agent-revision", "fix the file", []string{"work"})
	if err != nil {
		t.Fatal(err)
	}
	if err := task.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Workspace = root
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "done"}}}}, tools.NewRegistry(tools.Context{Workspace: root}), perm.New(config.ModeFast, root, nil))
	a.TaskStore = store
	a.SetTaskSession(task.SessionID)
	stale := a.TaskSnapshot()

	other, err := store.Load(task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	other.NoteAttempt()
	if err := store.Save(other); err != nil {
		t.Fatal(err)
	}

	h := &taskRecordingHandler{ag: a}
	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "fix the file"}, h)
	if err != nil {
		t.Fatal(err)
	}
	if result.Task == nil || result.Task.Revision != stale.Revision || result.Task.Attempts != stale.Attempts || result.Task.Status != stale.Status {
		t.Fatalf("stale candidate was published: got=%#v stale=%#v", result.Task, stale)
	}
	current, err := store.Load(task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != other.Revision || current.Attempts != other.Attempts {
		t.Fatalf("stale agent overwrote current state: got=%#v current=%#v", current, other)
	}
	if len(h.errors) == 0 || !strings.Contains(h.errors[0].Error(), "revision conflict") {
		t.Fatalf("revision conflict was not surfaced: %v", h.errors)
	}
}

func seedBoundPassingTask(t *testing.T, root, sessionID string) *taskstate.Store {
	t.Helper()
	store := taskstate.NewStore(t.TempDir())
	seedBoundPassingTaskInStore(t, root, store, sessionID)
	return store
}

func seedBoundPassingTaskInStore(t *testing.T, root string, store *taskstate.Store, sessionID string) *taskstate.Task {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "fixed.txt"), []byte("fixed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	observation, err := workspacepkg.Capture(context.Background(), root, []string{"fixed.txt"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := taskstate.New(sessionID, "fix the file", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := task.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	task.RecordChanged("fixed.txt")
	task.AddVerificationWithObservation("go test ./...", true, "verify PASS\n1 passed", &observation)
	if err := task.SetStatus(taskstate.StatusDone); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}
	return task
}

type rewriteAfterVerificationHandler struct {
	allowAll
	root string
	once sync.Once
}

func (h *rewriteAfterVerificationHandler) OnToolEnd(call llm.ToolCall, result string, err error) {
	if call.Name != "verify" || err != nil || !strings.Contains(strings.ToUpper(result), "VERIFY PASS") {
		return
	}
	h.once.Do(func() {
		if writeErr := os.WriteFile(filepath.Join(h.root, "fixed.txt"), []byte("changed after observation\n"), 0o644); writeErr != nil {
			panic(writeErr)
		}
	})
}
