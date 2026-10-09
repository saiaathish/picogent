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

func TestUndoRefusesCopiedWorkspaceInstance(t *testing.T) {
	parent := t.TempDir()
	workspace := filepath.Join(parent, "workspace")
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(workspace, "note.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	store := taskstate.NewStore(t.TempDir())
	const sessionID = "copied-workspace-undo"
	args, err := json.Marshal(map[string]string{"path": "note.txt", "content": "after\n"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Mode = config.ModeFast
	cfg.Provider = config.ProviderOllama
	writer := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{
		toolResponse("write", "write_file", json.RawMessage(args)),
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	writer.SetTaskStore(store)
	if err := writer.SetTaskSession(sessionID); err != nil {
		t.Fatal(err)
	}
	_, result, err := writer.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update note.txt"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.UndoAvailable {
		t.Fatalf("writer did not publish durable undo: %+v", result)
	}

	markerPath := filepath.Join(workspace, ".picogent", "workspace-instance")
	journalPath := filepath.Join(workspace, ".picogent", "undo", sessionID+".json")
	markerData, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatal(err)
	}
	journalData, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	parked := filepath.Join(parent, "parked-workspace")
	if err := os.Rename(workspace, parked); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "note.txt"), []byte("after\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	markerPath = filepath.Join(workspace, ".picogent", "workspace-instance")
	journalPath = filepath.Join(workspace, ".picogent", "undo", sessionID+".json")
	if err := os.MkdirAll(filepath.Dir(markerPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(markerPath, markerData, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(journalPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(journalPath, journalData, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := writer.UndoLastTurn(); err == nil || !strings.Contains(err.Error(), "workspace instance") {
		t.Fatalf("undo against copied workspace = %v, want workspace-instance rejection", err)
	}
	assertFreshUndoFileContent(t, filepath.Join(workspace, "note.txt"), "after\n")
	if after, err := os.ReadFile(journalPath); err != nil || !reflect.DeepEqual(after, journalData) {
		t.Fatalf("copied workspace journal changed: err=%v unchanged=%v", err, reflect.DeepEqual(after, journalData))
	}
}

func TestUndoRevalidatesOwnerAfterStaleSessionAttachment(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "note.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	store := taskstate.NewStore(t.TempDir())
	const sessionID = "stale-undo-attachment"
	args, err := json.Marshal(map[string]string{"path": "note.txt", "content": "after\n"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Mode = config.ModeFast
	cfg.Provider = config.ProviderOllama
	writer := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{
		toolResponse("write", "write_file", json.RawMessage(args)),
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	writer.SetTaskStore(store)
	if err := writer.SetTaskSession(sessionID); err != nil {
		t.Fatal(err)
	}

	// This Agent attaches before the writer creates durable task ownership.
	// Its in-memory TaskSnapshot stays nil even after the writer publishes undo.
	reader := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	reader.SetTaskStore(store)
	if err := reader.SetTaskSession(sessionID); err != nil {
		t.Fatal(err)
	}
	if reader.TaskSnapshot() != nil || reader.UndoAvailable() {
		t.Fatal("reader unexpectedly observed task ownership before the writer ran")
	}

	_, result, err := writer.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update note.txt"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.UndoAvailable {
		t.Fatalf("writer did not publish durable undo: %+v", result)
	}
	message, err := reader.UndoLastTurn()
	if err != nil || !strings.Contains(message, "restored note.txt") {
		t.Fatalf("undo from stale attachment = (%q, %v)", message, err)
	}
	assertFreshUndoFileContent(t, path, "before\n")
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

func TestFreshUndoRejectsDifferentTaskWithSameSessionAndTurn(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "note.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const sessionID = "same-session-owner-collision"
	storeA := taskstate.NewStore(t.TempDir())
	storeB := taskstate.NewStore(t.TempDir())
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Mode = config.ModeFast
	cfg.Provider = config.ProviderOllama
	args, err := json.Marshal(map[string]string{"path": "note.txt", "content": "agent edit\n"})
	if err != nil {
		t.Fatal(err)
	}
	first := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{
		toolResponse("write", "write_file", json.RawMessage(args)),
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	first.SetTaskStore(storeA)
	if err := first.SetTaskSession(sessionID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := first.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update note.txt"}, allowAll{}); err != nil {
		t.Fatal(err)
	}
	owner := first.TaskSnapshot()
	if owner == nil || owner.LastTurn() == nil {
		t.Fatal("native write did not persist its task and turn")
	}
	sealed := filepath.Join(workspace, ".picogent", "undo", sessionID+".json")
	journalBefore, err := os.ReadFile(sealed)
	if err != nil {
		t.Fatal(err)
	}

	unrelated, err := taskstate.New(sessionID, "unrelated task", nil)
	if err != nil {
		t.Fatal(err)
	}
	unrelated.IntentRevision = owner.LastTurn().IntentRevision
	sequence, ok := unrelated.BeginTurn(taskstate.TurnRouteImplement)
	if !ok || sequence != owner.LastTurn().Sequence {
		t.Fatalf("collision fixture sequence = %d, want %d", sequence, owner.LastTurn().Sequence)
	}
	if err := storeB.Save(unrelated); err != nil {
		t.Fatal(err)
	}
	unrelatedPath, err := storeB.Path(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	unrelatedBytesBefore, err := os.ReadFile(unrelatedPath)
	if err != nil {
		t.Fatal(err)
	}
	unrelatedBefore, err := storeB.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}

	first.SetTaskStore(storeB)
	if err := first.SetTaskSession(sessionID); err == nil || !strings.Contains(err.Error(), "task identity mismatch") {
		t.Fatalf("reattachment to unrelated task = %v", err)
	}
	if first.UndoAvailable() {
		t.Fatal("undo for a different task was advertised")
	}
	if _, err := first.UndoLastTurn(); err == nil || !strings.Contains(err.Error(), "reattach") {
		t.Fatalf("undo after rejected reattachment = %v", err)
	}
	assertFreshUndoFileContent(t, path, "agent edit\n")
	unrelatedAfter, err := storeB.Load(sessionID)
	if err != nil || !reflect.DeepEqual(unrelatedBefore, unrelatedAfter) {
		t.Fatalf("rejected recovery changed unrelated task: err=%v unchanged=%v", err, reflect.DeepEqual(unrelatedBefore, unrelatedAfter))
	}
	unrelatedBytesAfter, err := os.ReadFile(unrelatedPath)
	if err != nil || !reflect.DeepEqual(unrelatedBytesBefore, unrelatedBytesAfter) {
		t.Fatalf("rejected recovery changed replacement task bytes: err=%v unchanged=%v", err, reflect.DeepEqual(unrelatedBytesBefore, unrelatedBytesAfter))
	}
	ownerAfter, err := storeA.Load(sessionID)
	if err != nil || !reflect.DeepEqual(owner, ownerAfter) {
		t.Fatalf("rejected recovery changed original task: err=%v unchanged=%v", err, reflect.DeepEqual(owner, ownerAfter))
	}
	journalAfter, err := os.ReadFile(sealed)
	if err != nil || !reflect.DeepEqual(journalBefore, journalAfter) {
		t.Fatalf("rejected recovery changed journal: err=%v unchanged=%v", err, reflect.DeepEqual(journalBefore, journalAfter))
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
	if err := second.SetTaskSession("superseded"); err == nil || !strings.Contains(err.Error(), "superseded") {
		t.Fatalf("session attachment with superseded undo = %v", err)
	}
	if second.UndoAvailable() {
		t.Fatal("superseded durable undo was advertised as available")
	}
	if _, err := second.UndoLastTurn(); err == nil {
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
	if err := a.SetTaskSession("malformed"); err == nil || !strings.Contains(err.Error(), "decode undo journal") {
		t.Fatalf("session attachment with malformed journal = %v", err)
	}
	if a.UndoAvailable() {
		t.Fatal("malformed journal was advertised as available")
	}
	if _, err := a.UndoLastTurn(); err == nil || !strings.Contains(err.Error(), "undo is unavailable") {
		t.Fatalf("malformed journal error = %v", err)
	}
}

func TestFreshUndoFailsClosedWhenTaskOwnerIsMissing(t *testing.T) {
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
	sealed := filepath.Join(workspace, ".picogent", "undo", "missing-task.json")
	journalBefore, err := os.ReadFile(sealed)
	if err != nil {
		t.Fatal(err)
	}
	second := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	second.SetTaskStore(store)
	if err := second.SetTaskSession("missing-task"); err == nil || !strings.Contains(err.Error(), "owner validation") {
		t.Fatalf("session attachment with missing task owner = %v", err)
	}
	if _, err := second.UndoLastTurn(); err == nil || !strings.Contains(err.Error(), "owner validation") {
		t.Fatalf("missing task state error = %v", err)
	}
	assertFreshUndoFileContent(t, path, "after\n")
	if second.UndoAvailable() {
		t.Fatal("undo candidate without a durable owner was advertised")
	}
	journalAfter, err := os.ReadFile(sealed)
	if err != nil || !reflect.DeepEqual(journalBefore, journalAfter) {
		t.Fatalf("missing task owner changed recovery journal: err=%v unchanged=%v", err, reflect.DeepEqual(journalBefore, journalAfter))
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
