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
