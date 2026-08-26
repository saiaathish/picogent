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
	if !strings.Contains(out, `"languages"`) || !strings.Contains(out, `"Go"`) || !strings.Contains(out, `"provenance"`) || len(out) > 12<<10 {
		t.Fatalf("unexpected repo map (%d bytes): %s", len(out), out)
	}
}

func TestProjectHealthToolIsRegisteredAndBounded(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(Context{Workspace: root})
	tool, ok := registry.Get("project_health")
	if !ok {
		t.Fatal("project_health tool not registered")
	}
	out, err := tool.Run(context.Background(), "{}", registry.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"schema": "picogent.project-health.v1"`) ||
		!strings.Contains(out, `"state": "UNVERIFIED"`) || len(out) > 12<<10 {
		t.Fatalf("unexpected project health report (%d bytes): %s", len(out), out)
	}
}
