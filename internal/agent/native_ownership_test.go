package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
)

type nativeOwnershipEvents struct {
	allowUndoTest
	start func(llm.ToolCall)
	state func(*taskstate.Task)
	ends  []string
}

func (h *nativeOwnershipEvents) OnTaskState(task *taskstate.Task) {
	if h.state != nil {
		h.state(task)
	}
}

func (h *nativeOwnershipEvents) OnToolStart(call llm.ToolCall) {
	if h.start != nil {
		h.start(call)
	}
}

func (h *nativeOwnershipEvents) OnToolEnd(_ llm.ToolCall, _ string, err error) {
	if err != nil {
		h.ends = append(h.ends, err.Error())
	}
}

func TestNativeWritesRefuseChangedTaskBeforeSideEffects(t *testing.T) {
	changes := []struct {
		name   string
		change func(*testing.T, *taskstate.Task)
	}{
		{"task identity", func(_ *testing.T, task *taskstate.Task) { task.ID = "different-native-task" }},
		{"intent", func(t *testing.T, task *taskstate.Task) {
			if err := task.ReplaceOutcome("document the API"); err != nil {
				t.Fatal(err)
			}
		}},
		{"new turn", func(t *testing.T, task *taskstate.Task) {
			if _, ok := task.BeginTurn(taskstate.TurnRouteInspect); !ok {
				t.Fatal("start newer turn")
			}
		}},
		{"closed turn", func(t *testing.T, task *taskstate.Task) {
			if !task.FinishTurn(task.TurnRevision, taskstate.TurnRouteImplement, "closed elsewhere", "", taskstate.StopNone, 0, 0) {
				t.Fatal("close old turn")
			}
		}},
		{"same text ABA", func(t *testing.T, task *taskstate.Task) {
			original := task.Goal
			for _, prompt := range []string{"document the API", original} {
				if err := task.ReplaceOutcome(prompt); err != nil {
					t.Fatal(err)
				}
			}
		}},
	}
	for _, boundary := range []string{"before staging", "after recovery preparation"} {
		for _, kind := range []string{"write_file", "edit_file"} {
			for _, change := range changes {
				t.Run(boundary+"/"+kind+"/"+change.name, func(t *testing.T) {
					root := t.TempDir()
					path := filepath.Join(root, "note.txt")
					if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
						t.Fatal(err)
					}
					a := newUndoHookAgent(t, root)
					defer a.Close()
					a.SetTaskStore(taskstate.NewStore(t.TempDir()))
					if err := a.SetTaskSession("native-owner"); err != nil {
						t.Fatal(err)
					}
					call := fallbackCall(t, "first", kind, map[string]string{"path": "note.txt", "content": "after"})
					if kind == "edit_file" {
						call = fallbackCall(t, "first", kind, map[string]string{"path": "note.txt", "old_string": "before", "new_string": "after"})
					}
					second := call
					second.ID = "second"
					a.SetClient(&llm.Scripted{Responses: fallbackResponses(call, second)})
					changed := false
					mutate := func() {
						if changed {
							return
						}
						current, err := a.TaskStore.Load("native-owner")
						if err != nil {
							t.Fatal(err)
						}
						change.change(t, current)
						if err := a.TaskStore.Save(current); err != nil {
							t.Fatal(err)
						}
						changed = true
					}
					h := &nativeOwnershipEvents{}
					if boundary == "before staging" {
						h.start = func(llm.ToolCall) { mutate() }
					} else {
						a.runTool = func(ctx context.Context, call llm.ToolCall, tool tools.Tool, c tools.Context) (string, error) {
							prepare := c.BeforeWorkspacePublish
							c.BeforeWorkspacePublish = func(path string, data []byte, mode os.FileMode) error {
								if err := prepare(path, data, mode); err != nil {
									return err
								}
								mutate()
								return nil
							}
							return tool.Run(ctx, call.Arguments, c)
						}
					}
					_, _, _ = a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update note.txt"}, h)
					assertUndoFileContent(t, path, "before")
					if !changed || len(h.ends) < 2 || !strings.Contains(strings.Join(h.ends, "\n"), "ownership changed") {
						t.Fatalf("stale repeated writes were not refused: changed=%v errors=%v", changed, h.ends)
					}
					entries, err := os.ReadDir(root)
					if err != nil {
						t.Fatal(err)
					}
					for _, entry := range entries {
						if strings.HasPrefix(entry.Name(), ".picogent-workspace-") {
							t.Fatal("refused native publication leaked its staged file")
						}
					}
				})
			}
		}
	}
}

func TestNativeWriteUnavailableTaskStateCreatesNoParent(t *testing.T) {
	root := t.TempDir()
	a := newUndoHookAgent(t, root)
	defer a.Close()
	store := taskstate.NewStore(t.TempDir())
	a.SetTaskStore(store)
	if err := a.SetTaskSession("unavailable-native"); err != nil {
		t.Fatal(err)
	}
	call := fallbackCall(t, "create", "write_file", map[string]string{"path": "new/nested/note.txt", "content": "after"})
	a.SetClient(&llm.Scripted{Responses: fallbackResponses(call)})
	h := &nativeOwnershipEvents{start: func(llm.ToolCall) {
		path, err := store.Path("unavailable-native")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("invalid task JSON"), 0o600); err != nil {
			t.Fatal(err)
		}
	}}
	_, _, _ = a.Run(context.Background(), nil, llm.Message{Role: "user", Content: fallbackWritePrompt}, h)
	if len(h.ends) != 1 || !strings.Contains(h.ends[0], "task ownership") {
		t.Fatalf("unavailable task authority not explained: %v", h.ends)
	}
	if _, err := os.Stat(filepath.Join(root, "new")); !os.IsNotExist(err) {
		t.Fatalf("failed authority check created a parent: %v", err)
	}
}

func TestFallbackNativeOwnershipPrecedesFirstParentCreation(t *testing.T) {
	root := t.TempDir()
	a := newUndoHookAgent(t, root)
	defer a.Close()
	store := taskstate.NewStore(t.TempDir())
	a.SetTaskStore(store)
	if err := a.SetTaskSession("early-fallback"); err != nil {
		t.Fatal(err)
	}
	call := fallbackCall(t, "create", "write_file", map[string]string{"path": "new/nested/note.txt", "content": "after"})
	a.SetClient(&llm.Scripted{Responses: fallbackResponses(call)})
	checked := false
	a.runTool = func(ctx context.Context, call llm.ToolCall, tool tools.Tool, c tools.Context) (string, error) {
		guard := c.BeforeWorkspaceMutation
		if guard == nil {
			t.Fatal("native tool has no early guard")
		}
		c.BeforeWorkspaceMutation = func(path string) error {
			if err := guard(path); err != nil {
				return err
			}
			if !checked {
				persisted, err := store.Load("early-fallback")
				if err != nil || persisted.LastTurn() == nil || persisted.LastTurn().State != taskstate.TurnActive {
					t.Fatalf("first side effect lacks a durable turn: %+v %v", persisted, err)
				}
				if _, err := os.Stat(filepath.Join(root, "new")); !os.IsNotExist(err) {
					t.Fatal("ownership was persisted only after parent creation")
				}
				checked = true
			}
			return nil
		}
		return tool.Run(ctx, call.Arguments, c)
	}
	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: fallbackWritePrompt}, allowUndoTest{})
	if err != nil || !checked || !result.UndoAvailable {
		t.Fatalf("early fallback lost write/undo: result=%+v err=%v", result, err)
	}
	assertUndoFileContent(t, filepath.Join(root, "new", "nested", "note.txt"), "after")
}

func TestNativeWriteAllowsConcurrentSameOwnerProgress(t *testing.T) {
	root := t.TempDir()
	a := newUndoHookAgent(t, root)
	defer a.Close()
	store := taskstate.NewStore(t.TempDir())
	a.SetTaskStore(store)
	if err := a.SetTaskSession("same-native-owner"); err != nil {
		t.Fatal(err)
	}
	updated := false
	a.runTool = func(ctx context.Context, call llm.ToolCall, tool tools.Tool, c tools.Context) (string, error) {
		prepare := c.BeforeWorkspacePublish
		c.BeforeWorkspacePublish = func(path string, data []byte, mode os.FileMode) error {
			if err := prepare(path, data, mode); err != nil {
				return err
			}
			current, err := store.Load("same-native-owner")
			if err != nil {
				return err
			}
			current.NoteAttempt()
			updated = true
			return store.Save(current)
		}
		return tool.Run(ctx, call.Arguments, c)
	}
	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update note.txt"}, allowUndoTest{})
	if err != nil || !updated || !result.UndoAvailable || a.TaskSnapshot().Attempts != 2 {
		t.Fatalf("same owner progress blocked/lost: result=%+v err=%v", result, err)
	}
	assertUndoFileContent(t, filepath.Join(root, "note.txt"), "after")
}

func TestNativeRefusesAdmissionRebinding(t *testing.T) {
	for _, prompt := range []string{fallbackWritePrompt, "update new/nested/note.txt"} {
		for _, boundary := range []string{"task admission", "turn admission"} {
			for _, change := range []string{"store", "session generation", "task identity"} {
				t.Run(prompt+"/"+boundary+"/"+change, func(t *testing.T) {
					root := t.TempDir()
					a := newUndoHookAgent(t, root)
					defer a.Close()
					store := taskstate.NewStore(t.TempDir())
					a.SetTaskStore(store)
					if err := a.SetTaskSession("fallback-rebinding"); err != nil {
						t.Fatal(err)
					}
					call := fallbackCall(t, "create", "write_file", map[string]string{"path": "new/nested/note.txt", "content": "after"})
					a.SetClient(&llm.Scripted{Responses: fallbackResponses(call)})
					changed := false
					h := &nativeOwnershipEvents{state: func(task *taskstate.Task) {
						if changed || (task.TurnRevision == 0) != (boundary == "task admission") {
							return
						}
						changed = true
						switch change {
						case "store":
							replacement := taskstate.NewStore(t.TempDir())
							copy := cloneTask(task)
							copy.Revision = 0
							if err := replacement.Save(copy); err != nil {
								t.Fatal(err)
							}
							a.SetTaskStore(replacement)
						case "session generation":
							a.taskMu.Lock()
							a.taskSessionGeneration++ // same-ID session ABA
							a.taskMu.Unlock()
						case "task identity":
							a.taskMu.Lock()
							defer a.taskMu.Unlock()
							candidate := cloneTask(a.task)
							candidate.ID = "replacement-fallback-owner"
							if err := store.Save(candidate); err != nil {
								t.Fatal(err)
							}
							a.task = candidate
						}
					}}
					_, _, runErr := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: prompt}, h)
					if !changed || !strings.Contains(fmt.Sprint(runErr)+strings.Join(h.ends, "\n"), "ownership changed") {
						t.Fatalf("native write adopted rebound admission: changed=%v errors=%v runErr=%v", changed, h.ends, runErr)
					}
					if _, err := os.Stat(filepath.Join(root, "new")); !os.IsNotExist(err) {
						t.Fatalf("rebound admission created parent: %v", err)
					}
				})
			}
		}
	}
}
