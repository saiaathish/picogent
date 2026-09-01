package checkpoint

import (
	"errors"
	"os"
	"path/filepath"
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
