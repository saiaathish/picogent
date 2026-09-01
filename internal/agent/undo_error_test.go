package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
)

func TestUndoCapturesToolThatMutatesThenReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newUndoHookAgent(t, dir)
	a.runTool = func(_ context.Context, call llm.ToolCall, _ tools.Tool, _ tools.Context) (string, error) {
		if call.Name != "write_file" {
			t.Fatalf("unexpected tool %q", call.Name)
		}
		if err := os.WriteFile(path, []byte("partial mutation"), 0o644); err != nil {
			return "", err
		}
		return "", errors.New("simulated failure after mutation")
	}

	_, res, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update note"}, allowUndoTest{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.UndoAvailable || len(res.FilesChanged) != 1 || res.FilesChanged[0] != "note.txt" {
		t.Fatalf("mutating error undo state: %+v", res)
	}
	if !strings.Contains(res.Text, "Undo: /undo") {
		t.Fatalf("footer=%q", res.Text)
	}
	if _, err := a.UndoLastTurn(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "before" {
		t.Fatalf("restored content=%q", got)
	}
}

func TestRejectedPublishDoesNotCreateUndo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newUndoHookAgent(t, dir)
	a.SetTaskStore(taskstate.NewStore(t.TempDir()))
	if err := a.SetTaskSession("rejected-publish"); err != nil {
		t.Fatal(err)
	}
	a.runTool = func(ctx context.Context, call llm.ToolCall, tool tools.Tool, c tools.Context) (string, error) {
		if call.Name != "write_file" {
			t.Fatalf("unexpected tool %q", call.Name)
		}
		if err := os.WriteFile(path, []byte("user edit"), 0o644); err != nil {
			return "", err
		}
		return tool.Run(ctx, call.Arguments, c)
	}

	_, res, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update note"}, allowUndoTest{})
	if err != nil {
		t.Fatal(err)
	}
	if res.UndoAvailable || a.UndoAvailable() {
		t.Fatalf("rejected publish created undo state: %+v", res)
	}
	if msg, err := a.UndoLastTurn(); err != nil || msg != "nothing to undo" {
		t.Fatalf("undo after rejected publish = (%q, %v)", msg, err)
	}
	assertUndoFileContent(t, path, "user edit")
}

func TestRejectedLaterPublishPreservesEarlierUndo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newUndoHookAgent(t, dir)
	a.SetTaskStore(taskstate.NewStore(t.TempDir()))
	if err := a.SetTaskSession("rejected-later-publish"); err != nil {
		t.Fatal(err)
	}
	firstArgs, _ := json.Marshal(map[string]string{"path": "note.txt", "content": "first"})
	secondArgs, _ := json.Marshal(map[string]string{"path": "note.txt", "content": "second"})
	a.SetClient(&llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "1", Name: "write_file", Arguments: string(firstArgs)},
			{ID: "2", Name: "write_file", Arguments: string(secondArgs)},
		}}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}})
	runs := 0
	a.runTool = func(ctx context.Context, call llm.ToolCall, tool tools.Tool, c tools.Context) (string, error) {
		runs++
		if runs == 2 {
			if err := os.WriteFile(path, []byte("user edit"), 0o644); err != nil {
				return "", err
			}
		}
		return tool.Run(ctx, call.Arguments, c)
	}

	_, res, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update note twice"}, allowUndoTest{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.UndoAvailable || len(res.FilesChanged) != 1 {
		t.Fatalf("earlier undo state was not retained: %+v", res)
	}
	if _, err := a.UndoLastTurn(); err == nil || !strings.Contains(err.Error(), "newer changes") {
		t.Fatalf("undo did not preserve the external edit: %v", err)
	}
	assertUndoFileContent(t, path, "user edit")
}

func TestSealFailureReportsUndoUnavailable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("target"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newUndoHookAgent(t, dir)
	a.runTool = func(_ context.Context, _ llm.ToolCall, _ tools.Tool, _ tools.Context) (string, error) {
		if err := os.Symlink(target, path); err != nil {
			return "", err
		}
		return "wrote note.txt", nil
	}

	_, res, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "write note"}, allowUndoTest{})
	if err != nil {
		t.Fatal(err)
	}
	if res.UndoAvailable || res.UndoError == "" {
		t.Fatalf("expected unavailable undo, got %+v", res)
	}
	if !strings.Contains(res.Text, "Undo: unavailable") {
		t.Fatalf("missing unavailable footer: %q", res.Text)
	}
}

func newUndoHookAgent(t *testing.T, dir string) *Agent {
	t.Helper()
	args, _ := json.Marshal(map[string]string{"path": "note.txt", "content": "after"})
	client := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "1", Name: "write_file", Arguments: string(args)}}}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}}
	cfg := config.Default()
	cfg.Workspace = dir
	cfg.Mode = config.ModeFast
	cfg.Provider = config.ProviderOllama
	return New(cfg, client, tools.NewRegistry(tools.Context{Workspace: dir}), perm.New(config.ModeFast, dir, nil))
}

type allowUndoTest struct{ NopHandler }

func (allowUndoTest) OnNeedPermission(context.Context, perm.Request) (perm.Decision, error) {
	return perm.Allow, nil
}
