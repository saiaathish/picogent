package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRelativeRejectsWorkspaceEscape(t *testing.T) {
	root := t.TempDir()
	if _, err := Relative(root, filepath.Join(root, "..", "outside.txt")); err == nil {
		t.Fatal("Relative accepted a path outside the workspace")
	}
	if _, err := Relative(root, "."); err == nil {
		t.Fatal("Relative accepted the workspace directory")
	}
}

func TestUnixOpenRejectsSymlinkedWorkspaceAncestor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix descriptor traversal test")
	}
	base := t.TempDir()
	realRoot := filepath.Join(base, "real", "workspace")
	if err := os.MkdirAll(realRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(realRoot, "note.txt"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "real"), filepath.Join(base, "alias")); err != nil {
		t.Fatal(err)
	}

	if f, err := OpenRead(filepath.Join(base, "alias", "workspace"), "note.txt"); err == nil {
		_ = f.Close()
		t.Fatal("OpenRead followed a symlinked workspace ancestor")
	}
}

func TestReadDirListsDescriptorAnchoredDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("note"), 0o600); err != nil {
		t.Fatal(err)
	}

	entries, err := ReadDir(root, "nested/../.")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		seen[entry.Name()] = entry.IsDir()
	}
	if !seen["nested"] {
		t.Fatalf("unexpected descriptor-anchored entries: %#v", seen)
	}
	if isDir, ok := seen["note.txt"]; !ok || isDir {
		t.Fatalf("note.txt missing or reported as a directory: %#v", seen)
	}
}

func TestReadDirRejectsSymlinkedDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}

	if _, err := ReadDir(root, "linked"); err == nil {
		t.Fatal("ReadDir followed a symlinked directory")
	}
}

func TestWriteAtomicPublishesCompleteFileAndPreservesMode(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "nested", "state.txt")
	if err := WriteAtomic(root, "nested/state.txt", []byte("first\n")); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "first\n" {
		t.Fatalf("initial atomic write = %q, %v", got, err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := WriteAtomic(root, "nested/state.txt", []byte("second\n")); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "second\n" {
		t.Fatalf("replacement atomic write = %q, %v", got, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("replacement changed mode to %o", got)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".picogent-workspace-") {
			t.Fatalf("atomic temporary file leaked: %s", entry.Name())
		}
	}
}

func TestWriteAtomicWithModePublishesRequestedMode(t *testing.T) {
	root := t.TempDir()
	if err := WriteAtomicWithMode(root, "state.txt", []byte("first\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(root, "state.txt")); err != nil || string(got) != "first\n" {
		t.Fatalf("initial atomic write = %q, %v", got, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(root, "state.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("initial mode=%o", got)
		}
	}
	if err := WriteAtomicWithMode(root, "state.txt", []byte("second\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(root, "state.txt")); err != nil || string(got) != "second\n" {
		t.Fatalf("replacement atomic write = %q, %v", got, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(root, "state.txt"))
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o644 {
			t.Fatalf("replacement mode=%o", got)
		}
	}
}

func TestWriteAtomicIfUnchangedRefusesStaleContent(t *testing.T) {
	root := t.TempDir()
	if err := WriteAtomic(root, "state.txt", []byte("before\n")); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomicIfUnchanged(root, "state.txt", []byte("before\n"), []byte("after\n")); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomicIfUnchanged(root, "state.txt", []byte("before\n"), []byte("stale\n")); !errors.Is(err, ErrContentConflict) {
		t.Fatalf("stale edit error = %v, want ErrContentConflict", err)
	}
	if got, err := os.ReadFile(filepath.Join(root, "state.txt")); err != nil || string(got) != "after\n" {
		t.Fatalf("stale edit changed file to %q, %v", got, err)
	}
	if err := WriteAtomicIfUnchanged(root, "missing.txt", nil, []byte("must not create\n")); !errors.Is(err, ErrContentConflict) {
		t.Fatalf("missing edit error = %v, want ErrContentConflict", err)
	}
	if _, err := os.Stat(filepath.Join(root, "missing.txt")); !os.IsNotExist(err) {
		t.Fatalf("missing edit created a file: %v", err)
	}
}

func TestWriteAtomicIfUnchangedWithModeRefusesStaleMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows exposes only a writable/read-only mode projection")
	}
	root := t.TempDir()
	path := filepath.Join(root, "state.txt")
	if err := WriteAtomicWithMode(root, "state.txt", []byte("before\n"), 0o640); err != nil {
		t.Fatal(err)
	}

	err := WriteAtomicIfUnchangedWithMode(root, "state.txt", []byte("before\n"), 0o644, []byte("after\n"), 0o644)
	if !errors.Is(err, ErrContentConflict) {
		t.Fatalf("stale mode edit error = %v, want ErrContentConflict", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "before\n" {
		t.Fatalf("stale mode edit changed file to %q, %v", got, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("stale mode edit changed mode to %o", got)
	}
}

func TestWriteAtomicRejectsReadOnlyTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows read-only attributes have different semantics")
	}
	root := t.TempDir()
	path := filepath.Join(root, "state.txt")
	if err := WriteAtomic(root, "state.txt", []byte("before\n")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(root, "state.txt", []byte("after\n")); err == nil {
		t.Fatal("WriteAtomic replaced a read-only target")
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "before\n" {
		t.Fatalf("read-only target changed to %q, %v", got, err)
	}
}

func TestWriteAtomicRejectsSymlinkEntries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(target, []byte("private\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "linked.txt")); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(root, "linked.txt", []byte("must not replace\n")); err == nil {
		t.Fatal("WriteAtomic accepted a symlink target")
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "private\n" {
		t.Fatalf("symlink target changed to %q, %v", got, err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked-dir")); err != nil {
		t.Fatal(err)
	}
	if err := WriteAtomic(root, "linked-dir/state.txt", []byte("must not escape\n")); err == nil {
		t.Fatal("WriteAtomic accepted a symlink parent")
	}
	if _, err := os.Stat(filepath.Join(outside, "state.txt")); !os.IsNotExist(err) {
		t.Fatalf("symlink parent received a write: %v", err)
	}
}

func TestWorkspaceRejectsHardLinkedFiles(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	linked := filepath.Join(root, "linked.txt")
	if err := os.WriteFile(secret, []byte("private\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(secret, linked); err != nil {
		t.Skipf("hard links unavailable in test environment: %v", err)
	}

	for name, open := range map[string]func() (*os.File, error){
		"read":  func() (*os.File, error) { return OpenRead(root, "linked.txt") },
		"write": func() (*os.File, error) { return OpenWrite(root, "linked.txt") },
		"edit":  func() (*os.File, error) { return OpenEdit(root, "linked.txt") },
	} {
		file, err := open()
		if err == nil {
			_ = file.Close()
			t.Fatalf("%s opened a hard-linked workspace file", name)
		}
	}
	if err := WriteAtomic(root, "linked.txt", []byte("must not replace\n")); err == nil {
		t.Fatal("WriteAtomic accepted a hard-linked target")
	}
	if err := Remove(root, "linked.txt"); err == nil {
		t.Fatal("Remove accepted a hard-linked target")
	}
	if got, err := os.ReadFile(secret); err != nil || string(got) != "private\n" {
		t.Fatalf("hard-linked outside file changed: %q, %v", got, err)
	}
}

func TestRemoveDoesNotFollowOutsideSymlink(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "owned.txt"), []byte("owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Remove(root, "owned.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "owned.txt")); !os.IsNotExist(err) {
		t.Fatalf("removed file still exists: %v", err)
	}

	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(root, "escape")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("symlink creation requires privileges on Windows")
		}
		t.Fatal(err)
	}
	if err := Remove(root, "escape"); err == nil {
		t.Fatal("Remove accepted a symlink entry")
	}
	if info, err := os.Lstat(filepath.Join(root, "escape")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink entry was removed after rejection: info=%v err=%v", info, err)
	}
	if got, err := os.ReadFile(secret); err != nil || string(got) != "private" {
		t.Fatalf("outside file changed: %q, %v", got, err)
	}
}
