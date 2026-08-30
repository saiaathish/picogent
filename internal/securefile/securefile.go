// Package securefile provides small, fail-closed helpers for application-owned
// files. Paths are validated before use and writes are published as complete
// filesystem entries. Readers and removals coordinate with file locks; the
// atomic writer is intended to keep ordinary path readers from observing a
// partially serialized document.
package securefile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

var tempSequence atomic.Uint64

// ErrLocked reports that a non-blocking lock attempt found another owner.
// Callers can use errors.Is to distinguish contention from an invalid or
// unavailable file descriptor.
var ErrLocked = errors.New("secure file is locked")

// ErrReadLimit reports that a bounded read encountered more than its allowed
// number of bytes. Callers can use errors.Is to map this condition without
// depending on diagnostic text.
var ErrReadLimit = errors.New("secure file read limit exceeded")

// EnsureDir creates path and its missing parents while rejecting
// application-created symlink components. It is useful for a separate lock
// file that must be opened before the first document write.
func EnsureDir(path string, mode os.FileMode) error {
	if strings.TrimSpace(path) == "" || strings.ContainsRune(path, 0) {
		return errors.New("directory path is empty or contains NUL")
	}
	return ensureSecureDir(path, mode)
}

// OpenLockFile opens a regular lock file below its validated parent. The
// caller owns the returned descriptor and is responsible for platform locking
// and closing it.
func OpenLockFile(path string) (*os.File, error) {
	root, name, err := openParent(path, true)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if info, statErr := root.stat(name); statErr == nil {
		if info.kind == secureEntrySymlink {
			return nil, fmt.Errorf("lock file %q is a symbolic link", path)
		}
		if info.kind != secureEntryRegular {
			return nil, fmt.Errorf("lock file %q is not a regular file", path)
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return nil, statErr
	}
	return root.openLock(name)
}

// TryLockFile takes a non-blocking lock on file. The returned function releases
// the lock; the caller still owns and must close file. This is intentionally a
// small public seam for process-wide session ownership, while ordinary readers
// and writers continue to use the blocking internal lock helper.
func TryLockFile(file *os.File, exclusive bool) (func() error, error) {
	if file == nil {
		return nil, errors.New("secure lock file is nil")
	}
	return tryLockSecureFile(file, exclusive)
}

// LockFile takes a blocking lock on file. The returned function releases the
// lock; the caller still owns and must close file. This is the blocking
// counterpart to TryLockFile for application-owned serialization points.
func LockFile(file *os.File, exclusive bool) (func() error, error) {
	if file == nil {
		return nil, errors.New("secure lock file is nil")
	}
	return lockSecureFile(file, exclusive)
}

// ReadFile reads a regular, non-symlink file below its validated parent.
// Missing files retain the standard os.ErrNotExist behavior.
func ReadFile(path string) ([]byte, error) {
	return readFile(path, 0)
}

// ReadFileLimited is ReadFile with a hard byte limit. It is intended for
// bounded application state whose decoder should never receive an oversized
// document.
func ReadFileLimited(path string, maxBytes int) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, errors.New("secure file read limit must be positive")
	}
	return readFile(path, maxBytes)
}

func readFile(path string, maxBytes int) ([]byte, error) {
	root, name, err := openParent(path, false)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.stat(name)
	if err != nil {
		return nil, err
	}
	if info.kind != secureEntryRegular {
		return nil, fmt.Errorf("file %q is not a regular file", path)
	}
	file, err := root.openRead(name)
	if err != nil {
		return nil, err
	}
	unlock, err := lockSecureFile(file, false)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	var reader io.Reader = file
	if maxBytes > 0 {
		reader = io.LimitReader(file, int64(maxBytes)+1)
	}
	data, readErr := io.ReadAll(reader)
	if readErr == nil && maxBytes > 0 && len(data) > maxBytes {
		data = nil
		readErr = fmt.Errorf("%w: file %q exceeds the %d-byte read limit", ErrReadLimit, path, maxBytes)
	}
	unlockErr := unlock()
	closeErr := file.Close()
	return data, errors.Join(readErr, unlockErr, closeErr)
}

// RemoveFile removes a regular, non-symlink file below its validated parent.
// Missing files retain the standard os.ErrNotExist behavior. The operation is
// deliberately narrower than os.Remove so a corrupted or attacker-controlled
// path cannot turn cleanup into symlink traversal or directory removal.
func RemoveFile(path string) error {
	root, name, err := openParent(path, false)
	if err != nil {
		return err
	}
	defer root.Close()
	info, err := root.stat(name)
	if err != nil {
		return err
	}
	if info.kind == secureEntrySymlink {
		return fmt.Errorf("file %q is a symbolic link", path)
	}
	if info.kind != secureEntryRegular {
		return fmt.Errorf("file %q is not a regular file", path)
	}
	file, err := root.openRead(name)
	if err != nil {
		return err
	}
	unlock, err := lockSecureFile(file, true)
	if err != nil {
		_ = file.Close()
		return err
	}
	removeErr := root.removeMatching(name, file)
	unlockErr := unlock()
	closeErr := file.Close()
	if err := errors.Join(removeErr, unlockErr, closeErr); err != nil {
		return err
	}
	// Directory fsync is best-effort because it is not supported uniformly by
	// all platforms; the removal itself has already completed successfully.
	_ = root.sync()
	return nil
}

// WriteAtomic writes data to path using mode for a new file. Existing regular
// files retain their permissions, matching os.WriteFile's update behavior.
// Symlinked targets and symlinked parent components are rejected rather than
// followed or silently replaced. Publication replaces the destination entry
// in one filesystem operation, so ordinary path readers see either the old
// complete file or the new complete file. The temporary pathname is never
// reopened as the source of truth. On Unix and the unsupported-target fallback,
// the final rename still consumes a pathname: POSIX has no portable
// compare-and-rename-by-inode primitive, so an uncooperative same-UID writer
// can replace that name after the identity check. The staging inode is also
// writable while it is open, so success does not prove that the published
// bytes still equal data under same-UID tampering. This is not a hostile
// same-UID directory-tamper guarantee.
func WriteAtomic(path string, data []byte, mode os.FileMode) error {
	root, name, err := openParent(path, true)
	if err != nil {
		return err
	}
	defer root.Close()

	if info, statErr := root.stat(name); statErr == nil {
		if info.kind == secureEntrySymlink {
			return fmt.Errorf("file %q is a symbolic link", path)
		}
		if info.kind != secureEntryRegular {
			return fmt.Errorf("file %q is not a regular file", path)
		}
		mode = info.mode.Perm()
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	if mode.Perm() == 0 {
		mode = 0o600
	}

	tmpName := ""
	var file *os.File
	for attempt := uint64(0); attempt < 32; attempt++ {
		seq := tempSequence.Add(1)
		tmpName = fmt.Sprintf(".picogent-%d-%d-%d.tmp", os.Getpid(), time.Now().UnixNano(), seq)
		file, err = root.openExclusive(tmpName, mode.Perm())
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrExist) {
			return err
		}
	}
	if file == nil {
		return errors.New("could not allocate an atomic temporary file")
	}
	removeTemp := true
	defer func() {
		if removeTemp {
			// A hostile process may have replaced the temporary pathname after
			// it was opened. Never remove a different inode during cleanup.
			_ = root.removeMatching(tmpName, file)
			_ = file.Close()
		}
	}()

	if err := file.Chmod(mode.Perm()); err != nil {
		return err
	}
	if err := writeAll(file, data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	// Keep the descriptor open through publication. Platform implementations
	// copy from this trusted descriptor into a securely opened destination (or
	// create that destination exclusively); they never reopen the temporary
	// pathname as the source of truth.
	if err := root.replace(tmpName, name, file); err != nil {
		return err
	}
	removeTemp = false
	if err := file.Close(); err != nil {
		return err
	}
	// Directory fsync is best-effort because it is not supported uniformly by
	// all platforms; the destination has already been synced before publish.
	_ = root.sync()
	return nil
}

func writeAll(file *os.File, data []byte) error {
	for len(data) > 0 {
		n, err := file.Write(data)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

type secureEntryKind uint8

const (
	secureEntryRegular secureEntryKind = iota + 1
	secureEntryDirectory
	secureEntrySymlink
	secureEntryOther
)

type secureEntry struct {
	kind secureEntryKind
	mode os.FileMode
}

// secureParent is a descriptor/handle-anchored parent directory. Platform
// implementations must never re-open the directory through its original
// absolute path after this value is returned.
type secureParent interface {
	Close() error
	stat(name string) (secureEntry, error)
	openRead(name string) (*os.File, error)
	openLock(name string) (*os.File, error)
	openExclusive(name string, mode os.FileMode) (*os.File, error)
	remove(name string) error
	removeMatching(name string, source *os.File) error
	replace(oldName, newName string, source *os.File) error
	sync() error
}

func openParent(path string, createParent bool) (secureParent, string, error) {
	if strings.TrimSpace(path) == "" || strings.ContainsRune(path, 0) {
		return nil, "", errors.New("file path is empty or contains NUL")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, "", err
	}
	parent := filepath.Dir(abs)
	name := filepath.Base(abs)
	if name == "" || name == "." || name == ".." {
		return nil, "", fmt.Errorf("invalid file name %q", name)
	}
	root, err := openSecureParent(parent, createParent)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Preserve the os.IsNotExist behavior used by callers that treat a
			// missing parent like a missing file. The descriptor implementation
			// may add useful context around the underlying syscall, but the
			// legacy os helper only unwraps PathError/SyscallError values.
			return nil, "", &os.PathError{Op: "open", Path: parent, Err: os.ErrNotExist}
		}
		return nil, "", err
	}
	return root, name, nil
}
