package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
	"github.com/saiaathish/picogent/internal/workspace"
)

func TestUndoCapturesToolThatMutatesThenReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newUndoHookAgent(t, dir)
	a.runTool = func(_ context.Context, call llm.ToolCall, _ tools.Tool, c tools.Context) (string, error) {
		if call.Name != "write_file" {
			t.Fatalf("unexpected tool %q", call.Name)
		}
		// Native integrations must announce exact bytes before publication;
		// an error alone cannot distinguish their writes from a user edit.
		if err := c.BeforeWorkspacePublish(path, []byte("partial mutation"), 0o644); err != nil {
			return "", err
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

func TestEditContentConflictDoesNotCreateUndo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]string{
		"path":       "note.txt",
		"old_string": "before",
		"new_string": "agent edit",
	})
	a := newUndoHookAgent(t, dir)
	a.SetClient(&llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "edit", Name: "edit_file", Arguments: string(args)}}}},
		{Message: llm.Message{Role: "assistant", Content: "the file changed; edit was not applied"}},
	}})
	a.runTool = func(_ context.Context, call llm.ToolCall, _ tools.Tool, _ tools.Context) (string, error) {
		if call.Name != "edit_file" {
			t.Fatalf("unexpected tool %q", call.Name)
		}
		if err := os.WriteFile(path, []byte("newer user edit"), 0o644); err != nil {
			return "", err
		}
		// The real edit tool can return this wrapped error when its compare
		// check observes a concurrent user edit. Keep the agent-level test
		// deterministic; the workspace package covers the filesystem race
		// boundary itself.
		return "", fmt.Errorf("edit became stale: %w", workspace.ErrContentConflict)
	}

	_, res, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "edit note"}, allowUndoTest{})
	if err != nil {
		t.Fatal(err)
	}
	if res.UndoAvailable || a.UndoAvailable() {
		t.Fatalf("content-conflict edit created undo state: %+v", res)
	}
	if msg, err := a.UndoLastTurn(); err != nil || msg != "nothing to undo" {
		t.Fatalf("undo after content conflict = (%q, %v)", msg, err)
	}
	assertUndoFileContent(t, path, "newer user edit")
}

func TestMixedWriteAndContentConflictDoesNotUndoNewerPath(t *testing.T) {
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "first.txt")
	conflictPath := filepath.Join(dir, "conflict.txt")
	if err := os.WriteFile(firstPath, []byte("first before"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(conflictPath, []byte("conflict before"), 0o644); err != nil {
		t.Fatal(err)
	}

	firstArgs, _ := json.Marshal(map[string]string{"path": "first.txt", "content": "first agent"})
	conflictArgs, _ := json.Marshal(map[string]string{
		"path":       "conflict.txt",
		"old_string": "conflict before",
		"new_string": "conflict agent",
	})
	a := newUndoHookAgent(t, dir)
	a.SetTaskStore(taskstate.NewStore(t.TempDir()))
	if err := a.SetTaskSession("mixed-content-conflict"); err != nil {
		t.Fatal(err)
	}
	a.SetClient(&llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "first", Name: "write_file", Arguments: string(firstArgs)},
			{ID: "conflict", Name: "edit_file", Arguments: string(conflictArgs)},
		}}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}})
	runs := 0
	a.runTool = func(ctx context.Context, call llm.ToolCall, tool tools.Tool, c tools.Context) (string, error) {
		runs++
		if runs == 2 {
			if err := os.WriteFile(conflictPath, []byte("newer user edit"), 0o644); err != nil {
				return "", err
			}
			return "", fmt.Errorf("edit became stale: %w", workspace.ErrContentConflict)
		}
		return tool.Run(ctx, call.Arguments, c)
	}

	_, res, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update both files"}, allowUndoTest{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.UndoAvailable || len(res.FilesChanged) != 1 || res.FilesChanged[0] != "first.txt" {
		t.Fatalf("mixed content-conflict undo state: %+v", res)
	}
	if _, err := a.UndoLastTurn(); err != nil {
		t.Fatal(err)
	}
	assertUndoFileContent(t, firstPath, "first before")
	assertUndoFileContent(t, conflictPath, "newer user edit")
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
	a.runTool = func(_ context.Context, _ llm.ToolCall, _ tools.Tool, c tools.Context) (string, error) {
		if err := c.BeforeWorkspacePublish(path, []byte("intended regular file"), 0o644); err != nil {
			return "", err
		}
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

func TestFailedJournalPublicationDoesNotAdvertiseProcessLocalUndo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := newUndoHookAgent(t, dir)
	a.SetTaskStore(taskstate.NewStore(t.TempDir()))
	if err := a.SetTaskSession("failed-journal"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".picogent"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".picogent", "undo"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	a.runTool = func(_ context.Context, _ llm.ToolCall, _ tools.Tool, c tools.Context) (string, error) {
		// Simulate a broken integration that ignores a rejected recovery
		// journal and still publishes. It must not obtain process-only undo
		// by bypassing the durable publication contract.
		if err := c.BeforeWorkspacePublish(path, []byte("after"), 0o644); err == nil {
			t.Fatal("fixture did not reject recovery journal publication")
		}
		if err := os.WriteFile(path, []byte("after"), 0o644); err != nil {
			return "", err
		}
		return "wrote note.txt", nil
	}

	_, res, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update note"}, allowUndoTest{})
	if err != nil {
		t.Fatal(err)
	}
	if res.UndoAvailable || a.UndoAvailable() {
		t.Fatalf("failed journal publication was advertised as undoable: %+v", res)
	}
	if res.UndoError == "" || !strings.Contains(res.UndoError, "durable undo journal") {
		t.Fatalf("missing durable publication error: %+v", res)
	}
}

func TestUndoRejectsWorkspaceRebindBeforeUndoLock(t *testing.T) {
	oldWorkspace, newWorkspace := trailingWhitespaceWorkspacePair(t)
	a := newUndoHookAgent(t, oldWorkspace)
	defer a.Close()
	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "write note.txt"}, allowUndoTest{})
	if err != nil || !result.UndoAvailable {
		t.Fatalf("fixture did not create an undo checkpoint: available=%v err=%v", result.UndoAvailable, err)
	}
	path := filepath.Join(oldWorkspace, "note.txt")
	if got, err := os.ReadFile(path); err != nil || string(got) != "after" {
		t.Fatalf("fixture file = %q, %v", got, err)
	}
	_, err = a.undoLastTurnWithHook(func() {
		a.UpdateConfig(func(cfg *config.Config) { cfg.Workspace = newWorkspace })
	})
	if !errors.Is(err, errWorkspaceAuthorityChanged) {
		t.Fatalf("undo did not reject the stale workspace snapshot: %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "after" {
		t.Fatalf("stale undo changed the original workspace: %q, %v", got, err)
	}
	if a.latestUndo == nil {
		t.Fatal("stale undo discarded its recovery checkpoint")
	}
}

func TestSetTaskSessionRejectsWorkspaceRebindBeforeUndoLock(t *testing.T) {
	oldWorkspace, newWorkspace := trailingWhitespaceWorkspacePair(t)
	a := newUndoHookAgent(t, oldWorkspace)
	defer a.Close()
	store := taskstate.NewStore(t.TempDir())
	a.SetTaskStore(store)
	if err := a.SetTaskSession("original-session"); err != nil {
		t.Fatal(err)
	}
	beforeSession, beforeGeneration := a.taskSessionSnapshot()
	err := a.setTaskSessionWithHook("replacement-session", func() {
		a.UpdateConfig(func(cfg *config.Config) { cfg.Workspace = newWorkspace })
	})
	if !errors.Is(err, errWorkspaceAuthorityChanged) {
		t.Fatalf("session attachment did not reject the stale workspace snapshot: %v", err)
	}
	afterSession, afterGeneration := a.taskSessionSnapshot()
	if afterSession != beforeSession || afterGeneration != beforeGeneration || a.TaskStoreSnapshot() != store || a.TaskSnapshot() != nil {
		t.Fatalf("refused attachment changed task authority: session=%q generation=%d store_same=%v task=%+v", afterSession, afterGeneration, a.TaskStoreSnapshot() == store, a.TaskSnapshot())
	}
}

func TestUpdateConfigWaitsForUndoAuthorityWindow(t *testing.T) {
	oldWorkspace, newWorkspace := t.TempDir(), t.TempDir()
	a := newUndoHookAgent(t, oldWorkspace)
	defer a.Close()

	// Undo holds undoMu from its authority check through journal finalization.
	// A workspace update must not cross that protected interval.
	a.undoMu.Lock()
	locked := true
	defer func() {
		if locked {
			a.undoMu.Unlock()
		}
	}()
	started, finished := make(chan struct{}), make(chan struct{})
	go func() {
		close(started)
		a.UpdateConfig(func(cfg *config.Config) { cfg.Workspace = newWorkspace })
		close(finished)
	}()
	<-started
	select {
	case <-finished:
		t.Fatal("workspace config crossed active undo authority")
	case <-time.After(25 * time.Millisecond):
	}
	a.undoMu.Unlock()
	locked = false
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("workspace config did not resume after undo authority released")
	}
	if got := a.ConfigSnapshot().Workspace; got != newWorkspace {
		t.Fatalf("workspace = %q, want %q", got, newWorkspace)
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

func trailingWhitespaceWorkspacePair(t *testing.T) (string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Win32 paths do not preserve trailing spaces in directory names")
	}
	parent := t.TempDir()
	workspace := filepath.Join(parent, "workspace")
	trimmedDistinctWorkspace := workspace + " "
	for _, path := range []string{workspace, trimmedDistinctWorkspace} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatalf("create workspace %q: %v", path, err)
		}
	}
	if strings.TrimSpace(workspace) != strings.TrimSpace(trimmedDistinctWorkspace) {
		t.Fatal("fixture paths must compare equal after trimming whitespace")
	}
	return workspace, trimmedDistinctWorkspace
}

func TestSameUndoWorkspaceIdentityAcceptsPathAliases(t *testing.T) {
	workspace := t.TempDir()
	var alias string
	if runtime.GOOS == "windows" {
		alias = strings.ToUpper(workspace)
	} else {
		alias = filepath.Join(filepath.Dir(workspace), filepath.Base(workspace)+"-alias")
		if err := os.Symlink(workspace, alias); err != nil {
			t.Skipf("create workspace alias: %v", err)
		}
	}
	workspaceInfo, err := os.Stat(workspace)
	if err != nil {
		t.Fatal(err)
	}
	aliasInfo, err := os.Stat(alias)
	if err != nil {
		t.Fatalf("stat workspace alias: %v", err)
	}
	if !os.SameFile(workspaceInfo, aliasInfo) {
		t.Skip("the platform did not resolve the candidate path as a filesystem alias")
	}
	if !sameUndoWorkspaceIdentity(workspace, alias) {
		t.Fatalf("canonical workspace identity rejected filesystem aliases %q and %q", workspace, alias)
	}
}

type allowUndoTest struct{ NopHandler }

func (allowUndoTest) OnNeedPermission(context.Context, perm.Request) (perm.Decision, error) {
	return perm.Allow, nil
}
