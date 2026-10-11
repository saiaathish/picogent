package agent

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/saiaathish/picogent/internal/securefile"
	"github.com/saiaathish/picogent/internal/workspace"
)

func TestReadUndoWorkspaceFileRefusesReplacedLeafAfterLock(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "marker")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	identity, err := workspace.DirectoryIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	data, err := readUndoWorkspaceFileWithHooks(root, path, identity, maxUndoWorkspaceMarkerBytes, func() {
		if err := os.Rename(path, filepath.Join(root, "old-marker")); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("new\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}, nil)
	if !errors.Is(err, securefile.ErrReadChanged) || data != nil {
		t.Fatalf("stale locked read = %q, %v", data, err)
	}
}

func TestReadUndoWorkspaceFileRefusesReplacementDuringRead(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "marker")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	identity, err := workspace.DirectoryIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	data, err := readUndoWorkspaceFileWithHooks(root, path, identity, maxUndoWorkspaceMarkerBytes, nil, func() {
		if err := os.Rename(path, filepath.Join(root, "old-marker")); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("new\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	})
	if !errors.Is(err, securefile.ErrReadChanged) || data != nil {
		t.Fatalf("read concurrent replacement = %q, %v", data, err)
	}
}

func TestEnsureUndoWorkspaceInstanceRefusesRemovedMarkerAfterNameCheck(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows byte-range locks prevent removing the open marker fixture")
	}
	root := t.TempDir()
	if _, err := ensureUndoWorkspaceInstance(root); err != nil {
		t.Fatal(err)
	}
	_, marker, err := undoWorkspaceMarkerPath(root)
	if err != nil {
		t.Fatal(err)
	}
	instance, err := ensureUndoWorkspaceInstanceWithHooks(root, undoWorkspaceInitHooks{
		AfterMarkerReadNameCheck: func() {
			if err := os.Remove(marker); err != nil {
				t.Fatal(err)
			}
		},
	})
	if err == nil || instance.valid() || !errors.Is(err, securefile.ErrReadChanged) {
		t.Fatalf("initialization accepted a removed marker: instance=%+v err=%v", instance, err)
	}
	if isMissingUndoWorkspaceFile(err) {
		t.Fatalf("marker replacement race was classified as ordinary absence: %v", err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("initialization recreated marker after replacement race: %v", err)
	}
}

func TestEnsureUndoWorkspaceInstanceDoesNotMutateReplacementRoot(t *testing.T) {
	for _, point := range []string{"after-identity", "after-lock"} {
		t.Run(point, func(t *testing.T) {
			if runtime.GOOS == "windows" && point == "after-lock" {
				t.Skip("Windows byte-range lock prevents renaming its containing workspace; the separate after-identity test covers replacement before lock creation")
			}
			parent := t.TempDir()
			root := filepath.Join(parent, "workspace")
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatal(err)
			}
			swap := func() {
				if err := os.Rename(root, filepath.Join(parent, "original")); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(root, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			hooks := undoWorkspaceInitHooks{}
			if point == "after-identity" {
				hooks.AfterIdentity = swap
			} else {
				hooks.AfterLock = swap
			}
			if instance, err := ensureUndoWorkspaceInstanceWithHooks(root, hooks); err == nil || instance.valid() {
				t.Fatalf("replacement root initialized: instance=%+v err=%v", instance, err)
			}
			if _, err := os.Stat(filepath.Join(root, ".picogent")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("refused initialization mutated replacement root: %v", err)
			}
		})
	}
}

func TestEnsureUndoWorkspaceInstanceRefusesReplacedLockFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows byte-range locking prevents replacing the exclusively locked marker lock file")
	}
	root := t.TempDir()
	instance, err := ensureUndoWorkspaceInstanceWithHooks(root, undoWorkspaceInitHooks{
		AfterLock: func() {
			lockPath := filepath.Join(root, filepath.FromSlash(undoWorkspaceInstanceMarker)+".lock")
			if err := os.Rename(lockPath, lockPath+".old"); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(lockPath, []byte("replacement lock\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
	})
	if err == nil || instance.valid() {
		t.Fatalf("initialization accepted a replaced lock: instance=%+v err=%v", instance, err)
	}
	marker := filepath.Join(root, filepath.FromSlash(undoWorkspaceInstanceMarker))
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("initialization created marker after lock replacement: %v", err)
	}
}

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
