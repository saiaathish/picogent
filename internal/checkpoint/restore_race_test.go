package checkpoint

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRestoreRechecksPathBeforePublishing(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "note.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	cp, err := Capture(workspace, []string{"note.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("agent"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cp.Seal(); err != nil {
		t.Fatal(err)
	}
	cp.restoreBeforeApply = func(rel string) {
		if err := os.WriteFile(filepath.Join(workspace, rel), []byte("newer user edit"), 0o644); err != nil {
			t.Fatalf("write concurrent edit: %v", err)
		}
		cp.restoreBeforeApply = nil
	}

	result, err := cp.Restore()
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("restore error = %v, want conflict", err)
	}
	if result.Complete || len(result.Conflicts) != 1 || result.Conflicts[0].Path != "note.txt" {
		t.Fatalf("restore result = %+v", result)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "newer user edit" {
		t.Fatalf("concurrent edit = %q, err=%v", got, err)
	}
}

func TestRestoreRefusesReplacementWorkspaceBeforePublishing(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	cp, err := Capture(root, []string{"note.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("agent"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cp.Seal(); err != nil {
		t.Fatal(err)
	}
	parked := filepath.Join(parent, "parked-workspace")
	cp.restoreBeforeApply = func(rel string) {
		cp.restoreBeforeApply = nil
		if err := os.Rename(root, parked); err != nil {
			t.Fatalf("park original workspace: %v", err)
		}
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatalf("create replacement workspace: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, rel), []byte("replacement post-turn"), 0o644); err != nil {
			t.Fatalf("write replacement workspace: %v", err)
		}
	}

	result, err := cp.Restore()
	if !errors.Is(err, ErrWorkspaceChanged) {
		t.Fatalf("restore error = %v, want workspace identity change", err)
	}
	if result.Complete || result.RolledBack || len(result.Failures) == 0 {
		t.Fatalf("replacement restore result = %+v", result)
	}
	if got, err := os.ReadFile(filepath.Join(root, "note.txt")); err != nil || string(got) != "replacement post-turn" {
		t.Fatalf("replacement file = %q, err=%v", got, err)
	}
}

func TestRestoreDoesNotConsumeNoOpCheckpointAfterTransientWorkspaceReplacement(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	cp, err := Capture(root, []string{"note.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("agent"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cp.Seal(); err != nil {
		t.Fatal(err)
	}
	parked := filepath.Join(parent, "parked-workspace")
	transient := filepath.Join(parent, "transient-replacement")
	cp.restoreAfterInitialIdentityCheck = func() {
		cp.restoreAfterInitialIdentityCheck = nil
		if err := os.Rename(root, parked); err != nil {
			t.Fatalf("park original workspace: %v", err)
		}
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatalf("create transient replacement: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("before"), 0o644); err != nil {
			t.Fatalf("write transient pre-turn state: %v", err)
		}
	}
	cp.restoreBeforePreflightComplete = func() {
		cp.restoreBeforePreflightComplete = nil
		if err := os.Rename(root, transient); err != nil {
			t.Fatalf("park transient replacement: %v", err)
		}
		if err := os.Rename(parked, root); err != nil {
			t.Fatalf("restore original workspace: %v", err)
		}
	}

	result, err := cp.Restore()
	if !errors.Is(err, ErrConflict) || result.Complete || len(result.Conflicts) != 1 {
		t.Fatalf("transient-replacement restore = result:%+v err:%v", result, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "agent" {
		t.Fatalf("original workspace changed after rejected no-op: %q, %v", got, err)
	}

	result, err = cp.Restore()
	if err != nil || !result.Complete || len(result.Restored) != 1 {
		t.Fatalf("retry after transient replacement = result:%+v err:%v", result, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "before" {
		t.Fatalf("retry did not restore original workspace: %q, %v", got, err)
	}
}

func TestRestorePublicationRechecksDurableWorkspaceGuard(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	cp, err := Capture(root, []string{"note.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("agent"), 0o644); err != nil {
		t.Fatal(err)
	}
	current, err := readWorkspaceFile(root, "note.txt")
	if err != nil {
		t.Fatal(err)
	}
	markerPath := filepath.Join(root, ".picogent", "workspace-instance")
	if err := os.MkdirAll(filepath.Dir(markerPath), 0o700); err != nil {
		t.Fatal(err)
	}
	const expectedToken = "expected-workspace-token"
	if err := os.WriteFile(markerPath, []byte(expectedToken+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	checks := 0
	guard := func() error {
		checks++
		if checks == 3 {
			if err := os.WriteFile(markerPath, []byte("replacement-workspace-token\n"), 0o600); err != nil {
				return err
			}
		}
		data, err := os.ReadFile(markerPath)
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(data)) != expectedToken {
			return fmt.Errorf("workspace marker token changed")
		}
		return nil
	}

	err = writeWorkspaceState(root, "note.txt", current, cp.entries[0].before, cp.rootIdentity, guard)
	if !errors.Is(err, ErrWorkspaceChanged) {
		t.Fatalf("restore publication error = %v, want workspace guard rejection", err)
	}
	if checks != 3 {
		t.Fatalf("workspace guard checks = %d, want the pre-publication check", checks)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "agent" {
		t.Fatalf("workspace file changed after guard rejection: %q, %v", got, err)
	}
}

func TestRestoreTreatsAlreadyRestoredPathAsUnchanged(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "note.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	cp, err := Capture(workspace, []string{"note.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("agent"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cp.Seal(); err != nil {
		t.Fatal(err)
	}
	cp.restoreBeforeApply = func(rel string) {
		if err := os.WriteFile(filepath.Join(workspace, rel), []byte("before"), 0o644); err != nil {
			t.Fatalf("complete concurrent restore: %v", err)
		}
		cp.restoreBeforeApply = nil
	}

	result, err := cp.Restore()
	if err != nil || !result.Complete || len(result.Unchanged) != 1 || result.Unchanged[0] != "note.txt" {
		t.Fatalf("already-restored result = %+v err:%v", result, err)
	}
}

func TestRestoreRechecksModeBeforePublishing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not preserve Unix permission bits")
	}
	workspace := t.TempDir()
	path := filepath.Join(workspace, "note.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	cp, err := Capture(workspace, []string{"note.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("agent"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cp.Seal(); err != nil {
		t.Fatal(err)
	}
	cp.restoreBeforeApply = func(rel string) {
		if err := os.Chmod(filepath.Join(workspace, rel), 0o600); err != nil {
			t.Fatalf("change concurrent mode: %v", err)
		}
		cp.restoreBeforeApply = nil
	}

	result, err := cp.Restore()
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("restore error = %v, want conflict", err)
	}
	if result.Complete || len(result.Conflicts) != 1 || result.Conflicts[0].Path != "note.txt" {
		t.Fatalf("restore result = %+v", result)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("concurrent mode = %o, want 600", got)
	}
}

func TestRestoreDoesNotReplaceConcurrentRecreation(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "note.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	cp, err := Capture(workspace, []string{"note.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := cp.Seal(); err != nil {
		t.Fatal(err)
	}
	cp.restoreBeforeApply = func(rel string) {
		if err := os.WriteFile(filepath.Join(workspace, rel), []byte("newer user recreation"), 0o644); err != nil {
			t.Fatalf("recreate concurrent file: %v", err)
		}
		cp.restoreBeforeApply = nil
	}

	result, err := cp.Restore()
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("restore error = %v, want conflict", err)
	}
	if result.Complete || len(result.Conflicts) != 1 || result.Conflicts[0].Path != "note.txt" {
		t.Fatalf("restore result = %+v", result)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "newer user recreation" {
		t.Fatalf("concurrent recreation = %q, err=%v", got, err)
	}
}
