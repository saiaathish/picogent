package learn_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/saiaathish/picogent/internal/learn"
)

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
