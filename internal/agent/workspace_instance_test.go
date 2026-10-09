package agent

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestEnsureUndoWorkspaceInstanceConcurrentInitialization(t *testing.T) {
	root := t.TempDir()
	const workers = 8
	type outcome struct {
		instance undoWorkspaceInstance
		err      error
	}
	results := make(chan outcome, workers)
	var group sync.WaitGroup
	for i := 0; i < workers; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			instance, err := ensureUndoWorkspaceInstance(root)
			results <- outcome{instance: instance, err: err}
		}()
	}
	group.Wait()
	close(results)

	var first undoWorkspaceInstance
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if !result.instance.valid() {
			t.Fatalf("invalid initialized workspace instance: %+v", result.instance)
		}
		if !first.valid() {
			first = result.instance
		} else if result.instance != first {
			t.Fatalf("concurrent initializers disagreed: first=%+v next=%+v", first, result.instance)
		}
	}
}

func TestUndoWorkspaceInstanceFailsClosedOnMarkerCorruption(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, filepath.FromSlash(undoWorkspaceInstanceMarker))
	if err := os.MkdirAll(filepath.Dir(marker), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(marker, []byte("not-a-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readUndoWorkspaceInstance(root); err == nil {
		t.Fatal("malformed marker was accepted")
	}
	if _, err := ensureUndoWorkspaceInstance(root); err == nil {
		t.Fatal("malformed marker was overwritten during initialization")
	}
}

func TestUndoWorkspaceInstanceMarkerChangeInvalidatesBinding(t *testing.T) {
	root := t.TempDir()
	instance := testUndoWorkspaceInstance(t, root)
	token := strings.Repeat("a", undoWorkspaceInstanceBytes*2)
	if token == instance.Token {
		token = strings.Repeat("b", undoWorkspaceInstanceBytes*2)
	}
	marker := filepath.Join(root, filepath.FromSlash(undoWorkspaceInstanceMarker))
	if err := os.WriteFile(marker, []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateUndoWorkspaceInstance(root, instance); err == nil {
		t.Fatal("changed workspace marker retained the previous instance binding")
	}
}
