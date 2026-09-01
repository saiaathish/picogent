package benchmark_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/checkpoint"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
	"github.com/saiaathish/picogent/internal/workspace"
)

var (
	benchmarkStageBefore = []byte("before\n")
	benchmarkStageAfter  = []byte("after\n")
)

// BenchmarkScriptedAgentEditStageRunLock measures the project serialization
// boundary used by Agent.Run as a standalone primitive control, independently
// from model and workspace work.
func BenchmarkScriptedAgentEditStageRunLock(b *testing.B) {
	root := b.TempDir()
	store := taskstate.WorkspaceStore(root)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		release, err := store.AcquireRunLock()
		if err != nil {
			b.Fatal(err)
		}
		if err := release(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkScriptedAgentEditStageCheckpointCapture measures the pre-edit
// snapshot taken before a native write is allowed to run as a standalone
// primitive control.
func BenchmarkScriptedAgentEditStageCheckpointCapture(b *testing.B) {
	root := b.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, benchmarkStageBefore, 0o644); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := checkpoint.Capture(root, []string{"note.txt"}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkScriptedAgentEditStageCheckpointSeal measures the post-write
// fingerprint pass that turns a pending checkpoint into an undo checkpoint as
// a standalone primitive control.
func BenchmarkScriptedAgentEditStageCheckpointSeal(b *testing.B) {
	root := b.TempDir()
	path := filepath.Join(root, "note.txt")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		if err := os.WriteFile(path, benchmarkStageBefore, 0o644); err != nil {
			b.Fatal(err)
		}
		cp, err := checkpoint.Capture(root, []string{"note.txt"})
		if err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(path, benchmarkStageAfter, 0o644); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		if err := cp.Seal(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkScriptedAgentEditStageWorkspacePublish measures the secure atomic
// publication used by the write_file tool with an empty publication hook. It
// is a standalone non-durable-turn primitive control; the production-shaped
// durable path is measured by BenchmarkScriptedAgentEditStageDurableTurn.
func BenchmarkScriptedAgentEditStageWorkspacePublish(b *testing.B) {
	root := b.TempDir()
	path := filepath.Join(root, "note.txt")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		if err := os.WriteFile(path, benchmarkStageBefore, 0o644); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		if err := workspace.WriteAtomicWithPublishHook(root, path, benchmarkStageAfter, func(os.FileMode) error { return nil }); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkScriptedAgentEditStageTaskStateSave measures one task-state file
// save after the initial task file exists as a standalone persistence control.
func BenchmarkScriptedAgentEditStageTaskStateSave(b *testing.B) {
	root := b.TempDir()
	store := taskstate.NewStore(filepath.Join(root, "tasks"))
	task, err := taskstate.New("benchmark-session", "benchmark scripted edit", nil)
	if err != nil {
		b.Fatal(err)
	}
	if err := store.Save(task); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := store.Save(task); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkScriptedAgentEditStageDurableTurn measures the production-shaped
// scripted write path. Unlike the primitive controls above, each operation
// runs Agent.Run with workspace-local durable task state, a real turn sequence,
// the pre-publication undo journal hook, durable mutation recording, and the
// normal turn close. Undo is intentionally outside the timed region, matching
// BenchmarkScriptedAgentEdit's existing boundary while still proving that the
// durable checkpoint can be consumed.
func BenchmarkScriptedAgentEditStageDurableTurn(b *testing.B) {
	root := b.TempDir()
	path := filepath.Join(root, "note.txt")
	args, err := json.Marshal(map[string]string{"path": "note.txt", "content": "after\n"})
	if err != nil {
		b.Fatal(err)
	}
	store := taskstate.WorkspaceStore(root)
	b.ReportAllocs()
	b.ResetTimer()
	lastCalls := 0
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		if err := os.WriteFile(path, benchmarkStageBefore, 0o644); err != nil {
			b.Fatal(err)
		}
		sessionID := fmt.Sprintf("benchmark-durable-%d", i)
		fake := &llm.Scripted{Responses: []llm.ChatResponse{
			toolResponse("write", "write_file", string(args)),
			{Message: llm.Message{Role: "assistant", Content: "done"}},
		}}
		cfg := config.Default()
		cfg.Workspace = root
		cfg.Mode = config.ModeFast
		cfg.Provider = config.ProviderOllama
		a := agent.New(cfg, fake, tools.NewRegistry(tools.Context{Workspace: root}), perm.New(config.ModeFast, root, nil))
		a.SetTaskStore(store)
		if err := a.SetTaskSession(sessionID); err != nil {
			b.Fatal(err)
		}

		b.StartTimer()
		_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update note.txt"}, benchmarkAllow{})
		b.StopTimer()
		if err != nil {
			b.Fatal(err)
		}
		if !result.UndoAvailable {
			b.Fatal("durable scripted edit did not produce an undo checkpoint")
		}
		persisted, err := store.Load(sessionID)
		if err != nil {
			b.Fatal(err)
		}
		last := persisted.LastTurn()
		if last == nil || last.State != taskstate.TurnCompleted || last.MutationCount != 1 || len(last.ChangedFiles) != 1 || last.ChangedFiles[0] != "note.txt" {
			b.Fatalf("durable turn = %#v, want one completed note.txt mutation", last)
		}
		lastCalls = len(fake.Calls)
		if _, err := a.UndoLastTurn(); err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
	}
	b.StopTimer()
	b.ReportMetric(float64(lastCalls), "model-calls/op")
}
