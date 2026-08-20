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
