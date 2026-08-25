package checkpoint_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"

	"github.com/saiaathish/picogent/internal/checkpoint"
)

func TestRestoreReturnsFilesToPreTurnState(t *testing.T) {
	workspace := t.TempDir()
	write(t, workspace, "existing.txt", "before", 0o640)
	write(t, workspace, "deleted.txt", "bring me back", 0o600)
	write(t, workspace, "unrelated.txt", "hands off", 0o644)

	cp, err := checkpoint.Capture(workspace, []string{"existing.txt", "deleted.txt", "created.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if got := cp.Paths(); !slices.Equal(got, []string{"existing.txt", "deleted.txt", "created.txt"}) {
		t.Fatalf("paths=%v", got)
	}

	write(t, workspace, "existing.txt", "after", 0o700)
	if err := os.Remove(filepath.Join(workspace, "deleted.txt")); err != nil {
		t.Fatal(err)
	}
	write(t, workspace, "created.txt", "new", 0o644)
	if err := cp.Seal(); err != nil {
		t.Fatal(err)
	}

	result, err := cp.Restore()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.RolledBack || len(result.Conflicts) != 0 || len(result.Failures) != 0 {
		t.Fatalf("result=%+v", result)
	}
	if !slices.Equal(result.Restored, []string{"deleted.txt", "existing.txt"}) {
		t.Fatalf("restored=%v", result.Restored)
	}
	if !slices.Equal(result.Removed, []string{"created.txt"}) {
		t.Fatalf("removed=%v", result.Removed)
	}
	assertContents(t, workspace, "existing.txt", "before")
	assertContents(t, workspace, "deleted.txt", "bring me back")
	assertContents(t, workspace, "unrelated.txt", "hands off")
	if _, err := os.Stat(filepath.Join(workspace, "created.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("created file still exists: %v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(workspace, "existing.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o640 {
			t.Fatalf("mode=%o", got)
		}
	}
}

func TestRestoreDetectsConflictBeforeChangingAnyPath(t *testing.T) {
	workspace := t.TempDir()
	write(t, workspace, "a.txt", "a-before", 0o644)
	write(t, workspace, "b.txt", "b-before", 0o644)
	cp, err := checkpoint.Capture(workspace, []string{"a.txt", "b.txt"})
	if err != nil {
		t.Fatal(err)
	}
	write(t, workspace, "a.txt", "a-agent", 0o644)
	write(t, workspace, "b.txt", "b-agent", 0o644)
	if err := cp.Seal(); err != nil {
		t.Fatal(err)
	}

	write(t, workspace, "b.txt", "b-user", 0o644)
	result, err := cp.Restore()
	if !errors.Is(err, checkpoint.ErrConflict) {
		t.Fatalf("error=%v", err)
	}
	if result.Complete || len(result.Conflicts) != 1 || result.Conflicts[0].Path != "b.txt" {
		t.Fatalf("result=%+v", result)
	}
	assertContents(t, workspace, "a.txt", "a-agent")
	assertContents(t, workspace, "b.txt", "b-user")
}

func TestRestoreRequiresSealAndRunsOnce(t *testing.T) {
	workspace := t.TempDir()
	write(t, workspace, "file.txt", "before", 0o644)
	cp, err := checkpoint.Capture(workspace, []string{"file.txt"})
	if err != nil {
		t.Fatal(err)
	}
	write(t, workspace, "file.txt", "after", 0o644)
	if _, err := cp.Restore(); !errors.Is(err, checkpoint.ErrNotSealed) {
		t.Fatalf("unsealed restore error=%v", err)
	}
	if err := cp.Seal(); err != nil {
		t.Fatal(err)
	}
	if err := cp.Seal(); !errors.Is(err, checkpoint.ErrAlreadySealed) {
		t.Fatalf("second seal error=%v", err)
	}
	if _, err := cp.Restore(); err != nil {
		t.Fatal(err)
	}
	if _, err := cp.Restore(); !errors.Is(err, checkpoint.ErrAlreadyRestored) {
		t.Fatalf("second restore error=%v", err)
	}
}

func TestConcurrentRestoreHasOneWinnerAndOneShotLoser(t *testing.T) {
	workspace := t.TempDir()
	write(t, workspace, "file.txt", "before", 0o644)
	cp, err := checkpoint.Capture(workspace, []string{"file.txt"})
	if err != nil {
		t.Fatal(err)
	}
	write(t, workspace, "file.txt", "after", 0o644)
	if err := cp.Seal(); err != nil {
		t.Fatal(err)
	}

	type restoreResult struct {
		result checkpoint.RestoreResult
		err    error
	}
	start := make(chan struct{})
	results := make(chan restoreResult, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for range 2 {
		go func() {
			ready.Done()
			<-start
			result, err := cp.Restore()
			results <- restoreResult{result: result, err: err}
		}()
	}
	ready.Wait()
	close(start)

	winners, losers := 0, 0
	for range 2 {
		out := <-results
		switch {
		case out.err == nil:
			winners++
			if !out.result.Complete {
				t.Fatalf("successful restore was not complete: %#v", out.result)
			}
		case errors.Is(out.err, checkpoint.ErrAlreadyRestored):
			losers++
		default:
			t.Fatalf("unexpected concurrent restore error: %v", out.err)
		}
	}
	if winners != 1 || losers != 1 {
		t.Fatalf("concurrent restore outcomes = winners:%d losers:%d", winners, losers)
	}
	assertContents(t, workspace, "file.txt", "before")
}

func TestAddCapturesMorePathsDeduplicatesAndRejectsAfterSeal(t *testing.T) {
	workspace := t.TempDir()
	write(t, workspace, "a.txt", "a-before", 0o644)
	write(t, workspace, "b.txt", "b-before", 0o644)
	cp, err := checkpoint.Capture(workspace, []string{"a.txt"})
	if err != nil {
		t.Fatal(err)
	}
	write(t, workspace, "a.txt", "a-agent", 0o644)
	if err := cp.Add([]string{"./a.txt", "b.txt", "b.txt"}); err != nil {
		t.Fatal(err)
	}
	if got := cp.Paths(); !slices.Equal(got, []string{"a.txt", "b.txt"}) {
		t.Fatalf("paths=%v", got)
	}
	write(t, workspace, "b.txt", "b-agent", 0o644)
	if err := cp.Seal(); err != nil {
		t.Fatal(err)
	}
	if got, err := cp.ChangedPaths(); err != nil || !slices.Equal(got, []string{"a.txt", "b.txt"}) {
		t.Fatalf("changed paths=(%v, %v)", got, err)
	}
	if err := cp.Add([]string{"c.txt"}); !errors.Is(err, checkpoint.ErrAlreadySealed) {
		t.Fatalf("add after seal error=%v", err)
	}
	if _, err := cp.Restore(); err != nil {
		t.Fatal(err)
	}
	assertContents(t, workspace, "a.txt", "a-before")
	assertContents(t, workspace, "b.txt", "b-before")
}

func TestChangedPathsRequiresSealAndOmitsUnchanged(t *testing.T) {
	workspace := t.TempDir()
	write(t, workspace, "changed.txt", "before", 0o644)
	write(t, workspace, "same.txt", "same", 0o644)
	cp, err := checkpoint.Capture(workspace, []string{"changed.txt", "same.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cp.ChangedPaths(); !errors.Is(err, checkpoint.ErrNotSealed) {
		t.Fatalf("unsealed changed paths error=%v", err)
	}
	write(t, workspace, "changed.txt", "after", 0o644)
	if err := cp.Seal(); err != nil {
		t.Fatal(err)
	}
	if got, err := cp.ChangedPaths(); err != nil || !slices.Equal(got, []string{"changed.txt"}) {
		t.Fatalf("changed paths=(%v, %v)", got, err)
	}
}

func TestModeOnlyChangesAreRestored(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not preserve Unix permission bits")
	}
	workspace := t.TempDir()
	write(t, workspace, "script.sh", "echo ok\n", 0o644)
	cp, err := checkpoint.Capture(workspace, []string{"script.sh"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(workspace, "script.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := cp.Seal(); err != nil {
		t.Fatal(err)
	}
	result, err := cp.Restore()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(result.Restored, []string{"script.sh"}) {
		t.Fatalf("result=%+v", result)
	}
	info, err := os.Stat(filepath.Join(workspace, "script.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("mode=%o", got)
	}
}

func TestCaptureRejectsWorkspaceEscapesAndNonFiles(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	write(t, outside, "outside.txt", "private", 0o644)

	tests := []struct {
		name string
		path string
	}{
		{name: "relative escape", path: "../outside.txt"},
		{name: "absolute escape", path: filepath.Join(outside, "outside.txt")},
		{name: "workspace root", path: "."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := checkpoint.Capture(workspace, []string{tt.path}); err == nil {
				t.Fatal("expected path rejection")
			}
		})
	}

	if err := os.Symlink(outside, filepath.Join(workspace, "escape")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("symlink creation requires privileges on Windows")
		}
		t.Fatal(err)
	}
	if _, err := checkpoint.Capture(workspace, []string{"escape/outside.txt"}); err == nil {
		t.Fatal("expected symlink escape rejection")
	}
	if _, err := checkpoint.Capture(workspace, []string{"escape"}); err == nil {
		t.Fatal("expected symlink file rejection")
	}
}

func TestUnchangedCheckpointReportsNoMutation(t *testing.T) {
	workspace := t.TempDir()
	write(t, workspace, "same.txt", "same", 0o644)
	cp, err := checkpoint.Capture(workspace, []string{"same.txt", "same.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if got := cp.Paths(); !slices.Equal(got, []string{"same.txt"}) {
		t.Fatalf("paths=%v", got)
	}
	if err := cp.Seal(); err != nil {
		t.Fatal(err)
	}
	result, err := cp.Restore()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || !slices.Equal(result.Unchanged, []string{"same.txt"}) {
		t.Fatalf("result=%+v", result)
	}
}

func write(t *testing.T, workspace, rel, contents string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(workspace, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func assertContents(t *testing.T, workspace, rel, want string) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(workspace, rel))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s=%q want %q", rel, got, want)
	}
}
