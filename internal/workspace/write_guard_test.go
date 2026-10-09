package workspace_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/workspace"
)

func assertNoStagedWrite(t *testing.T, root string) {
	t.Helper()
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(entry.Name(), ".picogent-workspace-") {
			t.Errorf("aborted write retained staged file: %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestWriteGuardRefusesBeforeParentCreation(t *testing.T) {
	root := t.TempDir()
	refused := errors.New("authority revoked")
	prepared := false
	err := workspace.WriteAtomicWithHooks(root, filepath.Join(root, "new", "nested", "note.txt"), []byte("after"), workspace.WriteHooks{
		Check:          func() error { return refused },
		PreparePublish: func(os.FileMode) error { prepared = true; return nil },
	})
	if !errors.Is(err, refused) || prepared {
		t.Fatalf("initial guard refusal = %v, prepared=%v", err, prepared)
	}
	if _, err := os.Stat(filepath.Join(root, "new")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("refused write created parents: %v", err)
	}
	assertNoStagedWrite(t, root)
}

func TestWriteGuardRechecksBeforeStaging(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	refused := errors.New("authority revoked before staging")
	checks := 0
	err := workspace.WriteAtomicWithHooks(root, path, []byte("after"), workspace.WriteHooks{Check: func() error {
		checks++
		if checks >= 2 {
			return refused
		}
		return nil
	}})
	if !errors.Is(err, refused) || checks != 2 {
		t.Fatalf("staging guard = %v, checks=%d", err, checks)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("revoked write published a file")
	}
	assertNoStagedWrite(t, root)
}

func TestWriteGuardRechecksAfterPreparationForWritesAndEdits(t *testing.T) {
	for _, edit := range []bool{false, true} {
		name := "write"
		if edit {
			name = "edit"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "note.txt")
			if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
				t.Fatal(err)
			}
			refused := errors.New("authority revoked after preparation")
			prepared := false
			hooks := workspace.WriteHooks{
				Check: func() error {
					if prepared {
						return refused
					}
					return nil
				},
				PreparePublish: func(os.FileMode) error { prepared = true; return nil },
			}
			var err error
			if edit {
				err = workspace.WriteAtomicIfUnchangedWithHooks(root, path, []byte("before"), []byte("after"), hooks)
			} else {
				err = workspace.WriteAtomicWithHooks(root, path, []byte("after"), hooks)
			}
			if !errors.Is(err, refused) || !prepared {
				t.Fatalf("post-preparation refusal = %v, prepared=%v", err, prepared)
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != "before" {
				t.Fatalf("refused publication changed target: %q %v", got, err)
			}
			assertNoStagedWrite(t, root)
		})
	}
}

func TestWritePublicationStaysOnOpenedWorkspaceAfterRootSwap(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("agent\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	parked := filepath.Join(parent, "parked-workspace")
	checks := 0
	err := workspace.WriteAtomicIfUnchangedWithHooks(root, "note.txt", []byte("agent\n"), []byte("before\n"), workspace.WriteHooks{
		Check: func() error {
			checks++
			if checks == 3 {
				if err := os.Rename(root, parked); err != nil {
					return err
				}
				if err := os.Mkdir(root, 0o700); err != nil {
					return err
				}
				if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("agent\n"), 0o640); err != nil {
					return err
				}
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("atomic restore after root swap: %v", err)
	}
	if checks < 3 {
		t.Fatalf("publication checks = %d, want final descriptor-anchored check", checks)
	}
	if got, err := os.ReadFile(filepath.Join(root, "note.txt")); err != nil || string(got) != "agent\n" {
		t.Fatalf("replacement workspace file = %q, err=%v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(parked, "note.txt")); err != nil || string(got) != "before\n" {
		t.Fatalf("original workspace file = %q, err=%v", got, err)
	}
}

func TestRemoveWithCheckStaysOnOpenedWorkspaceAfterRootSwap(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pending.json"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	parked := filepath.Join(parent, "parked-workspace")
	checks := 0
	err := workspace.RemoveWithCheck(root, "pending.json", func() error {
		checks++
		if err := os.Rename(root, parked); err != nil {
			return err
		}
		if err := os.Mkdir(root, 0o700); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(root, "pending.json"), []byte("replacement"), 0o600)
	})
	if err != nil {
		t.Fatalf("remove after root swap: %v", err)
	}
	if checks != 1 {
		t.Fatalf("removal checks = %d, want one check after parent open", checks)
	}
	if got, err := os.ReadFile(filepath.Join(root, "pending.json")); err != nil || string(got) != "replacement" {
		t.Fatalf("replacement journal = %q, err=%v", got, err)
	}
	if _, err := os.Stat(filepath.Join(parked, "pending.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("original journal was not removed: %v", err)
	}
}
