package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestProjectHealthOnlyReadsManifestMetadata(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "must-not-be-created")
	manifest, err := json.Marshal(map[string]any{
		"scripts": map[string]string{
			"test": "touch " + marker,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(Context{Workspace: root})
	tool, ok := registry.Get("project_health")
	if !ok {
		t.Fatal("project_health tool not registered")
	}
	if _, err := tool.Run(context.Background(), "{}", registry.Ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("manifest script was executed or marker lookup failed: %v", err)
	}
}
