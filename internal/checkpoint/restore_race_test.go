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

func TestRestoreRejectsTransientReplacementDuringPreflight(t *testing.T) {
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
	cp.restoreReadHook = func(stage, _ string, after bool) {
		if stage != "preflight" || !after {
			return
		}
		cp.restoreReadHook = nil
		if err := os.Rename(root, transient); err != nil {
			t.Fatalf("park transient replacement: %v", err)
		}
		if err := os.Rename(parked, root); err != nil {
			t.Fatalf("restore original workspace: %v", err)
		}
	}

	result, err := cp.Restore()
	if !errors.Is(err, ErrWorkspaceChanged) || result.Complete || len(result.Failures) == 0 {
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

func TestNoOpConfirmationRejectsTransientReplacementWorkspace(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	replacement := filepath.Join(parent, "replacement")
	parkedOriginal := filepath.Join(parent, "parked-original")
	parkedReplacement := filepath.Join(parent, "parked-replacement")
	for _, dir := range []string{root, replacement} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(replacement, "note.txt"), []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	cp, err := Capture(root, []string{"note.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := cp.Seal(); err != nil {
		t.Fatal(err)
	}
	cp.restoreReadHook = func(stage, rel string, after bool) {
		if stage != "noop-confirmation" {
			return
		}
		if !after {
			if err := os.WriteFile(path, []byte("agent"), 0o644); err != nil {
				t.Fatalf("write newer original workspace state: %v", err)
			}
			if err := os.Rename(root, parkedOriginal); err != nil {
				t.Fatalf("park original workspace: %v", err)
			}
			if err := os.Rename(replacement, root); err != nil {
				t.Fatalf("install transient replacement: %v", err)
			}
			return
		}
		if err := os.Rename(root, parkedReplacement); err != nil {
			t.Fatalf("park transient replacement: %v", err)
		}
		if err := os.Rename(parkedOriginal, root); err != nil {
			t.Fatalf("restore original workspace: %v", err)
		}
	}

	result, err := cp.Restore()
	if !errors.Is(err, ErrWorkspaceChanged) || result.Complete {
		t.Fatalf("transient confirmation restore = result:%+v err:%v", result, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "agent" {
		t.Fatalf("original workspace state after rejected no-op = %q, %v", got, err)
	}

	cp.restoreReadHook = nil
	result, err = cp.Restore()
	if !errors.Is(err, ErrConflict) || result.Complete || len(result.Conflicts) != 1 {
		t.Fatalf("retry after transient confirmation = result:%+v err:%v", result, err)
	}
}

func TestNoOpConfirmationRejectsMissingWorkspaceRootOrParent(t *testing.T) {
	for _, test := range []struct {
		name        string
		missingRoot bool
	}{
		{name: "root", missingRoot: true},
		{name: "parent"},
	} {
		t.Run(test.name, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "workspace")
			nested := filepath.Join(root, "nested")
			if err := os.MkdirAll(nested, 0o755); err != nil {
				t.Fatal(err)
			}
			rel := filepath.Join("nested", "note.txt")
			cp, err := Capture(root, []string{rel})
			if err != nil {
				t.Fatal(err)
			}
			if err := cp.Seal(); err != nil {
				t.Fatal(err)
			}
			parkedRoot := filepath.Join(parent, "parked-workspace")
			parkedParent := filepath.Join(root, "parked-nested")
			cp.restoreReadHook = func(stage, _ string, after bool) {
				if stage != "noop-confirmation" {
					return
				}
				if !after {
					if test.missingRoot {
						if err := os.Rename(root, parkedRoot); err != nil {
							t.Fatalf("temporarily remove workspace root: %v", err)
						}
					} else if err := os.Rename(nested, parkedParent); err != nil {
						t.Fatalf("temporarily remove workspace parent: %v", err)
					}
					return
				}
				if test.missingRoot {
					if err := os.Rename(parkedRoot, root); err != nil {
						t.Fatalf("restore workspace root: %v", err)
					}
				} else if err := os.Rename(parkedParent, nested); err != nil {
					t.Fatalf("restore workspace parent: %v", err)
				}
			}

			result, err := cp.Restore()
			if !errors.Is(err, ErrWorkspaceChanged) || result.Complete {
				t.Fatalf("restore during missing %s = result:%+v err:%v", test.name, result, err)
			}

			cp.restoreReadHook = nil
			result, err = cp.Restore()
			if err != nil || !result.Complete {
				t.Fatalf("retry after restoring %s = result:%+v err:%v", test.name, result, err)
			}
		})
	}
}

func TestConflictConfirmationRejectsTransientReplacementWorkspace(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	replacement := filepath.Join(parent, "replacement")
	parkedOriginal := filepath.Join(parent, "parked-original")
	parkedReplacement := filepath.Join(parent, "parked-replacement")
	for _, dir := range []string{root, replacement} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(replacement, "note.txt"), []byte("before"), 0o644); err != nil {
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
	cp.restoreBeforeApply = func(string) {
		cp.restoreBeforeApply = nil
		if err := os.WriteFile(path, []byte("newer user edit"), 0o644); err != nil {
			t.Fatalf("write newer user edit: %v", err)
		}
	}
	cp.restoreReadHook = func(stage, rel string, after bool) {
		if stage != "conflict-confirmation" {
			return
		}
		if !after {
			if err := os.Rename(root, parkedOriginal); err != nil {
				t.Fatalf("park original workspace: %v", err)
			}
			if err := os.Rename(replacement, root); err != nil {
				t.Fatalf("install transient replacement: %v", err)
			}
			return
		}
		if err := os.Rename(root, parkedReplacement); err != nil {
			t.Fatalf("park transient replacement: %v", err)
		}
		if err := os.Rename(parkedOriginal, root); err != nil {
			t.Fatalf("restore original workspace: %v", err)
		}
	}

	result, err := cp.Restore()
	if !errors.Is(err, ErrWorkspaceChanged) || result.Complete {
		t.Fatalf("transient conflict confirmation = result:%+v err:%v", result, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "newer user edit" {
		t.Fatalf("original user edit after rejected confirmation = %q, %v", got, err)
	}

	cp.restoreReadHook = nil
	if err := os.WriteFile(path, []byte("agent"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err = cp.Restore()
	if err != nil || !result.Complete || len(result.Restored) != 1 {
		t.Fatalf("retry after transient conflict confirmation = result:%+v err:%v", result, err)
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
