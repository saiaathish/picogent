package goal

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDoesNotCreateStateUntilSet(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "missing", "picogent")
	t.Setenv("PICOGENT_HOME", home)
	ws := t.TempDir()
	got, err := Load(ws)
	if err != nil || got != "" {
		t.Fatalf("Load: %q err=%v", got, err)
	}
	if _, err := os.Stat(home); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load created state directory: %v", err)
	}
	if err := Set(ws, "finish the release"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "goals")); err != nil {
		t.Fatalf("Set did not create state directory: %v", err)
	}
}

func TestSetLoadClear(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PICOGENT_HOME", root)
	ws := filepath.Join(root, "proj")

	if err := Set(ws, "fix flaky tests"); err != nil {
		t.Fatal(err)
	}
	got, err := Load(ws)
	if err != nil || got != "fix flaky tests" {
		t.Fatalf("load: %q err=%v", got, err)
	}
	if err := Clear(ws); err != nil {
		t.Fatal(err)
	}
	got, _ = Load(ws)
	if got != "" {
		t.Fatalf("expected empty after clear, got %q", got)
	}
}

func TestGoalRevisionPreventsSameTextABA(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PICOGENT_HOME", root)
	ws := filepath.Join(root, "proj")

	first, err := SetState(ws, "finish this project")
	if err != nil {
		t.Fatal(err)
	}
	second, err := SetState(ws, "finish this project")
	if err != nil {
		t.Fatal(err)
	}
	if first == 0 || second == 0 || first == second {
		t.Fatalf("revisions did not advance: first=%d second=%d", first, second)
	}
	staleResult := make(chan struct {
		cleared bool
		err     error
	}, 1)
	releaseStaleCompletion := make(chan struct{})
	go func() {
		<-releaseStaleCompletion
		cleared, err := ClearIfState(ws, "finish this project", first)
		staleResult <- struct {
			cleared bool
			err     error
		}{cleared: cleared, err: err}
	}()
	close(releaseStaleCompletion)
	stale := <-staleResult
	if stale.err != nil || stale.cleared {
		t.Fatalf("stale revision cleared goal: cleared=%v err=%v", stale.cleared, stale.err)
	}
	if cleared, err := ClearIfState(ws, "finish this project", second); err != nil || !cleared {
		t.Fatalf("current revision did not clear goal: cleared=%v err=%v", cleared, err)
	}
}

func TestGoalRevisionSurvivesClearBeforeSameTextReissue(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PICOGENT_HOME", root)
	ws := filepath.Join(root, "proj")

	first, err := SetState(ws, "finish this project")
	if err != nil {
		t.Fatal(err)
	}
	if err := Clear(ws); err != nil {
		t.Fatal(err)
	}
	second, err := SetState(ws, "finish this project")
	if err != nil {
		t.Fatal(err)
	}
	if second <= first {
		t.Fatalf("clear reset revision: first=%d second=%d", first, second)
	}
	if cleared, err := ClearIfState(ws, "finish this project", first); err != nil || cleared {
		t.Fatalf("pre-clear completion cleared reissued goal: cleared=%v err=%v", cleared, err)
	}
}

func TestGoalTombstoneAndBackupRecoveryPreserveRevision(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PICOGENT_HOME", root)
	ws := filepath.Join(root, "proj")

	first, err := SetState(ws, "finish this project")
	if err != nil {
		t.Fatal(err)
	}
	if err := Clear(ws); err != nil {
		t.Fatal(err)
	}
	tombstone, err := LoadState(ws)
	if err != nil || tombstone.Text != "" || tombstone.Revision != first {
		t.Fatalf("tombstone = %#v err=%v, want revision %d", tombstone, err, first)
	}

	second, err := SetState(ws, "finish this project")
	if err != nil || second <= first {
		t.Fatalf("reissued revision = %d err=%v, want > %d", second, err, first)
	}
	path, err := readPath(ws)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, stateBackupPath(path)); err != nil {
		t.Fatal(err)
	}
	recovered, err := LoadState(ws)
	if err != nil || recovered.Text != "finish this project" || recovered.Revision != second {
		t.Fatalf("recovered state = %#v err=%v, want revision %d", recovered, err, second)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("recovery did not restore primary record: %v", err)
	}
}

func TestLegacyGoalGetsRevisionOnLoad(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PICOGENT_HOME", root)
	ws := filepath.Join(root, "proj")
	path, err := storePath(ws)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("legacy goal"), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := LoadState(ws)
	if err != nil || state.Text != "legacy goal" || state.Revision == 0 {
		t.Fatalf("legacy state was not upgraded: %#v err=%v", state, err)
	}
	oldRevision := state.Revision
	newRevision, err := SetState(ws, "legacy goal")
	if err != nil || newRevision == oldRevision {
		t.Fatalf("legacy replacement did not advance revision: old=%d new=%d err=%v", oldRevision, newRevision, err)
	}
	if cleared, err := ClearIfState(ws, "legacy goal", oldRevision); err != nil || cleared {
		t.Fatalf("stale legacy revision cleared replacement: cleared=%v err=%v", cleared, err)
	}
	if cleared, err := ClearIfState(ws, "legacy goal", newRevision); err != nil || !cleared {
		t.Fatalf("upgraded goal did not clear: cleared=%v err=%v", cleared, err)
	}
}

func TestClearIfPreservesNewerGoal(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PICOGENT_HOME", root)
	ws := filepath.Join(root, "proj")

	if err := Set(ws, "finish this project"); err != nil {
		t.Fatal(err)
	}
	if cleared, err := ClearIf(ws, "a newer project goal"); err != nil || cleared {
		t.Fatal(err)
	}
	if got, _ := Load(ws); got != "finish this project" {
		t.Fatalf("stale ClearIf removed goal: %q", got)
	}
	if cleared, err := ClearIf(ws, "finish this project"); err != nil || !cleared {
		t.Fatal(err)
	}
	if got, _ := Load(ws); got != "" {
		t.Fatalf("matching ClearIf kept goal: %q", got)
	}
}

func TestLooksComplete(t *testing.T) {
	if !LooksComplete("Goal complete: tests green") {
		t.Fatal("prefix")
	}
	if LooksComplete("still working") {
		t.Fatal("false positive")
	}
}
