package perm_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/tools"
)

func TestClassifyPathBindsWorkspaceRootIdentity(t *testing.T) {
	root := t.TempDir()
	req := perm.ClassifyPath("write_file", "probe.txt", root, "write probe.txt")
	if req.WorkspaceIdentity() == nil {
		t.Fatal("expected workspace identity binding")
	}
	if err := req.ValidateWorkspaceIdentity(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateWorkspaceIdentityRejectsOrdinaryRootReplacement(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	req := perm.ClassifyPath("write_file", "probe.txt", root, "write probe.txt")
	if req.WorkspaceIdentity() == nil {
		t.Fatal("expected workspace identity binding")
	}

	old := filepath.Join(parent, "old-workspace")
	if err := os.Rename(root, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "attacker.txt"), []byte("pwned\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := req.ValidateWorkspaceIdentity()
	if !errors.Is(err, perm.ErrWorkspaceChanged) {
		t.Fatalf("ValidateWorkspaceIdentity = %v, want ErrWorkspaceChanged", err)
	}

	registry := tools.NewRegistry(tools.Context{
		Workspace:         root,
		WorkspaceIdentity: req.WorkspaceIdentity(),
	})
	write, ok := registry.Get("write_file")
	if !ok {
		t.Fatal("missing write_file")
	}
	if _, runErr := write.Run(t.Context(), `{"path":"probe.txt","content":"should-not-land"}`, tools.Context{
		Workspace:         root,
		WorkspaceIdentity: req.WorkspaceIdentity(),
	}); !errors.Is(runErr, perm.ErrWorkspaceChanged) {
		t.Fatalf("write after replacement = %v, want ErrWorkspaceChanged", runErr)
	}
	if _, err := os.Stat(filepath.Join(root, "probe.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement tree was written: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(root, "attacker.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "pwned\n" {
		t.Fatalf("attacker sentinel changed: %q", got)
	}
}

func TestFileToolMissingIdentityFailsClosed(t *testing.T) {
	req := perm.Request{Tool: "write_file", Summary: "write"}
	if err := req.ValidateWorkspaceIdentity(); !errors.Is(err, perm.ErrWorkspaceChanged) {
		t.Fatalf("missing identity = %v, want ErrWorkspaceChanged", err)
	}
}
