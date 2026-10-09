package workspace_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
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

func TestWriteIfUnchangedBindsCompareToExpectedRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	replacement := filepath.Join(parent, "replacement")
	for _, dir := range []string{root, replacement} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("newer user edit"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(replacement, "note.txt"), []byte("expected"), 0o600); err != nil {
		t.Fatal(err)
	}
	expected, err := workspace.DirectoryIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	parkedOriginal := filepath.Join(parent, "parked-original")
	parkedReplacement := filepath.Join(parent, "parked-replacement")
	checks := 0
	err = workspace.WriteAtomicIfUnchangedWithHooks(root, "note.txt", []byte("expected"), []byte("restored"), workspace.WriteHooks{
		Check: func() error {
			checks++
			if checks == 1 {
				if err := os.Rename(root, parkedOriginal); err != nil {
					return err
				}
				return os.Rename(replacement, root)
			}
			if checks == 2 {
				if err := os.Rename(root, parkedReplacement); err != nil {
					return err
				}
				return os.Rename(parkedOriginal, root)
			}
			return nil
		},
		CheckRootIdentity: func(actual workspace.Identity) error {
			if actual != expected {
				return workspace.ErrRootIdentityChanged
			}
			return nil
		},
	})
	if !errors.Is(err, workspace.ErrRootIdentityChanged) {
		t.Fatalf("compare across replacement workspace = %v, want root-identity rejection", err)
	}
	if checks != 1 {
		t.Fatalf("authority checks = %d, want rejection before publication", checks)
	}
	if got, err := os.ReadFile(filepath.Join(root, "note.txt")); err != nil || string(got) != "expected" {
		t.Fatalf("replacement workspace file = %q, %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(parkedOriginal, "note.txt")); err != nil || string(got) != "newer user edit" {
		t.Fatalf("original workspace file = %q, %v", got, err)
	}
}

func TestWriteIfMissingBindsAbsenceCheckToExpectedRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	replacement := filepath.Join(parent, "replacement")
	for _, dir := range []string{root, replacement} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("newer user edit"), 0o600); err != nil {
		t.Fatal(err)
	}
	expected, err := workspace.DirectoryIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	parkedOriginal := filepath.Join(parent, "parked-original")
	parkedReplacement := filepath.Join(parent, "parked-replacement")
	checks := 0
	err = workspace.WriteAtomicIfMissingWithModeAndHooks(root, "note.txt", []byte("created"), 0o600, workspace.WriteHooks{
		Check: func() error {
			checks++
			if checks == 1 {
				if err := os.Rename(root, parkedOriginal); err != nil {
					return err
				}
				return os.Rename(replacement, root)
			}
			if checks == 2 {
				if err := os.Rename(root, parkedReplacement); err != nil {
					return err
				}
				return os.Rename(parkedOriginal, root)
			}
			return nil
		},
		CheckRootIdentity: func(actual workspace.Identity) error {
			if actual != expected {
				return workspace.ErrRootIdentityChanged
			}
			return nil
		},
	})
	if !errors.Is(err, workspace.ErrRootIdentityChanged) {
		t.Fatalf("absence check across replacement workspace = %v, want root-identity rejection", err)
	}
	if checks != 1 {
		t.Fatalf("authority checks = %d, want rejection before publication", checks)
	}
	if _, err := os.Stat(filepath.Join(root, "note.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement workspace gained a file: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(parkedOriginal, "note.txt")); err != nil || string(got) != "newer user edit" {
		t.Fatalf("original workspace file = %q, %v", got, err)
	}
}

func TestWritePublicationRejectsWorkspaceSwapAfterCompare(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not permit this directory swap while publication handles are open; replacement-before-open coverage runs on Windows")
	}
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("agent\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	expected, err := workspace.DirectoryIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	parked := filepath.Join(parent, "parked-workspace")
	checks := 0
	swapped := false
	err = workspace.WriteAtomicIfUnchangedWithHooks(root, "note.txt", []byte("agent\n"), []byte("before\n"), workspace.WriteHooks{
		Check: func() error {
			checks++
			return nil
		},
		CheckRootIdentity: func(actual workspace.Identity) error {
			if actual != expected {
				return workspace.ErrRootIdentityChanged
			}
			if swapped {
				return nil
			}
			if err := os.Rename(root, parked); err != nil {
				return err
			}
			if err := os.Mkdir(root, 0o700); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("agent\n"), 0o640); err != nil {
				return err
			}
			swapped = true
			return nil
		},
	})
	if !errors.Is(err, workspace.ErrRootIdentityChanged) {
		t.Fatalf("atomic restore after root swap = %v, want root-identity rejection", err)
	}
	if !swapped {
		t.Fatal("workspace root was not replaced after the original root handle opened")
	}
	if checks != 2 {
		t.Fatalf("publication checks = %d, want compare and pre-publication checks", checks)
	}
	if got, err := os.ReadFile(filepath.Join(root, "note.txt")); err != nil || string(got) != "agent\n" {
		t.Fatalf("replacement workspace file = %q, err=%v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(parked, "note.txt")); err != nil || string(got) != "agent\n" {
		t.Fatalf("original workspace file = %q, err=%v", got, err)
	}
}

func TestRemoveRefusesReplacementAfterRootHandleOpen(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not permit this directory swap while removal handles are open; replacement-before-open coverage runs on Windows")
	}
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pending.json"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	expected, err := workspace.DirectoryIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	parked := filepath.Join(parent, "parked-workspace")
	checks := 0
	swapped := false
	err = workspace.RemoveWithChecks(root, "pending.json", func() error {
		checks++
		if !swapped {
			return nil
		}
		actual, err := workspace.DirectoryIdentity(root)
		if err != nil {
			return err
		}
		if actual != expected {
			return workspace.ErrRootIdentityChanged
		}
		return nil
	}, func(actual workspace.Identity) error {
		if actual != expected {
			return workspace.ErrRootIdentityChanged
		}
		if err := os.Rename(root, parked); err != nil {
			return err
		}
		if err := os.Mkdir(root, 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(root, "pending.json"), []byte("replacement"), 0o600); err != nil {
			return err
		}
		swapped = true
		return nil
	})
	if err == nil {
		t.Fatal("remove after root replacement succeeded; want a fail-closed refusal")
	}
	if !swapped {
		t.Fatal("workspace root was not replaced after the original root handle opened")
	}
	if checks < 1 {
		t.Fatalf("removal authority checks = %d, want initial authority check", checks)
	}
	if got, err := os.ReadFile(filepath.Join(root, "pending.json")); err != nil || string(got) != "replacement" {
		t.Fatalf("replacement journal = %q, err=%v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(parked, "pending.json")); err != nil || string(got) != "original" {
		t.Fatalf("original journal changed after refusal = %q, err=%v", got, err)
	}
}

func TestWriteRootIdentityGuardRejectsSwapDuringRootOpen(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	replacement := filepath.Join(parent, "replacement")
	parkedOriginal := filepath.Join(parent, "parked-original")
	for _, dir := range []string{root, replacement} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "note.txt"), []byte("agent"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	expected, err := workspace.DirectoryIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	replacementInstalled := false
	err = workspace.WriteAtomicIfUnchangedWithHooks(root, "note.txt", []byte("agent"), []byte("restored"), workspace.WriteHooks{
		Check: func() error {
			if replacementInstalled {
				return nil
			}
			if err := os.Rename(root, parkedOriginal); err != nil {
				return err
			}
			if err := os.Rename(replacement, root); err != nil {
				return err
			}
			replacementInstalled = true
			return nil
		},
		CheckRootIdentity: func(actual workspace.Identity) error {
			if actual != expected {
				return workspace.ErrRootIdentityChanged
			}
			return nil
		},
	})
	if !errors.Is(err, workspace.ErrRootIdentityChanged) {
		t.Fatalf("write after transient root swap = %v, want root-identity rejection", err)
	}
	if !replacementInstalled {
		t.Fatal("replacement was not installed before the workspace root opened")
	}
	if got, err := os.ReadFile(filepath.Join(root, "note.txt")); err != nil || string(got) != "agent" {
		t.Fatalf("replacement workspace file = %q, %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(parkedOriginal, "note.txt")); err != nil || string(got) != "agent" {
		t.Fatalf("original workspace file = %q, %v", got, err)
	}
}

func TestRemoveRootIdentityGuardRejectsReplacement(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	replacement := filepath.Join(parent, "replacement")
	parkedOriginal := filepath.Join(parent, "parked-original")
	for _, dir := range []string{root, replacement} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "pending.json"), []byte("pending"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	expected, err := workspace.DirectoryIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(root, parkedOriginal); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, root); err != nil {
		t.Fatal(err)
	}
	err = workspace.RemoveWithChecks(root, "pending.json", nil, func(actual workspace.Identity) error {
		if actual != expected {
			return workspace.ErrRootIdentityChanged
		}
		return nil
	})
	if !errors.Is(err, workspace.ErrRootIdentityChanged) {
		t.Fatalf("remove after root replacement = %v, want root-identity rejection", err)
	}
	for _, path := range []string{filepath.Join(root, "pending.json"), filepath.Join(parkedOriginal, "pending.json")} {
		if got, err := os.ReadFile(path); err != nil || string(got) != "pending" {
			t.Fatalf("pending file %s = %q, %v", path, got, err)
		}
	}
}
