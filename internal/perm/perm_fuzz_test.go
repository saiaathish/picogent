package perm

import (
	"path/filepath"
	"strings"
	"testing"
)

func FuzzResolveWorkspacePathBoundary(f *testing.F) {
	for _, seed := range []string{".", "a.txt", "../outside", "a/../../outside", "", string([]byte{0, '/', 'x'})} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, requested string) {
		workspace := t.TempDir()
		got, err := ResolveWorkspacePath(workspace, requested)
		if err != nil {
			return
		}
		rel, err := filepath.Rel(got.Root, got.Path)
		if err != nil {
			t.Fatalf("relative path: %v", err)
		}
		outside := rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
		if got.OutsideWorkspace != outside {
			t.Fatalf("resolved path=%q root=%q outside=%v, computed outside=%v", got.Path, got.Root, got.OutsideWorkspace, outside)
		}
	})
}
