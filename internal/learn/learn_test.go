package learn_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/learn"
	"github.com/saiaathish/picogent/internal/projects"
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
	if err := learn.Save(&store); err != nil {
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
	if err := learn.Save(&s); err != nil {
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

func TestRecordTestInitializesNilToolCounts(t *testing.T) {
	s := learn.Store{}
	s.RecordTest(1, 0, 0, "ok")
	if s.ToolCounts == nil || s.ToolCounts["test"] != 1 {
		t.Fatalf("RecordTest tool counts=%v, want test=1", s.ToolCounts)
	}
}

func TestRecordTestRedactsAndBoundsOutputBeforePersistence(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	t.Setenv("PICOGENT_HOME", home)
	ws := filepath.Join(root, "project")
	const secret = "test-output-secret"

	s := learn.Store{Workspace: ws}
	s.RecordTest(2, 0, 0, `go test API_KEY="`+secret+`"`+strings.Repeat("x", 6000))
	if s.LastTest == nil {
		t.Fatal("RecordTest did not record a snapshot")
	}
	if strings.Contains(s.LastTest.Output, secret) {
		t.Fatalf("credential-shaped output remained in snapshot: %q", s.LastTest.Output)
	}
	if !strings.Contains(s.LastTest.Output, "[REDACTED]") {
		t.Fatalf("expected redaction marker in snapshot: %q", s.LastTest.Output)
	}
	if len(s.LastTest.Output) > 4003 {
		t.Fatalf("test output exceeded bound: %d", len(s.LastTest.Output))
	}

	if err := learn.Save(&s); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "learn", projects.IDForPath(ws)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secret) {
		t.Fatalf("credential-shaped output was persisted: %s", data)
	}
	loaded, err := learn.Load(ws)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LastTest == nil || strings.Contains(loaded.LastTest.Output, secret) {
		t.Fatalf("loaded test output still contains secret: %+v", loaded.LastTest)
	}
}

func TestSaveRejectsSymlinkedStateTarget(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	t.Setenv("PICOGENT_HOME", home)
	workspace := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(home, "learn"), 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(home, "learn", projects.IDForPath(workspace)+".json")
	if err := os.Symlink(outside, statePath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := learn.Save(&learn.Store{Workspace: workspace}); err == nil {
		t.Fatal("learn.Save accepted a symlinked state target")
	}
	got, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep\n" {
		t.Fatalf("symlink target changed to %q", got)
	}
}

func TestSaveRejectsStaleSnapshot(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	t.Setenv("PICOGENT_HOME", home)
	workspace := filepath.Join(root, "project")

	first := learn.Store{Workspace: workspace}
	first.RecordTurn()
	if err := learn.Save(&first); err != nil {
		t.Fatal(err)
	}
	stale, err := learn.Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := learn.Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	fresh.RecordSearch()
	if err := learn.Save(&fresh); err != nil {
		t.Fatal(err)
	}

	stale.RecordRead("stale.go")
	err = learn.Save(&stale)
	if !errors.Is(err, learn.ErrRevisionConflict) {
		t.Fatalf("stale save error = %v, want revision conflict", err)
	}
	loaded, err := learn.Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Searches != 1 {
		t.Fatalf("fresh snapshot was overwritten: searches=%d", loaded.Searches)
	}
	if _, ok := loaded.FilesRead["stale.go"]; ok {
		t.Fatal("stale snapshot was published")
	}
}

func TestStoreBoundsLongExploration(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	t.Setenv("PICOGENT_HOME", home)
	ws := t.TempDir()
	s, err := learn.Load(ws)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 400; i++ {
		path := fmt.Sprintf("src/generated/file-%03d.go", i)
		s.RecordRead(path)
		s.RecordChange(path, i+1, i)
		s.RecordTool(fmt.Sprintf("tool-%03d", i))
	}
	s.RecordRead(strings.Repeat("x", 2000))
	if err := learn.Save(&s); err != nil {
		t.Fatal(err)
	}
	loaded, err := learn.Load(ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.FilesRead) > 256 {
		t.Fatalf("files read grew beyond bound: %d", len(loaded.FilesRead))
	}
	if len(loaded.FilesChanged) > 256 {
		t.Fatalf("files changed grew beyond bound: %d", len(loaded.FilesChanged))
	}
	if len(loaded.ToolCounts) > 64 {
		t.Fatalf("tool counts grew beyond bound: %d", len(loaded.ToolCounts))
	}
	for path := range loaded.FilesRead {
		if len(path) > 600 {
			t.Fatalf("path key was not bounded: %d bytes", len(path))
		}
	}
}
