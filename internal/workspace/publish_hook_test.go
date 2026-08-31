package workspace_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/saiaathish/picogent/internal/workspace"
)

func TestWriteAtomicPublishHookRunsBeforeRename(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "state.txt")
	if err := os.WriteFile(path, []byte("before"), 0o640); err != nil {
		t.Fatal(err)
	}
	called := false
	if err := workspace.WriteAtomicWithPublishHook(root, path, []byte("after"), func(mode os.FileMode) error {
		called = true
		if mode.Perm() != 0o640 {
			t.Fatalf("hook mode=%o, want 640", mode.Perm())
		}
		got, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if string(got) != "before" {
			t.Fatalf("hook observed published content %q", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("publish hook was not called")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "after" {
		t.Fatalf("published file = %q, err=%v", got, err)
	}
}

func TestWriteAtomicPublishHookCanAbortPublication(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "state.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	hookErr := os.ErrPermission
	if err := workspace.WriteAtomicWithPublishHook(root, path, []byte("after"), func(os.FileMode) error {
		return hookErr
	}); err == nil {
		t.Fatal("aborted publication unexpectedly succeeded")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "before" {
		t.Fatalf("aborted publication changed file = %q, err=%v", got, err)
	}
}
