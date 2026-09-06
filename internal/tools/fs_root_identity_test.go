package tools_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/tools"
)

func TestReadWriteEditRejectReplacedWorkspaceRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "seed.txt"), []byte("seed\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	reg := tools.NewRegistry(tools.Context{Workspace: root})
	base := reg.ContextSnapshot()
	for _, name := range []string{"read_file", "write_file", "edit_file"} {
		tool, ok := reg.Get(name)
		if !ok {
			t.Fatalf("missing %s", name)
		}
		var args string
		switch name {
		case "read_file":
			args = `{"path":"seed.txt"}`
		case "write_file":
			args = `{"path":"out.txt","content":"new\n"}`
		case "edit_file":
			args = `{"path":"seed.txt","old_string":"seed\n","new_string":"edited\n"}`
		}
		req := tool.Permission(args, base)
		identity := req.WorkspaceIdentity()
		if identity == nil {
			t.Fatalf("%s missing identity", name)
		}

		old := filepath.Join(parent, "old-"+name)
		if err := os.Rename(root, old); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "attacker.txt"), []byte("attacker\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		_, err := tool.Run(t.Context(), args, tools.Context{
			Workspace:         root,
			WorkspaceIdentity: identity,
		})
		if !errors.Is(err, perm.ErrWorkspaceChanged) {
			t.Fatalf("%s after replacement = %v, want ErrWorkspaceChanged", name, err)
		}
		if _, err := os.Stat(filepath.Join(root, "out.txt")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s wrote into replacement tree", name)
		}
		got, err := os.ReadFile(filepath.Join(root, "attacker.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "attacker\n" {
			t.Fatalf("%s mutated attacker sentinel: %q", name, got)
		}

		// Restore for the next tool under a fresh identity capture.
		_ = os.RemoveAll(root)
		if err := os.Rename(old, root); err != nil {
			t.Fatal(err)
		}
	}
}
