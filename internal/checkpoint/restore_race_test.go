package checkpoint

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
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
