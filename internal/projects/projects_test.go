package projects_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/saiaathish/picogent/internal/projects"
)

func TestAddAndSwitch(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	os.Setenv("PICOGENT_HOME", home)
	t.Cleanup(func() { os.Unsetenv("PICOGENT_HOME") })

	ws := t.TempDir()
	p, err := projects.Add("demo", ws)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "demo" {
		t.Fatalf("name=%q", p.Name)
	}
	list, cur, err := projects.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || cur != p.ID {
		t.Fatalf("list=%v cur=%q", list, cur)
	}
	p2, err := projects.Switch(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if p2.Path != ws {
		t.Fatalf("path=%q", p2.Path)
	}
}

func TestCreateMakesOnlyOneNewFolderAndSelectsIt(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	parent := t.TempDir()
	p, err := projects.Create(parent, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if p.Path != filepath.Join(parent, "demo") {
		t.Fatalf("path=%q", p.Path)
	}
	if st, err := os.Stat(p.Path); err != nil || !st.IsDir() {
		t.Fatalf("created folder: %v", err)
	}
	list, current, err := projects.List()
	if err != nil || len(list) != 1 || current != p.ID {
		t.Fatalf("list=%v current=%q err=%v", list, current, err)
	}
	if _, err := projects.Create(parent, "demo"); err == nil {
		t.Fatal("existing folder was accepted")
	}
	for _, name := range []string{"", ".", "..", "../escape", "a/b", `a\b`} {
		if _, err := projects.Create(parent, name); err == nil {
			t.Fatalf("unsafe name %q was accepted", name)
		}
	}
	if _, err := os.Stat(filepath.Join(parent, "escape")); !os.IsNotExist(err) {
		t.Fatalf("escaped parent: %v", err)
	}
}
