package agent

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
	"github.com/saiaathish/picogent/internal/workspace"
)

func TestUndoSealDoesNotAdoptUserEditAfterUnpreparedFailure(t *testing.T) {
	for _, durable := range []bool{false, true} {
		for _, kind := range []string{"write-read-only", "edit-no-match"} {
			name := "process/" + kind
			if durable {
				name = "durable/" + kind
			}
			t.Run(name, func(t *testing.T) {
				if kind == "write-read-only" && runtime.GOOS == "windows" {
					t.Skip("Windows normalizes Unix write permissions")
				}
				root := t.TempDir()
				path := filepath.Join(root, "note.txt")
				mode := os.FileMode(0o644)
				if kind == "write-read-only" {
					mode = 0o444
				}
				if err := os.WriteFile(path, []byte("before"), mode); err != nil {
					t.Fatal(err)
				}
				a := newUndoHookAgent(t, root)
				if durable {
					a.SetTaskStore(taskstate.NewStore(t.TempDir()))
					if err := a.SetTaskSession("unprepared-failure"); err != nil {
						t.Fatal(err)
					}
				}
				toolName := "write_file"
				args := `{"path":"note.txt","content":"agent"}`
				if kind == "edit-no-match" {
					toolName = "edit_file"
					args = `{"path":"note.txt","old_string":"missing","new_string":"agent"}`
				}
				a.SetClient(&llm.Scripted{Responses: []llm.ChatResponse{
					{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "failed", Name: toolName, Arguments: args}}}},
					{Message: llm.Message{Role: "assistant", Content: "The write failed."}},
				}})
				prepared := false
				a.runTool = func(ctx context.Context, call llm.ToolCall, tool tools.Tool, c tools.Context) (string, error) {
					hook := c.BeforeWorkspacePublish
					c.BeforeWorkspacePublish = func(path string, data []byte, mode os.FileMode) error {
						prepared = true
						return hook(path, data, mode)
					}
					return tool.Run(ctx, call.Arguments, c)
				}
				edited := false
				events := &undoSealToolEndHandler{onToolEnd: func(call llm.ToolCall, _ string, err error) {
					if call.ID != "failed" {
						return
					}
					if err == nil || prepared {
						t.Fatalf("fixture did not fail before publication: err=%v prepared=%v", err, prepared)
					}
					if err := os.Chmod(path, 0o644); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte("user"), 0o644); err != nil {
						t.Fatal(err)
					}
					edited = true
				}}
				_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update note.txt"}, events)
				if err != nil || !edited || result.UndoAvailable || len(result.FilesChanged) != 0 {
					t.Fatalf("unprepared failure adopted user bytes: edited=%v result=%+v err=%v", edited, result, err)
				}
				if message, err := a.UndoLastTurn(); err != nil || message != "nothing to undo" {
					t.Fatalf("unprepared undo = %q, %v", message, err)
				}
				assertUndoFileContent(t, path, "user")
			})
		}
	}
}

func TestUndoSealRetainsProcessUndoAfterRejectedLaterPublication(t *testing.T) {
	for _, kind := range []string{"write-hook", "edit-content-conflict"} {
		t.Run(kind, func(t *testing.T) { testProcessUndoAfterRejectedPublication(t, kind) })
	}
}

func testProcessUndoAfterRejectedPublication(t *testing.T, kind string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newUndoHookAgent(t, root)
	if kind == "edit-content-conflict" {
		a.runTool = func(ctx context.Context, call llm.ToolCall, tool tools.Tool, c tools.Context) (string, error) {
			if call.ID == "later" {
				// Model a native edit's final compare detecting a user edit
				// after its read. No new publication belongs to this call.
				return "", workspace.ErrContentConflict
			}
			return tool.Run(ctx, call.Arguments, c)
		}
	}
	laterTool := "write_file"
	if kind == "edit-content-conflict" {
		laterTool = "edit_file"
	}
	a.SetClient(&llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "first", Name: "write_file", Arguments: `{"path":"note.txt","content":"A"}`},
			{ID: "later", Name: laterTool, Arguments: `{"path":"note.txt","content":"B"}`},
		}}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}})
	edited := false
	events := &undoSealToolEndHandler{onToolEnd: func(call llm.ToolCall, _ string, err error) {
		if call.ID == "first" {
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("user"), 0o644); err != nil {
				t.Fatal(err)
			}
			edited = true
		} else if call.ID == "later" && err == nil {
			t.Fatal("later publication did not reject the user edit")
		}
	}}
	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update note.txt twice"}, events)
	if err != nil || !edited || !result.UndoAvailable {
		t.Fatalf("earlier process undo disappeared: edited=%v result=%+v err=%v", edited, result, err)
	}
	if _, err := a.UndoLastTurn(); err == nil || !strings.Contains(err.Error(), "newer changes") {
		t.Fatalf("undo overwrote unknown user bytes: %v", err)
	}
	assertUndoFileContent(t, path, "user")
	if err := os.WriteFile(path, []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.UndoLastTurn(); err != nil {
		t.Fatalf("known earlier publication not undoable: %v", err)
	}
	assertUndoFileContent(t, path, "before")
}
