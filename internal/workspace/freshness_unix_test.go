//go:build darwin || linux || freebsd || netbsd || openbsd || dragonfly

package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestCaptureFIFOIsRejectedWithoutBlocking(t *testing.T) {
	root := t.TempDir()
	pipe := filepath.Join(root, "pipe")
	if err := unix.Mkfifo(pipe, 0o600); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	_, err := Capture(context.Background(), root, []string{"pipe"})
	if err == nil {
		t.Fatal("FIFO was accepted as a regular file")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("FIFO capture blocked for %s: %v", elapsed, err)
	}
	if _, statErr := os.Stat(pipe); statErr != nil {
		t.Fatal(statErr)
	}
}
