package agent_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
)

func TestUndoPersistsAcrossFreshAgent(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "note.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	store := taskstate.NewStore(t.TempDir())
	args, err := json.Marshal(map[string]string{"path": "note.txt", "content": "after\n"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Mode = config.ModeFast
	cfg.Provider = config.ProviderOllama
	first := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{
		toolResponse("write", "write_file", json.RawMessage(args)),
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	first.SetTaskStore(store)
	if err := first.SetTaskSession("fresh-undo"); err != nil {
		t.Fatal(err)
	}
	_, result, err := first.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update note.txt"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.UndoAvailable {
		t.Fatalf("first process did not publish undo: %+v", result)
	}

	second := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	second.SetTaskStore(store)
	if err := second.SetTaskSession("fresh-undo"); err != nil {
		t.Fatal(err)
	}
	if !second.UndoAvailable() {
		t.Fatal("fresh process did not discover durable undo")
	}
	message, err := second.UndoLastTurn()
	if err != nil || !strings.Contains(message, "restored note.txt") {
		t.Fatalf("fresh-process undo = (%q, %v)", message, err)
	}
	assertFreshUndoFileContent(t, path, "before\n")
	if second.UndoAvailable() {
		t.Fatal("fresh-process undo remained available after recovery")
	}
	if _, err := os.Stat(filepath.Join(workspace, ".picogent", "undo", "fresh-undo.json")); !os.IsNotExist(err) {
		t.Fatalf("sealed undo journal remains after recovery: %v", err)
	}
}

func TestFreshUndoConflictPreservesNewerWorkspaceEdit(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "note.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := taskstate.NewStore(t.TempDir())
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Mode = config.ModeFast
	cfg.Provider = config.ProviderOllama
	args, _ := json.Marshal(map[string]string{"path": "note.txt", "content": "agent edit\n"})
	first := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{
		toolResponse("write", "write_file", json.RawMessage(args)),
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	first.SetTaskStore(store)
	if err := first.SetTaskSession("fresh-conflict"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := first.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update note"}, allowAll{}); err != nil {
		t.Fatal(err)
	}
	second := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	second.SetTaskStore(store)
	if err := second.SetTaskSession("fresh-conflict"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("newer user edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := second.UndoLastTurn(); err == nil || !strings.Contains(err.Error(), "newer changes") {
		t.Fatalf("fresh conflict = %v", err)
	}
	assertFreshUndoFileContent(t, path, "newer user edit\n")
	if !second.UndoAvailable() {
		t.Fatal("conflicted durable undo was discarded")
	}
}

func TestSupersededFreshUndoFailsClosed(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "note.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := taskstate.NewStore(t.TempDir())
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Mode = config.ModeFast
	cfg.Provider = config.ProviderOllama
	args, _ := json.Marshal(map[string]string{"path": "note.txt", "content": "agent edit\n"})
	first := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{
		toolResponse("write", "write_file", json.RawMessage(args)),
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	first.SetTaskStore(store)
	if err := first.SetTaskSession("superseded"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := first.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update note"}, allowAll{}); err != nil {
		t.Fatal(err)
	}

	task, err := store.Load("superseded")
	if err != nil {
		t.Fatal(err)
	}
	sequence, ok := task.BeginTurn(taskstate.TurnRouteImplement)
	if !ok || !task.FinishTurn(sequence, taskstate.TurnRouteImplement, "a later workspace mutation", "UNVERIFIED", taskstate.StopNone, 1, 1) {
		t.Fatal("could not record the later mutating turn")
	}
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}

	second := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	second.SetTaskStore(store)
	if err := second.SetTaskSession("superseded"); err != nil {
		t.Fatal(err)
	}
	if second.UndoAvailable() {
		t.Fatal("superseded durable undo was advertised as available")
	}
	if _, err := second.UndoLastTurn(); err == nil || !strings.Contains(err.Error(), "superseded") {
		t.Fatalf("superseded durable undo error = %v", err)
	}
	assertFreshUndoFileContent(t, path, "agent edit\n")
}

func TestMalformedFreshUndoFailsClosed(t *testing.T) {
	workspace := t.TempDir()
	store := taskstate.NewStore(t.TempDir())
	journalDir := filepath.Join(workspace, ".picogent", "undo")
	if err := os.MkdirAll(journalDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(journalDir, "malformed.json"), []byte(`{"version":1`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Mode = config.ModeFast
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	a.SetTaskStore(store)
	if err := a.SetTaskSession("malformed"); err != nil {
		t.Fatal(err)
	}
	if a.UndoAvailable() {
		t.Fatal("malformed journal was advertised as available")
	}
	if _, err := a.UndoLastTurn(); err == nil || !strings.Contains(err.Error(), "undo is unavailable") {
		t.Fatalf("malformed journal error = %v", err)
	}
}

func TestFreshUndoRetainsRecoveryWhenTaskStateIsMissing(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "note.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := taskstate.NewStore(t.TempDir())
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Mode = config.ModeFast
	cfg.Provider = config.ProviderOllama
	args, _ := json.Marshal(map[string]string{"path": "note.txt", "content": "after\n"})
	first := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{
		toolResponse("write", "write_file", json.RawMessage(args)),
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	first.SetTaskStore(store)
	if err := first.SetTaskSession("missing-task"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := first.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update note"}, allowAll{}); err != nil {
		t.Fatal(err)
	}
	taskPath, err := store.Path("missing-task")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(taskPath); err != nil {
		t.Fatal(err)
	}
	second := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	second.SetTaskStore(store)
	if err := second.SetTaskSession("missing-task"); err != nil {
		t.Fatal(err)
	}
	if _, err := second.UndoLastTurn(); err == nil || !strings.Contains(err.Error(), "durable task state is unavailable") {
		t.Fatalf("missing task state error = %v", err)
	}
	assertFreshUndoFileContent(t, path, "before\n")
	if !second.UndoAvailable() {
		t.Fatal("undo candidate was discarded after missing task state")
	}
}

func assertFreshUndoFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}
