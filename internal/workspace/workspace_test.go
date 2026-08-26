package workspace

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRelativeRejectsWorkspaceEscape(t *testing.T) {
	root := t.TempDir()
	if _, err := Relative(root, filepath.Join(root, "..", "outside.txt")); err == nil {
		t.Fatal("Relative accepted a path outside the workspace")
	}
	if _, err := Relative(root, "."); err == nil {
		t.Fatal("Relative accepted the workspace directory")
	}
}

func TestUnixOpenRejectsSymlinkedWorkspaceAncestor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix descriptor traversal test")
	}
	base := t.TempDir()
	realRoot := filepath.Join(base, "real", "workspace")
	if err := os.MkdirAll(realRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realRoot, "note.txt"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "real"), filepath.Join(base, "alias")); err != nil {
		t.Fatal(err)
	}

	if f, err := OpenRead(filepath.Join(base, "alias", "workspace"), "note.txt"); err == nil {
		_ = f.Close()
		t.Fatal("OpenRead followed a symlinked workspace ancestor")
	}
}

func TestRemoveDoesNotFollowOutsideSymlink(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "owned.txt"), []byte("owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Remove(root, "owned.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "owned.txt")); !os.IsNotExist(err) {
		t.Fatalf("removed file still exists: %v", err)
	}

	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(root, "escape")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("symlink creation requires privileges on Windows")
		}
		t.Fatal(err)
	}
	if err := Remove(root, "escape"); err == nil && runtime.GOOS == "windows" {
		t.Fatal("Windows Remove unexpectedly followed a reparse point")
	}
	if got, err := os.ReadFile(secret); err != nil || string(got) != "private" {
		t.Fatalf("outside file changed: %q, %v", got, err)
	}
}
