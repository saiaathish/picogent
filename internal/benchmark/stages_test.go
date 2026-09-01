package benchmark_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/saiaathish/picogent/internal/checkpoint"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/workspace"
)

var (
	benchmarkStageBefore = []byte("before\n")
	benchmarkStageAfter  = []byte("after\n")
)

// BenchmarkScriptedAgentEditStageRunLock measures the project serialization
// boundary used by Agent.Run independently from model and workspace work.
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
// snapshot taken before a native write is allowed to run.
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
// fingerprint pass that turns a pending checkpoint into an undo checkpoint.
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
// publication used by the write_file tool, with its publication-hook seam
// present but empty as it is for a non-durable turn.
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

// BenchmarkScriptedAgentEditStageTaskStateSave measures one durable task
// mutation persistence after the initial task file exists.
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
