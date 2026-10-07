package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
)

func TestUndoSealPreservesUserEditBeforeTurnSeal(t *testing.T) {
	for _, toolName := range []string{"write_file", "edit_file"} {
		for _, reportError := range []bool{false, true} {
			name := toolName + "/success"
			if reportError {
				name = toolName + "/error after publication"
			}
			t.Run(name, func(t *testing.T) {
				root := t.TempDir()
				path := filepath.Join(root, "note.txt")
				if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
					t.Fatal(err)
				}
				store := taskstate.NewStore(t.TempDir())
				first := newUndoHookAgent(t, root)
				first.SetTaskStore(store)
				const sessionID = "pre-seal-user-edit"
				if err := first.SetTaskSession(sessionID); err != nil {
					t.Fatal(err)
				}
				args := map[string]string{"path": "note.txt", "content": "after"}
				if toolName == "edit_file" {
					args = map[string]string{"path": "note.txt", "old_string": "before", "new_string": "after"}
				}
				raw, err := json.Marshal(args)
				if err != nil {
					t.Fatal(err)
				}
				first.SetClient(&llm.Scripted{Responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "native-write", Name: toolName, Arguments: string(raw)}}}},
					{Message: llm.Message{Role: "assistant", Content: "done"}},
				}})
				if reportError {
					first.runTool = func(ctx context.Context, call llm.ToolCall, tool tools.Tool, c tools.Context) (string, error) {
						out, err := tool.Run(ctx, call.Arguments, c)
						if err != nil {
							return out, err
						}
						return out, errors.New("simulated error after native publication")
					}
				}
				events := &undoSealUserEditHandler{t: t, path: path, root: root, sessionID: sessionID, toolName: toolName}
				_, result, err := first.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update note.txt"}, events)
				if err != nil {
					t.Fatal(err)
				}
				if !events.edited || !result.UndoAvailable || result.UndoError != "" {
					t.Fatalf("turn did not publish a sealed undo record: edited=%v result=%+v", events.edited, result)
				}
				sealed, err := loadUndoJournal(root, sessionID, false)
				if err != nil {
					t.Fatal(err)
				}
				if len(sealed.Checkpoint.Entries) != 1 || sealed.Checkpoint.Entries[0].Expected != events.expected || sealed.Checkpoint.Entries[0].Published != "" {
					t.Fatalf("sealed journal adopted the user's edit: %+v; prepared fingerprint=%s", sealed.Checkpoint, events.expected)
				}
				if _, err := first.UndoLastTurn(); err == nil || !strings.Contains(err.Error(), "newer changes") {
					t.Fatalf("in-process undo did not block the newer edit: %v", err)
				}
				assertUndoFileContent(t, path, "user")

				second := newUndoHookAgent(t, root)
				second.SetTaskStore(store)
				if err := second.SetTaskSession(sessionID); err != nil {
					t.Fatal(err)
				}
				if !second.UndoAvailable() {
					t.Fatal("fresh agent did not retain the conflicted sealed undo")
				}
				if _, err := second.UndoLastTurn(); err == nil || !strings.Contains(err.Error(), "newer changes") {
					t.Fatalf("fresh-agent undo did not block the newer edit: %v", err)
				}
				assertUndoFileContent(t, path, "user")
				if !first.UndoAvailable() || !second.UndoAvailable() {
					t.Fatal("conflicted undo was consumed")
				}
			})
		}
	}
}

func TestUndoSealPreservesUserEditWithoutDurableTurn(t *testing.T) {
	for _, missing := range []string{"task-store", "task-session", "both"} {
		for _, toolName := range []string{"write_file", "edit_file"} {
			for _, reportError := range []bool{false, true} {
				name := missing + "/" + toolName + "/success"
				if reportError {
					name = missing + "/" + toolName + "/error after publication"
				}
				t.Run(name, func(t *testing.T) {
					root := t.TempDir()
					path := filepath.Join(root, "note.txt")
					if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
						t.Fatal(err)
					}
					a := newUndoHookAgent(t, root)
					if missing == "task-session" {
						a.SetTaskStore(taskstate.NewStore(t.TempDir()))
					}
					if missing == "task-store" {
						if err := a.SetTaskSession("non-durable-user-edit"); err != nil {
							t.Fatal(err)
						}
					}
					args := map[string]string{"path": "note.txt", "content": "after"}
					if toolName == "edit_file" {
						args = map[string]string{"path": "note.txt", "old_string": "before", "new_string": "after"}
					}
					raw, err := json.Marshal(args)
					if err != nil {
						t.Fatal(err)
					}
					a.SetClient(&llm.Scripted{Responses: []llm.ChatResponse{
						{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "native-write", Name: toolName, Arguments: string(raw)}}}},
						{Message: llm.Message{Role: "assistant", Content: "done"}},
					}})
					a.runTool = func(ctx context.Context, call llm.ToolCall, tool tools.Tool, c tools.Context) (string, error) {
						if c.BeforeWorkspacePublish == nil {
							t.Fatal("non-durable native publication has no expectation hook")
						}
						out, err := tool.Run(ctx, call.Arguments, c)
						if err == nil && reportError {
							err = errors.New("simulated error after native publication")
						}
						return out, err
					}
					edited := false
					events := &undoSealToolEndHandler{onToolEnd: func(call llm.ToolCall, _ string, _ error) {
						if call.Name != toolName || edited {
							return
						}
						assertUndoFileContent(t, path, "after")
						if err := os.WriteFile(path, []byte("user"), 0o644); err != nil {
							t.Fatal(err)
						}
						edited = true
					}}
					_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update note.txt"}, events)
					if err != nil || !edited || !result.UndoAvailable || result.UndoError != "" {
						t.Fatalf("non-durable turn undo: edited=%v result=%+v err=%v", edited, result, err)
					}
					if a.latestUndo == nil || a.latestUndo.turnSequence != 0 || a.latestUndo.durable || a.TaskSnapshot() != nil {
						t.Fatalf("fixture unexpectedly created durable undo: undo=%+v task=%+v", a.latestUndo, a.TaskSnapshot())
					}
					if _, err := os.Stat(filepath.Join(root, ".picogent", "undo")); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("non-durable turn created an undo journal directory: %v", err)
					}
					if _, err := a.UndoLastTurn(); err == nil || !strings.Contains(err.Error(), "newer changes") {
						t.Fatalf("non-durable undo accepted the user's edit: %v", err)
					}
					assertUndoFileContent(t, path, "user")
					if !a.UndoAvailable() {
						t.Fatal("conflicted non-durable undo was consumed")
					}
				})
			}
		}
	}
}

func TestUndoSealRetainsEarlierPublicationThroughUserConflict(t *testing.T) {
	for _, fresh := range []bool{false, true} {
		name := "in-process"
		if fresh {
			name = "fresh-agent"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "note.txt")
			if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
				t.Fatal(err)
			}
			store := taskstate.NewStore(t.TempDir())
			first := newUndoHookAgent(t, root)
			first.SetTaskStore(store)
			const sessionID = "conflicted-later-publication"
			if err := first.SetTaskSession(sessionID); err != nil {
				t.Fatal(err)
			}
			first.SetClient(&llm.Scripted{Responses: []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{
					{ID: "first", Name: "write_file", Arguments: `{"path":"note.txt","content":"after"}`},
					{ID: "later", Name: "write_file", Arguments: `{"path":"note.txt","content":"second"}`},
				}}},
				{Message: llm.Message{Role: "assistant", Content: "done"}},
			}})
			var firstExpected, laterExpected string
			preparedLater := false
			first.runTool = func(ctx context.Context, call llm.ToolCall, tool tools.Tool, c tools.Context) (string, error) {
				if call.ID == "later" {
					prepare := c.BeforeWorkspacePublish
					if prepare == nil {
						t.Fatal("native publication has no recovery hook")
					}
					c.BeforeWorkspacePublish = func(path string, data []byte, mode os.FileMode) error {
						if err := prepare(path, data, mode); err != nil {
							return err
						}
						pending, err := loadUndoJournal(root, sessionID, true)
						if err != nil || len(pending.Checkpoint.Entries) != 1 || pending.Checkpoint.Entries[0].Published != firstExpected {
							t.Fatalf("later prepare lost earlier publication: journal=%+v err=%v", pending, err)
						}
						laterExpected = pending.Checkpoint.Entries[0].Expected
						preparedLater = true
						return errors.New("simulated failure before later publication")
					}
				}
				return tool.Run(ctx, call.Arguments, c)
			}
			edited := false
			events := &undoSealToolEndHandler{onToolEnd: func(call llm.ToolCall, _ string, toolErr error) {
				assertUndoFileContent(t, path, "after")
				if call.ID == "first" {
					pending, err := loadUndoJournal(root, sessionID, true)
					if err != nil || len(pending.Checkpoint.Entries) != 1 {
						t.Fatalf("first publication journal=%+v err=%v", pending, err)
					}
					firstExpected = pending.Checkpoint.Entries[0].Expected
					return
				}
				if call.ID != "later" || !preparedLater || toolErr == nil {
					t.Fatalf("later publication did not fail after prepare: call=%+v prepared=%v err=%v", call, preparedLater, toolErr)
				}
				if err := os.WriteFile(path, []byte("user"), 0o644); err != nil {
					t.Fatal(err)
				}
				edited = true
			}}
			_, result, err := first.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update note.txt twice"}, events)
			if err != nil || !edited || !result.UndoAvailable || result.UndoError != "" {
				t.Fatalf("earlier publication lost conflicted undo: edited=%v result=%+v err=%v", edited, result, err)
			}
			sealed, err := loadUndoJournal(root, sessionID, false)
			if err != nil || len(sealed.Checkpoint.Entries) != 1 || sealed.Checkpoint.Entries[0].Expected != laterExpected || sealed.Checkpoint.Entries[0].Published != firstExpected {
				t.Fatalf("sealed journal lost publication history: journal=%+v err=%v", sealed, err)
			}
			undo := first
			if fresh {
				undo = newUndoHookAgent(t, root)
				undo.SetTaskStore(store)
				if err := undo.SetTaskSession(sessionID); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := undo.UndoLastTurn(); err == nil || !strings.Contains(err.Error(), "newer changes") {
				t.Fatalf("undo accepted unknown user bytes: %v", err)
			}
			assertUndoFileContent(t, path, "user")
			if !undo.UndoAvailable() {
				t.Fatal("conflicted undo was consumed")
			}
			if err := os.WriteFile(path, []byte("after"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := undo.UndoLastTurn(); err != nil {
				t.Fatalf("undo retry rejected the known earlier publication: %v", err)
			}
			assertUndoFileContent(t, path, "before")
			if undo.UndoAvailable() {
				t.Fatal("completed undo remained available")
			}
		})
	}
}

func TestUndoSealRetainsEarlierPublicationAfterLaterWriteFailure(t *testing.T) {
	for _, fresh := range []bool{false, true} {
		name := "in-process"
		if fresh {
			name = "fresh-agent"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "note.txt")
			if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
				t.Fatal(err)
			}
			store := taskstate.NewStore(t.TempDir())
			first := newUndoHookAgent(t, root)
			first.SetTaskStore(store)
			const sessionID = "failed-later-publication"
			if err := first.SetTaskSession(sessionID); err != nil {
				t.Fatal(err)
			}
			first.SetClient(&llm.Scripted{Responses: []llm.ChatResponse{
				{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{
					{ID: "first", Name: "write_file", Arguments: `{"path":"note.txt","content":"after"}`},
					{ID: "later", Name: "write_file", Arguments: `{"path":"note.txt","content":"before"}`},
				}}},
				{Message: llm.Message{Role: "assistant", Content: "done"}},
			}})
			first.runTool = func(ctx context.Context, call llm.ToolCall, tool tools.Tool, c tools.Context) (string, error) {
				if call.ID == "later" {
					prepare := c.BeforeWorkspacePublish
					if prepare == nil {
						t.Fatal("native publication has no recovery hook")
					}
					c.BeforeWorkspacePublish = func(path string, data []byte, mode os.FileMode) error {
						if err := prepare(path, data, mode); err != nil {
							return err
						}
						// The recovery record was prepared, but this native
						// publication will be aborted before its rename.
						return errors.New("simulated failure before later publication")
					}
				}
				return tool.Run(ctx, call.Arguments, c)
			}
			_, result, err := first.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update note.txt twice"}, allowUndoTest{})
			if err != nil || !result.UndoAvailable || result.UndoError != "" {
				t.Fatalf("earlier publication lost undo: result=%+v err=%v", result, err)
			}
			assertUndoFileContent(t, path, "after")
			undo := first
			if fresh {
				undo = newUndoHookAgent(t, root)
				undo.SetTaskStore(store)
				if err := undo.SetTaskSession(sessionID); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := undo.UndoLastTurn(); err != nil {
				t.Fatal(err)
			}
			assertUndoFileContent(t, path, "before")
		})
	}
}

type undoSealUserEditHandler struct {
	allowUndoTest
	t         *testing.T
	path      string
	root      string
	sessionID string
	toolName  string
	expected  string
	edited    bool
}

func (h *undoSealUserEditHandler) OnToolEnd(call llm.ToolCall, _ string, _ error) {
	if call.Name != h.toolName || h.edited {
		return
	}
	// This callback runs after the real native rename and before finishTurnUndo.
	assertUndoFileContent(h.t, h.path, "after")
	pending, err := loadUndoJournal(h.root, h.sessionID, true)
	if err != nil || len(pending.Checkpoint.Entries) != 1 {
		h.t.Fatalf("native publication has no prepared recovery record: journal=%+v err=%v", pending, err)
	}
	h.expected = pending.Checkpoint.Entries[0].Expected
	if err := os.WriteFile(h.path, []byte("user"), 0o644); err != nil {
		h.t.Fatal(err)
	}
	h.edited = true
}

type undoSealToolEndHandler struct {
	allowUndoTest
	onToolEnd func(llm.ToolCall, string, error)
}

func (h *undoSealToolEndHandler) OnToolEnd(call llm.ToolCall, output string, err error) {
	h.onToolEnd(call, output, err)
}
