package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoMapToolIsRegisteredAndBounded(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(Context{Workspace: root})
	tool, ok := registry.Get("repo_map")
	if !ok {
		t.Fatal("repo_map tool not registered")
	}
	out, err := tool.Run(context.Background(), "{}", registry.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"languages"`) || !strings.Contains(out, `"Go"`) || len(out) > 12<<10 {
		t.Fatalf("unexpected repo map (%d bytes): %s", len(out), out)
	}
}
