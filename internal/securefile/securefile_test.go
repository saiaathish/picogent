package securefile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

func TestReadFileMissingPreservesOSNotExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	_, err := ReadFile(path)
	if err == nil {
		t.Fatal("ReadFile succeeded for a missing file")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("ReadFile missing error=%v, want os.IsNotExist", err)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ReadFile missing error=%v, want errors.Is(os.ErrNotExist)", err)
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
		defer close(done)
		<-start
		for i := 1; i <= rounds; i++ {
			if err := WriteAtomic(path, []byte(fmt.Sprintf("version-%d:%s", i, strings.Repeat("x", 2048))), 0o600); err != nil {
				errs <- err
				return
			}
		}
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
				// Unix uses an ordinary path reader deliberately. Windows path
				// readers commonly deny delete sharing, which makes a concurrent
				// atomic rename legitimately fail while their handle is open; use
				// securefile's delete-sharing reader there.
				read := os.ReadFile
				if runtime.GOOS == "windows" {
					read = ReadFile
				}
				data, err := read(path)
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

func TestOpenLockFileCreatesMissingParent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "trace.lock")
	file, err := OpenLockFile(path)
	if err != nil {
		t.Fatal(err)
	}
	unlock, err := LockFile(file, true)
	if err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := unlock(); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if !info.Mode().IsRegular() {
		t.Fatalf("lock path is not a regular file: %s", info.Mode())
	}
}
