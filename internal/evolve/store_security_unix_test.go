//go:build unix

package evolve

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveRejectsSymlinkedStateParent(t *testing.T) {
	root := t.TempDir()
	realHome := filepath.Join(root, "real-home")
	if err := os.Mkdir(realHome, 0o700); err != nil {
		t.Fatal(err)
	}
	linkedHome := filepath.Join(root, "linked-home")
	if err := os.Symlink(realHome, linkedHome); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PICOGENT_HOME", linkedHome)

	if err := Save(Store{Workspace: filepath.Join(root, "project")}); err == nil {
		t.Fatal("Save followed a symlinked state parent")
	}
}

func TestLoadRejectsSymlinkedStateFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PICOGENT_HOME", root)
	workspace := filepath.Join(root, "project")
	if err := Save(Store{Workspace: workspace}); err != nil {
		t.Fatal(err)
	}
	path, err := readPath(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside-state.json")
	if err := os.WriteFile(outside, []byte(`{"workspace":"outside"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(workspace); err == nil {
		t.Fatal("Load followed a symlinked state file")
	}
}

func TestUpdateRejectsSymlinkedLockFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PICOGENT_HOME", root)
	workspace := filepath.Join(root, "project")
	if err := Save(Store{Workspace: workspace}); err != nil {
		t.Fatal(err)
	}
	path, err := readPath(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path + ".lock"); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside-lock")
	if err := os.WriteFile(outside, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path+".lock"); err != nil {
		t.Fatal(err)
	}

	if _, err := Update(workspace, func(store Store) (Store, error) {
		return store, nil
	}); err == nil {
		t.Fatal("Update followed a symlinked lock file")
	}
}
