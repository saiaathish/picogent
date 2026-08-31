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
