package projects_test

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
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

func TestConcurrentAddPreservesRegistryUpdates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)

	const count = 12
	workspaces := make([]string, count)
	for i := range workspaces {
		workspaces[i] = t.TempDir()
	}
	start := make(chan struct{})
	errs := make(chan error, count)
	var wg sync.WaitGroup
	for i, workspace := range workspaces {
		wg.Add(1)
		go func(i int, workspace string) {
			defer wg.Done()
			<-start
			if _, err := projects.Add("project-"+string(rune('a'+i)), workspace); err != nil {
				errs <- err
			}
		}(i, workspace)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	list, _, err := projects.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != count {
		t.Fatalf("concurrent registry entries = %d, want %d: %#v", len(list), count, list)
	}
}

func TestSaveRejectsSymlinkedRegistryTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)
	target := filepath.Join(t.TempDir(), "outside.yaml")
	if err := os.WriteFile(target, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(home, "projects.yaml")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := projects.Save(projects.Registry{Current: "x"}); err == nil {
		t.Fatal("projects.Save accepted a symlinked target")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep\n" {
		t.Fatalf("symlink target changed to %q", got)
	}
}

func TestSaveIfCurrentRejectsStaleRollback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)

	first := projects.Registry{Current: "first"}
	second := projects.Registry{Current: "second"}
	if err := projects.Save(first); err != nil {
		t.Fatal(err)
	}
	if err := projects.SaveIfCurrent(first, second); err != nil {
		t.Fatal(err)
	}
	if err := projects.SaveIfCurrent(first, projects.Registry{Current: "rollback"}); !errors.Is(err, projects.ErrRegistryChanged) {
		t.Fatalf("stale rollback error = %v, want ErrRegistryChanged", err)
	}
	got, err := projects.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Current != second.Current {
		t.Fatalf("stale rollback overwrote newer registry: got %#v", got)
	}
}
