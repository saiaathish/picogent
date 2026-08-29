package securefile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestReadFileLimitedReturnsSentinel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	data := []byte("0123456789")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := ReadFileLimited(path, len(data)-1); !errors.Is(err, ErrReadLimit) {
		t.Fatalf("ReadFileLimited oversized record=%v, want ErrReadLimit", err)
	}
}

func TestWriteAtomicRejectsSymlinkTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "outside.yaml")
	if err := os.WriteFile(target, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "config.yaml")
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := WriteAtomic(path, []byte("replace\n"), 0o600); err == nil {
		t.Fatal("atomic writer accepted a symlink target")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep\n" {
		t.Fatalf("symlink target changed to %q", got)
	}
}

func TestWriteAtomicRejectsSymlinkParent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	path := filepath.Join(link, "config.yaml")
	if err := WriteAtomic(path, []byte("must not escape\n"), 0o600); err == nil {
		t.Fatal("atomic writer accepted a symlink parent")
	}
	if _, err := os.Stat(filepath.Join(outside, "config.yaml")); !os.IsNotExist(err) {
		t.Fatalf("symlink parent received a write: %v", err)
	}
}

func TestRemoveFileRejectsSymlinkTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "outside.yaml")
	if err := os.WriteFile(target, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "config.yaml")
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := RemoveFile(path); err == nil {
		t.Fatal("remove accepted a symlink target")
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("symlink was removed after rejection: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep\n" {
		t.Fatalf("symlink target changed to %q", got)
	}
}

func TestWriteAtomicReadersNeverObservePartialData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yaml")
	if err := WriteAtomic(path, []byte("version-0:"+strings.Repeat("x", 2048)), 0o600); err != nil {
		t.Fatal(err)
	}

	const rounds = 160
	known := make(map[string]bool, rounds+1)
	for i := 0; i <= rounds; i++ {
		known[fmt.Sprintf("version-%d:%s", i, strings.Repeat("x", 2048))] = true
	}
	start := make(chan struct{})
	done := make(chan struct{})
	errs := make(chan error, 4)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 1; i <= rounds; i++ {
			if err := WriteAtomic(path, []byte(fmt.Sprintf("version-%d:%s", i, strings.Repeat("x", 2048))), 0o600); err != nil {
				errs <- err
				return
			}
		}
		close(done)
	}()
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for {
				select {
				case <-done:
					return
				default:
				}
				// Use an ordinary path reader deliberately. ReadFile coordinates
				// through securefile's lock and would not detect a live-destination
				// truncate/copy publication bug.
				data, err := os.ReadFile(path)
				if err != nil {
					errs <- err
					return
				}
				if !known[string(data)] {
					errs <- fmt.Errorf("reader observed partial/unknown state of %d bytes", len(data))
					return
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
