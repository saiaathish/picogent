//go:build unix

package runtimeboundary

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestRetainReportParentSwapNeverEscapesDescriptor(t *testing.T) {
	workspace := t.TempDir()
	root := t.TempDir()
	outside := t.TempDir()
	parent := filepath.Join(root, "artifacts")
	backup := filepath.Join(root, "artifacts-real")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "sentinel.txt"), []byte("outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sha := strings.Repeat("a", 40)
	report := sampleReport(sha)
	if err := RetainReport(workspace, filepath.Join(parent, "baseline.json"), report); err != nil {
		t.Fatal(err)
	}

	stop := make(chan struct{})
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
			if err := os.Rename(parent, backup); err != nil {
				continue
			}
			if err := os.Symlink(outside, parent); err != nil {
				_ = os.Rename(backup, parent)
				continue
			}
			_ = os.Remove(parent)
			_ = os.Rename(backup, parent)
		}
	}()

	for i := 0; i < 400; i++ {
		// A failure during the hostile interval is acceptable; a successful
		// operation must never create an entry in the outside directory.
		_ = RetainReport(workspace, filepath.Join(parent, fmt.Sprintf("matrix-%03d.json", i)), report)
	}
	close(stop)
	swaps.Wait()
	_ = os.RemoveAll(parent)
	_ = os.Rename(backup, parent)

	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "sentinel.txt" {
		t.Fatalf("hostile parent swap created outside entries: %+v", entries)
	}
}
