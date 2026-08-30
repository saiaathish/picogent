//go:build unix

package securefile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestReplaceRejectsReplacedTemporaryEntry(t *testing.T) {
	parent := t.TempDir()
	outside := t.TempDir()
	root, err := openSecureParent(parent, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	p := root.(*unixParent)

	const tempName = ".picogent-test.tmp"
	source, err := p.openExclusive(tempName, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if _, err := source.Write([]byte("trusted\n")); err != nil {
		t.Fatal(err)
	}
	if err := source.Sync(); err != nil {
		t.Fatal(err)
	}

	if err := os.Rename(filepath.Join(parent, tempName), filepath.Join(parent, tempName+".saved")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(parent, tempName)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	err = p.replace(tempName, "state.yaml", source)
	if err == nil || !strings.Contains(err.Error(), "changed before commit") {
		t.Fatalf("replace accepted a swapped temporary name: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(parent, "state.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("swapped temporary entry was committed: %v", err)
	}
	if info, err := os.Lstat(filepath.Join(parent, tempName)); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("hostile temporary replacement was not preserved for cleanup: info=%v err=%v", info, err)
	}
	if _, err := os.Stat(filepath.Join(outside, "state.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement escaped into outside directory: %v", err)
	}
}

func TestRemoveMatchingLeavesReplacedTemporaryEntry(t *testing.T) {
	parent := t.TempDir()
	root, err := openSecureParent(parent, false)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	p := root.(*unixParent)

	const tempName = ".picogent-cleanup.tmp"
	source, err := p.openExclusive(tempName, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if _, err := source.Write([]byte("trusted\n")); err != nil {
		t.Fatal(err)
	}
	if err := source.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(parent, tempName), filepath.Join(parent, tempName+".saved")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, tempName), []byte("attacker\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := p.removeMatching(tempName, source); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(parent, tempName))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "attacker\n" {
		t.Fatalf("replaced temporary entry changed during cleanup: %q", got)
	}
}

func TestWriteAtomicParentSwapNeverEscapesDescriptor(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	parent := filepath.Join(root, "state")
	backup := filepath.Join(root, "state-real")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	outsideFile := filepath.Join(outside, "state.yaml")
	if err := os.WriteFile(outsideFile, []byte("outside\n"), 0o600); err != nil {
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
			// The swap is intentionally hostile: a path-based OpenRoot can
			// follow the temporary link, while descriptor-relative traversal
			// either retains the original directory or fails closed.
			if err := os.Rename(parent, backup); err != nil {
				continue
			}
			_ = os.Symlink(outside, parent)
			_ = os.Remove(parent)
			_ = os.Rename(backup, parent)
		}
	}()

	for i := 0; i < 400; i++ {
		_ = WriteAtomic(filepath.Join(parent, "state.yaml"), []byte("inside\n"), 0o600)
	}
	close(stop)
	swaps.Wait()
	if info, err := os.Lstat(parent); err == nil && info.Mode()&os.ModeSymlink != 0 {
		_ = os.Remove(parent)
	}
	if _, err := os.Lstat(parent); errors.Is(err, os.ErrNotExist) {
		_ = os.Rename(backup, parent)
	}
	if info, err := os.Lstat(parent); err != nil || !info.IsDir() {
		t.Fatalf("parent was not restored after swap campaign: info=%v err=%v", info, err)
	}
	got, err := os.ReadFile(outsideFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "outside\n" {
		t.Fatalf("parent swap redirected write outside descriptor: %q", got)
	}
}
