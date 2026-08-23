package goal

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDoesNotCreateStateUntilSet(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "missing", "picogent")
	t.Setenv("PICOGENT_HOME", home)
	ws := t.TempDir()
	got, err := Load(ws)
	if err != nil || got != "" {
		t.Fatalf("Load: %q err=%v", got, err)
	}
	if _, err := os.Stat(home); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load created state directory: %v", err)
	}
	if err := Set(ws, "finish the release"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "goals")); err != nil {
		t.Fatalf("Set did not create state directory: %v", err)
	}
}

func TestSetLoadClear(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PICOGENT_HOME", root)
	ws := filepath.Join(root, "proj")

	if err := Set(ws, "fix flaky tests"); err != nil {
		t.Fatal(err)
	}
	got, err := Load(ws)
	if err != nil || got != "fix flaky tests" {
		t.Fatalf("load: %q err=%v", got, err)
	}
	if err := Clear(ws); err != nil {
		t.Fatal(err)
	}
	got, _ = Load(ws)
	if got != "" {
		t.Fatalf("expected empty after clear, got %q", got)
	}
}

func TestLooksComplete(t *testing.T) {
	if !LooksComplete("Goal complete: tests green") {
		t.Fatal("prefix")
	}
	if LooksComplete("still working") {
		t.Fatal("false positive")
	}
}
