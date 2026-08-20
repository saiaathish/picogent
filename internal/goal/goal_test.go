package goal

import (
	"path/filepath"
	"testing"
)

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
