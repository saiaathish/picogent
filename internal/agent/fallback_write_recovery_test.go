package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
)

const fallbackWritePrompt = "what is in note.txt?"

func fallbackCall(t *testing.T, id, name string, args map[string]string) llm.ToolCall {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return llm.ToolCall{ID: id, Name: name, Arguments: string(raw)}
}

func fallbackResponses(calls ...llm.ToolCall) []llm.ChatResponse {
	return []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: calls}},
		{Message: llm.Message{Role: "assistant", Content: "Stopped here."}},
	}
}

func TestFallbackNativeWritePersistsBeforePublication(t *testing.T) {
	if taskstate.Infer(fallbackWritePrompt).TaskLike {
		t.Fatal("fixture is not informational")
	}
	for _, kind := range []string{"write_file", "edit_file", "multi-round"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "note.txt")
			if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
				t.Fatal(err)
			}
			store := taskstate.NewStore(t.TempDir())
			a := newUndoHookAgent(t, root)
			defer a.Close()
			a.SetTaskStore(store)
			const session = "fallback-publish"
			if err := a.SetTaskSession(session); err != nil {
				t.Fatal(err)
			}
			first := fallbackCall(t, "first", "write_file", map[string]string{"path": "note.txt", "content": "after"})
			if kind == "edit_file" {
				first = fallbackCall(t, "first", "edit_file", map[string]string{"path": "note.txt", "old_string": "before", "new_string": "after"})
			}
			responses := fallbackResponses(first)
			if kind == "multi-round" {
				create := fallbackCall(t, "create", "write_file", map[string]string{"path": "created.txt", "content": "created"})
				edit := fallbackCall(t, "edit", "edit_file", map[string]string{"path": "note.txt", "old_string": "after", "new_string": "final"})
				responses = []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{first, create}}},
					{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{edit}}},
					{Message: llm.Message{Role: "assistant", Content: "Stopped here."}},
				}
			}
			a.SetClient(&llm.Scripted{Responses: responses})
			publications := 0
			var sequence uint64
			a.runTool = func(ctx context.Context, call llm.ToolCall, tool tools.Tool, c tools.Context) (string, error) {
				if publications == 0 && a.TaskSnapshot() != nil {
					t.Fatal("informational prompt admitted a task before native publication")
				}
				hook := c.BeforeWorkspacePublish
				c.BeforeWorkspacePublish = func(path string, data []byte, mode os.FileMode) error {
					if err := hook(path, data, mode); err != nil {
						return err
					}
					publications++
					persisted, err := store.Load(session)
					if err != nil || persisted.LastTurn() == nil || persisted.LastTurn().State != taskstate.TurnActive || persisted.LastTurn().Sequence == 0 {
						t.Fatalf("native rename has no durable active turn: task=%+v err=%v", persisted, err)
					}
					journal, err := loadUndoJournal(root, session, true)
					if err != nil || journal.TurnSequence != persisted.LastTurn().Sequence || len(journal.Checkpoint.Entries) == 0 {
						t.Fatalf("native rename has no matching recovery journal: journal=%+v err=%v", journal, err)
					}
					if sequence != 0 && journal.TurnSequence != sequence {
						t.Fatal("one tool batch/round admitted multiple turn identities")
					}
					sequence = journal.TurnSequence
					if publications == 1 {
						assertUndoFileContent(t, path, "before")
					}
					return nil
				}
				return tool.Run(ctx, call.Arguments, c)
			}
			_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: fallbackWritePrompt}, allowUndoTest{})
			want := 1
			if kind == "multi-round" {
				want = 3
			}
			if err != nil || publications != want || !result.UndoAvailable || len(a.TaskSnapshot().Turns) != 1 {
				t.Fatalf("fallback native turn=%+v publications=%d err=%v", result, publications, err)
			}
			if _, err := a.UndoLastTurn(); err != nil {
				t.Fatal(err)
			}
			assertUndoFileContent(t, path, "before")
			if _, err := os.Stat(filepath.Join(root, "created.txt")); !os.IsNotExist(err) {
				t.Fatalf("undo retained newly created file: %v", err)
			}
		})
	}
}

type fallbackTaskObserver struct {
	allowUndoTest
	onTask  func(*taskstate.Task)
	toolErr error
}

func (h *fallbackTaskObserver) OnTaskState(task *taskstate.Task) {
	if h.onTask != nil {
		h.onTask(task)
	}
}

func (h *fallbackTaskObserver) OnToolEnd(_ llm.ToolCall, _ string, err error) {
	if err != nil {
		h.toolErr = err
	}
}

func TestFallbackNativeWritePersistenceFailureForbidsPublication(t *testing.T) {
	for _, stage := range []string{"task", "load", "turn", "missing-task", "exhausted-turn", "journal"} {
		t.Run(stage, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "note.txt")
			if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
				t.Fatal(err)
			}
			beforeInfo, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			a := newUndoHookAgent(t, root)
			defer a.Close()
			a.SetTaskStore(taskstate.NewStore(t.TempDir()))
			if err := a.SetTaskSession("fallback-save-failure"); err != nil {
				t.Fatal(err)
			}
			blockedStore := filepath.Join(t.TempDir(), "not-a-directory")
			if err := os.WriteFile(blockedStore, []byte("owned fixture"), 0o600); err != nil {
				t.Fatal(err)
			}
			badStore := taskstate.NewStore(blockedStore)
			h := &fallbackTaskObserver{}
			if stage == "turn" || stage == "missing-task" || stage == "exhausted-turn" {
				h.onTask = func(task *taskstate.Task) {
					if task.LastTurn() == nil {
						if stage == "turn" {
							a.SetTaskStore(badStore)
						} else {
							a.taskMu.Lock()
							if stage == "missing-task" {
								a.task = nil
							} else {
								a.task.TurnRevision = ^uint64(0)
							}
							a.taskMu.Unlock()
						}
					}
				}
			}
			a.runTool = func(ctx context.Context, call llm.ToolCall, tool tools.Tool, c tools.Context) (string, error) {
				if stage == "task" {
					a.SetTaskStore(badStore)
				} else if stage == "load" {
					a.taskMu.Lock()
					a.taskLoadErr = errors.New("owned admission failure")
					a.taskMu.Unlock()
				} else if stage == "journal" {
					if err := os.MkdirAll(filepath.Join(root, ".picogent"), 0o700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(root, ".picogent", "undo"), []byte("owned fixture"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				return tool.Run(ctx, call.Arguments, c)
			}
			_, result, _ := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: fallbackWritePrompt}, h)
			if h.toolErr == nil || len(result.FilesChanged) != 0 || result.UndoAvailable {
				t.Fatalf("failed %s persistence still published: result=%+v toolErr=%v", stage, result, h.toolErr)
			}
			assertUndoFileContent(t, path, "before")
			afterInfo, err := os.Stat(path)
			if err != nil || !os.SameFile(beforeInfo, afterInfo) {
				t.Fatalf("rejected publication still replaced original inode: %v", err)
			}
		})
	}
}

func TestFallbackReadAndNoMatchDoNotAdmitNativeTurn(t *testing.T) {
	for _, name := range []string{"read_file", "edit_file"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("before"), 0o644); err != nil {
				t.Fatal(err)
			}
			a := newUndoHookAgent(t, root)
			defer a.Close()
			a.SetTaskStore(taskstate.NewStore(t.TempDir()))
			if err := a.SetTaskSession("fallback-control"); err != nil {
				t.Fatal(err)
			}
			args := map[string]string{"path": "note.txt"}
			if name == "edit_file" {
				args["old_string"], args["new_string"] = "not present", "after"
			}
			a.SetClient(&llm.Scripted{Responses: fallbackResponses(fallbackCall(t, "control", name, args))})
			_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: fallbackWritePrompt}, allowUndoTest{})
			if err != nil || a.TaskSnapshot() != nil || result.UndoAvailable || len(result.FilesChanged) != 0 {
				t.Fatalf("non-publishing tool admitted a fallback: %+v %v", result, err)
			}
			assertUndoFileContent(t, filepath.Join(root, "note.txt"), "before")
		})
	}
}

type fallbackDenyHandler struct{ NopHandler }

func (fallbackDenyHandler) OnNeedPermission(context.Context, perm.Request) (perm.Decision, error) {
	return perm.Deny, nil
}

func TestFallbackDeniedWriteDoesNotReachPublication(t *testing.T) {
	root := t.TempDir()
	a := newUndoHookAgent(t, root)
	defer a.Close()
	a.SetTaskStore(taskstate.NewStore(t.TempDir()))
	if err := a.SetTaskSession("fallback-denied"); err != nil {
		t.Fatal(err)
	}
	a.SetMode("safe")
	called := false
	a.runTool = func(context.Context, llm.ToolCall, tools.Tool, tools.Context) (string, error) {
		called = true
		return "", nil
	}
	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: fallbackWritePrompt}, fallbackDenyHandler{})
	task := a.TaskSnapshot()
	denied := false
	if task != nil {
		for _, evidence := range task.Evidence {
			denied = denied || (evidence.Kind == "approval" && evidence.Status == "DENIED")
		}
	}
	if err != nil || called || task == nil || task.Status != taskstate.StatusBlocked || result.UndoAvailable || !denied {
		t.Fatalf("denial lost blocked projection or reached publication: %+v called=%v task=%+v err=%v", result, called, task, err)
	}
}
