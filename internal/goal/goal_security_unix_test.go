//go:build unix

package goal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadStateRejectsSymlinkedStateFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PICOGENT_HOME", root)
	workspace := filepath.Join(root, "project")
	if err := Set(workspace, "keep this goal"); err != nil {
		t.Fatal(err)
	}
	path, err := readPath(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside-goal")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadState(workspace); err == nil {
		t.Fatal("LoadState followed a symlinked goal state file")
	}
}

func TestLoadStateRejectsSymlinkedBackupFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PICOGENT_HOME", root)
	workspace := filepath.Join(root, "project")
	path, err := storePath(workspace)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside-goal-backup")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, stateBackupPath(path)); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadState(workspace); err == nil {
		t.Fatal("LoadState followed a symlinked goal backup file")
	}
}
