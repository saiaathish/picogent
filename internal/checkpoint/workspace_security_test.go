package checkpoint

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCheckpointWorkspaceReadDoesNotEscapeAncestorSwap(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix descriptor-relative stress test")
	}
	workspaceRoot := t.TempDir()
	outside := t.TempDir()
	parent := filepath.Join(workspaceRoot, "parent")
	realParent := filepath.Join(workspaceRoot, "parent-real")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "marker.txt"), []byte("workspace"), 0o600); err != nil {
		t.Fatal(err)
	}
	outsideTarget := filepath.Join(outside, "marker.txt")
	if err := os.WriteFile(outsideTarget, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := readWorkspaceFile(canonicalRoot, "parent/marker.txt")
	if err != nil || !initial.exists || string(initial.data) != "workspace" {
		t.Fatalf("baseline checkpoint read failed: %q, %v", initial.data, err)
	}

	stop := make(chan struct{})
	var swaps sync.WaitGroup
	var swapCount atomic.Int32
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
				_ = os.Remove(parent)
			}
			_ = os.Rename(realParent, parent)
			time.Sleep(100 * time.Microsecond)
		}
	}()

	successfulReads := 0
	for i := 0; i < 250; i++ {
		state, err := readWorkspaceFile(canonicalRoot, "parent/marker.txt")
		if err != nil {
			continue
		}
		successfulReads++
		if state.exists && string(state.data) != "workspace" {
			t.Fatalf("checkpoint read escaped workspace: %q", state.data)
		}
	}
	close(stop)
	swaps.Wait()

	if got, err := os.ReadFile(outsideTarget); err != nil || string(got) != "private" {
		t.Fatalf("outside file changed: %q, %v", got, err)
	}
	if swapCount.Load() == 0 {
		t.Fatal("ancestor swap never completed")
	}
	if successfulReads == 0 {
		t.Fatal("all checkpoint reads failed; stress test did not exercise a successful read")
	}
}
