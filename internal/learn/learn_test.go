package learn_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/saiaathish/picogent/internal/learn"
)

func TestLoadDoesNotCreateStateUntilSave(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "missing", "picogent")
	t.Setenv("PICOGENT_HOME", home)
	store, err := learn.Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(home); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load created state directory: %v", err)
	}
	if err := learn.Save(store); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "learn")); err != nil {
		t.Fatalf("Save did not create state directory: %v", err)
	}
}

func TestStoreKnowledge(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	os.Setenv("PICOGENT_HOME", home)
	t.Cleanup(func() { os.Unsetenv("PICOGENT_HOME") })

	ws := t.TempDir()
	s, err := learn.Load(ws)
	if err != nil {
		t.Fatal(err)
	}
	s.RecordRead("main.go")
	s.RecordRead("main.go")
	s.RecordChange("main.go", 5, 2)
	s.RecordTurn()
	if err := learn.Save(s); err != nil {
		t.Fatal(err)
	}
	s2, err := learn.Load(ws)
	if err != nil {
		t.Fatal(err)
	}
	if s2.Knowledge == 0 {
		t.Fatal("expected knowledge > 0")
	}
	if len(s2.Overview) == 0 {
		t.Fatal("expected overview bullets")
	}
}
