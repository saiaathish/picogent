package checkpoint_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/checkpoint"
)

func TestRestoreRejectsReplacementSymlinkBeforeMutation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	workspace := t.TempDir()
	outside := t.TempDir()
	writeFile(t, workspace, "note.txt", "before")
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}

	cp, err := checkpoint.Capture(workspace, []string{"note.txt"})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, workspace, "note.txt", "after")
	if err := cp.Seal(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(workspace, "note.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(workspace, "note.txt")); err != nil {
		t.Fatal(err)
	}

	result, err := cp.Restore()
	if err == nil || result.Complete || len(result.Failures) != 1 {
		t.Fatalf("restore through replacement symlink = result:%+v err:%v", result, err)
	}
	if !errors.Is(err, checkpoint.ErrConflict) && result.Failures[0].Operation != "inspect" {
		t.Fatalf("unexpected replacement-symlink error: result:%+v err:%v", result, err)
	}
	if got, readErr := os.ReadFile(secret); readErr != nil || string(got) != "private" {
		t.Fatalf("outside file changed: %q, %v", got, readErr)
	}
}

func TestRestoreRejectsReplacementHardlinkBeforeMutation(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	writeFile(t, workspace, "note.txt", "before")
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}

	cp, err := checkpoint.Capture(workspace, []string{"note.txt"})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, workspace, "note.txt", "after")
	if err := cp.Seal(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(workspace, "note.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(secret, filepath.Join(workspace, "note.txt")); err != nil {
		t.Skipf("hard links unavailable in test environment: %v", err)
	}

	result, err := cp.Restore()
	if err == nil || result.Complete || len(result.Failures) != 1 {
		t.Fatalf("restore through replacement hardlink = result:%+v err:%v", result, err)
	}
	if result.Failures[0].Operation != "inspect" {
		t.Fatalf("unexpected replacement-hardlink error: result:%+v err:%v", result, err)
	}
	if got, readErr := os.ReadFile(secret); readErr != nil || string(got) != "private" {
		t.Fatalf("outside file changed: %q, %v", got, readErr)
	}
}

func TestRestoreRejectsReplacementHardlinkBeforeDeletingCreatedPath(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	parent := filepath.Join(workspace, "parent")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("parent", "created.txt")
	created := filepath.Join(workspace, path)

	cp, err := checkpoint.Capture(workspace, []string{path})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, workspace, path, "created")
	if err := cp.Seal(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(created); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(secret, created); err != nil {
		t.Skipf("hard links unavailable in test environment: %v", err)
	}

	result, err := cp.Restore()
	if err == nil || result.Complete || len(result.Failures) != 1 {
		t.Fatalf("restore through created-path hardlink = result:%+v err:%v", result, err)
	}
	if result.Failures[0].Operation != "inspect" {
		t.Fatalf("unexpected created-path hardlink error: result:%+v err:%v", result, err)
	}
	if got, readErr := os.ReadFile(secret); readErr != nil || string(got) != "private" {
		t.Fatalf("outside file changed: %q, %v", got, readErr)
	}
	if got, readErr := os.ReadFile(created); readErr != nil || string(got) != "private" {
		t.Fatalf("replacement hardlink was removed or changed: %q, %v", got, readErr)
	}
}

func TestRestoreDoesNotEscapeAncestorSwap(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix descriptor-relative stress test")
	}
	workspace := t.TempDir()
	outside := t.TempDir()
	parent := filepath.Join(workspace, "parent")
	realParent := filepath.Join(workspace, "parent-real")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	paths := make([]string, 16)
	for i := range paths {
		paths[i] = filepath.Join("parent", "file-"+string(rune('a'+i))+".txt")
		writeFile(t, workspace, paths[i], "before")
	}
	outsideFiles := make([]string, len(paths))
	for i, rel := range paths {
		name := filepath.Base(rel)
		outsideFiles[i] = filepath.Join(outside, name)
		if err := os.WriteFile(outsideFiles[i], []byte("private"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	cp, err := checkpoint.Capture(workspace, paths)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range paths {
		writeFile(t, workspace, rel, "after")
	}
	if err := cp.Seal(); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	firstSwap := make(chan struct{})
	var once sync.Once
	var swapCount atomic.Int32
	var swaps sync.WaitGroup
	swaps.Add(1)
	go func() {
		defer swaps.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if err := os.Rename(parent, realParent); err != nil {
				continue
			}
			if err := os.Symlink(outside, parent); err == nil {
				swapCount.Add(1)
				once.Do(func() { close(firstSwap) })
				_ = os.Remove(parent)
			}
			_ = os.Rename(realParent, parent)
			time.Sleep(100 * time.Microsecond)
		}
	}()
	select {
	case <-firstSwap:
	case <-time.After(time.Second):
		close(stop)
		swaps.Wait()
		t.Fatal("ancestor swap never completed")
	}

	_, _ = cp.Restore()
	close(stop)
	swaps.Wait()
	if swapCount.Load() == 0 {
		t.Fatal("ancestor swap was not observed")
	}
	for _, path := range outsideFiles {
		if got, readErr := os.ReadFile(path); readErr != nil || string(got) != "private" {
			t.Fatalf("outside file changed: %q, %v", got, readErr)
		}
	}
}

func TestRestoreDoesNotDeleteOutsideDuringAncestorSwap(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix descriptor-relative deletion stress test")
	}
	workspace := t.TempDir()
	outside := t.TempDir()
	parent := filepath.Join(workspace, "parent")
	realParent := filepath.Join(workspace, "parent-real")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	paths := make([]string, 16)
	for i := range paths {
		paths[i] = filepath.Join("parent", "created-"+string(rune('a'+i))+".txt")
	}
	outsideFiles := make([]string, len(paths))
	for i, rel := range paths {
		name := filepath.Base(rel)
		outsideFiles[i] = filepath.Join(outside, name)
		if err := os.WriteFile(outsideFiles[i], []byte("private"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	cp, err := checkpoint.Capture(workspace, paths)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range paths {
		writeFile(t, workspace, rel, "created")
	}
	if err := cp.Seal(); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
	firstSwap := make(chan struct{})
	var once sync.Once
	var swapCount atomic.Int32
	var swaps sync.WaitGroup
	swaps.Add(1)
	go func() {
		defer swaps.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if err := os.Rename(parent, realParent); err != nil {
				continue
			}
			if err := os.Symlink(outside, parent); err == nil {
				swapCount.Add(1)
				once.Do(func() { close(firstSwap) })
				_ = os.Remove(parent)
			}
			_ = os.Rename(realParent, parent)
			time.Sleep(100 * time.Microsecond)
		}
	}()
	select {
	case <-firstSwap:
	case <-time.After(time.Second):
		close(stop)
		swaps.Wait()
		t.Fatal("ancestor swap never completed")
	}

	_, _ = cp.Restore()
	close(stop)
	swaps.Wait()
	if swapCount.Load() == 0 {
		t.Fatal("ancestor swap was not observed")
	}
	for _, path := range outsideFiles {
		if got, readErr := os.ReadFile(path); readErr != nil || string(got) != "private" {
			t.Fatalf("outside file changed: %q, %v", got, readErr)
		}
	}
}

func writeFile(t *testing.T, root, rel, contents string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
