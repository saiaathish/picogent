package agent_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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

func TestCachedUndoRequiresOriginalTaskStoreAuthority(t *testing.T) {
	for _, scenario := range []string{"copied store", "same store setter", "store ABA"} {
		t.Run(scenario, func(t *testing.T) {
			workspace := t.TempDir()
			path := filepath.Join(workspace, "note.txt")
			if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			storeA := taskstate.NewStore(t.TempDir())
			storeB := taskstate.NewStore(t.TempDir())
			const sessionID = "store-bound-undo"
			cfg := config.Default()
			cfg.Workspace = workspace
			cfg.Mode = config.ModeFast
			cfg.Provider = config.ProviderOllama
			args, err := json.Marshal(map[string]string{"path": "note.txt", "content": "agent edit\n"})
			if err != nil {
				t.Fatal(err)
			}
			a := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{
				toolResponse("write", "write_file", json.RawMessage(args)),
				{Message: llm.Message{Role: "assistant", Content: "done"}},
			}}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
			a.SetTaskStore(storeA)
			if err := a.SetTaskSession(sessionID); err != nil {
				t.Fatal(err)
			}
			_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update note.txt"}, allowAll{})
			if err != nil {
				t.Fatal(err)
			}
			if !result.UndoAvailable {
				t.Fatalf("native write did not publish undo: %+v", result)
			}
			committedTask := a.TaskSnapshot()
			if committedTask == nil {
				t.Fatal("native write did not persist its task")
			}
			if scenario == "copied store" {
				storeAPath, err := storeA.Path(sessionID)
				if err != nil {
					t.Fatal(err)
				}
				storeBPath, err := storeB.Path(sessionID)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Dir(storeBPath), 0o700); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(storeAPath)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(storeBPath, data, 0o600); err != nil {
					t.Fatal(err)
				}
				a.SetTaskStore(storeB)
			} else if scenario == "same store setter" {
				a.SetTaskStore(storeA)
			} else {
				a.SetTaskStore(storeB)
				a.SetTaskStore(storeA)
			}

			if _, err := a.UndoLastTurn(); err == nil || !strings.Contains(err.Error(), "authority") {
				t.Fatalf("undo after %s = %v, want an authority refusal", scenario, err)
			}
			assertFreshUndoFileContent(t, path, "agent edit\n")
			if a.UndoAvailable() {
				t.Fatal("stale cached undo is still advertised")
			}
			if _, err := os.Stat(filepath.Join(workspace, ".picogent", "undo", sessionID+".json")); err != nil {
				t.Fatalf("stale cached undo removed its recovery journal: %v", err)
			}
			activeStore := storeA
			if scenario == "copied store" {
				activeStore = storeB
			}
			activeTask, err := activeStore.Load(sessionID)
			if err != nil {
				t.Fatalf("load active task after refused undo: %v", err)
			}
			if !reflect.DeepEqual(activeTask, committedTask) {
				t.Fatal("refused cached undo changed the active task store")
			}

			// Reattaching the session is the explicit recovery boundary that may
			// bind the journal to a new in-process store epoch after validation.
			if err := a.SetTaskSession(sessionID); err != nil {
				t.Fatalf("explicit session recovery admission: %v", err)
			}
			if !a.UndoAvailable() {
				t.Fatal("validated session reattachment did not recover undo")
			}
			if _, err := a.UndoLastTurn(); err != nil {
				t.Fatalf("undo after validated session reattachment: %v", err)
			}
			assertFreshUndoFileContent(t, path, "before\n")
		})
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
