// Package workspace provides descriptor-safe access to workspace-relative
// files and directories. Callers must still perform their own permission
// decision; this package protects the subsequent filesystem operation from
// symlink, hard-link, and reparse-point path substitution.
package workspace

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ErrContentConflict reports that a file changed after an edit operation read
// it. The caller must re-read the file and recompute the edit.
var (
	ErrContentConflict         = errors.New("workspace file changed during edit")
	ErrTargetChanged           = errors.New("workspace target identity changed")
	ErrRootIdentityChanged     = errors.New("workspace root identity changed")
	ErrWorkspaceParentNotExist = errors.New("workspace root or parent directory does not exist")
)

// RootIdentityCheck validates the identity obtained from the opened workspace
// root handle that anchors a filesystem operation.
type RootIdentityCheck func(Identity) error

// ReadHooks provides narrow observation points for a root-bound read.
// AfterParentOpen runs after descriptor-anchored parent traversal and before
// opening the final file name.
type ReadHooks struct {
	AfterParentOpen func()
}

func classifyWorkspaceParentError(err error) error {
	if err == nil || errors.Is(err, ErrWorkspaceParentNotExist) || !isWorkspaceNotExist(err) {
		return err
	}
	return fmt.Errorf("%w: %w", ErrWorkspaceParentNotExist, err)
}

func rejectHardLinkCount(count uint64) error {
	if count > 1 {
		return errors.New("workspace files with multiple hard links are not allowed")
	}
	return nil
}

// Relative returns a clean, non-empty path relative to root. path may be an
// absolute path or a path relative to root; it must not escape root.
func Relative(root, path string) (string, error) {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("workspace path is empty")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	target := path
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", fmt.Errorf("path is not in workspace: %w", err)
	}
	if rel == "." || rel == "" {
		return "", fmt.Errorf("workspace path names a directory")
	}
	if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path is outside workspace")
	}
	if _, err := pathParts(rel); err != nil {
		return "", err
	}
	return filepath.Clean(rel), nil
}

func pathParts(rel string) ([]string, error) {
	clean := filepath.Clean(rel)
	if clean == "." || clean == "" || filepath.IsAbs(clean) || filepath.VolumeName(clean) != "" {
		return nil, fmt.Errorf("path must be a non-empty workspace-relative file path")
	}
	parts := strings.Split(clean, string(filepath.Separator))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part {
		case "", ".":
			continue
		case "..":
			return nil, fmt.Errorf("path escapes workspace")
		default:
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("workspace path names a directory")
	}
	return out, nil
}

func directoryParts(root, path string) ([]string, error) {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("workspace directory path is empty")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	root = filepath.Clean(root)
	target := path
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return nil, fmt.Errorf("path is not in workspace: %w", err)
	}
	if rel == "." || rel == "" {
		return nil, nil
	}
	if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("path is outside workspace")
	}
	return pathParts(rel)
}

// OpenRead opens a regular file below root without following path-component
// symlinks, hard links, or reparse points.
func OpenRead(root, path string) (*os.File, error) {
	return open(root, path, openRead)
}

// OpenReadWithRootIdentity opens a regular file only when the directory handle
// used to reach it matches expectedRoot. This binds the read to the requested
// workspace even if its pathname is swapped while the operation is starting.
func OpenReadWithRootIdentity(root, path string, expectedRoot Identity) (*os.File, error) {
	return OpenReadWithRootIdentityAndHooks(root, path, expectedRoot, ReadHooks{})
}

// OpenReadWithRootIdentityAndHooks opens a regular file using the expected
// workspace root and confirms that the opened parent still resolves to the
// requested path before returning either a file or a missing-leaf result. The
// hook is intended for deterministic operation instrumentation; ordinary
// callers should use OpenReadWithRootIdentity.
func OpenReadWithRootIdentityAndHooks(root, path string, expectedRoot Identity, hooks ReadHooks) (*os.File, error) {
	if !expectedRoot.Known {
		return nil, errors.New("expected workspace root identity is unknown")
	}
	checkRootIdentity := func(actual Identity) error {
		if !actual.Known || actual != expectedRoot {
			return ErrRootIdentityChanged
		}
		return nil
	}
	return openWithRootIdentityCheckAndHooks(root, path, openRead, checkRootIdentity, hooks)
}

func openReadForOperation(root, path string, checkRootIdentity RootIdentityCheck) (*os.File, error) {
	if checkRootIdentity == nil {
		return OpenRead(root, path)
	}
	return openWithRootIdentityCheck(root, path, openRead, checkRootIdentity)
}

// OpenDir opens a directory below root without following path-component
// symlinks or reparse points. The workspace root itself may be named by path;
// callers do not need to special-case listing ".".
func OpenDir(root, path string) (*os.File, error) {
	return openDir(root, path)
}

// ReadDir reads entries from a directory below root using a descriptor or
// handle anchored to the workspace. Entry names and types are safe to inspect;
// callers should use workspace file helpers before opening an entry.
func ReadDir(root, path string) ([]os.DirEntry, error) {
	dir, err := OpenDir(root, path)
	if err != nil {
		return nil, err
	}
	entries, readErr := dir.ReadDir(-1)
	closeErr := dir.Close()
	return entries, errors.Join(readErr, closeErr)
}

// OpenWrite opens or creates a regular file below root without following
// path-component symlinks, hard links, or reparse points. It does not truncate
// the file; callers can validate cancellation before truncating the returned
// handle.
func OpenWrite(root, path string) (*os.File, error) {
	return open(root, path, openWrite)
}

// OpenEdit opens an existing regular file below root for read/write without
// following path-component symlinks, hard links, or reparse points.
func OpenEdit(root, path string) (*os.File, error) {
	return open(root, path, openEdit)
}

// WriteAtomic writes a complete file below root and publishes it as one
// filesystem replacement. Missing parent directories are created like
// OpenWrite. Existing regular files retain their permission bits; symlinked
// or non-regular targets are rejected, as are regular files with multiple hard
// links. Ordinary path readers therefore see
// either the previous complete file or the new complete file, not a live
// truncate-and-rewrite. Like all pathname-based filesystem publication, this
// does not make a directory writable by an uncooperative same-UID attacker
// safe from every name-replacement race.
func WriteAtomic(root, path string, data []byte) error {
	return writeAtomic(root, path, data)
}

// WriteAtomicWithPublishHook writes a complete file and invokes hook after
// the new inode is fully written and validated but immediately before it is
// published at path. A hook error aborts publication. This seam lets callers
// persist recovery metadata before the workspace rename without exposing a
// temporary pathname or weakening the normal workspace safety checks.
func WriteAtomicWithPublishHook(root, path string, data []byte, hook func(os.FileMode) error) error {
	return WriteAtomicWithHooks(root, path, data, WriteHooks{PreparePublish: hook})
}

// WriteHooks separates repeatable authority checks from recovery preparation.
// CheckRootIdentity validates the identity captured from the opened workspace
// root handle before any anchored mutation. Check runs before parent creation
// and staging and again after preparation, immediately before publication
// (including every Windows rename retry).
// CreateParentMode overrides the Unix mode for newly created parent
// directories; zero keeps the platform default. On Windows, the private
// 0700-parent/0600-file combination used by durable undo journals receives
// protected current-user-only ACLs; other writes retain inherited ACLs.
// A refusal stops future side effects; already-authorized parent creation is
// not rolled back. Like the underlying pathname primitives, this is not an
// atomic lock against an uncooperative writer changing authority after Check.
type WriteHooks struct {
	Check             func() error
	CheckRootIdentity RootIdentityCheck
	PreparePublish    func(os.FileMode) error
	CreateParentMode  os.FileMode
}

func (h WriteHooks) check() error {
	if h.Check != nil {
		return h.Check()
	}
	return nil
}

// WriteAtomicWithHooks is WriteAtomic with repeated caller-owned guards.
func WriteAtomicWithHooks(root, path string, data []byte, hooks WriteHooks) error {
	return writeAtomicWithHook(root, path, data, 0, false, hooks)
}

// WriteAtomicDurableWithModeAndHooks is WriteAtomicWithHooks with an explicit
// file mode and strict directory durability, including parent directories it
// creates. It fails before staging if a directory cannot be synchronously
// flushed, and reports a post-publication flush failure rather than claiming
// the new entry is durable.
func WriteAtomicDurableWithModeAndHooks(root, path string, data []byte, mode os.FileMode, hooks WriteHooks) error {
	return writeAtomicDurableWithHook(root, path, data, mode, true, hooks)
}

// WriteAtomicWithMode writes a complete file below root and publishes its
// contents and permission mode as one filesystem replacement. Missing parent
// directories are created like OpenWrite. Unlike WriteAtomic, the replacement
// receives the requested mode instead of inheriting the existing target mode.
// The same symlink, reparse-point, hard-link, and pathname-race limits as
// WriteAtomic apply.
func WriteAtomicWithMode(root, path string, data []byte, mode os.FileMode) error {
	return writeAtomicWithMode(root, path, data, mode, true)
}

// WriteAtomicIfUnchanged publishes data only when the current regular file
// content still equals expected. A mismatch leaves the current file untouched
// and returns ErrContentConflict. The content check and publication are a
// best-effort compare-then-publish boundary, not an atomic cross-process CAS:
// another writer can replace the file after the check, and the later pathname
// lookup can observe a replacement workspace root. This does not provide a
// hostile same-UID filesystem race barrier or cross-process lock.
func WriteAtomicIfUnchanged(root, path string, expected, data []byte) error {
	return WriteAtomicIfUnchangedWithPublishHook(root, path, expected, data, nil)
}

// WriteAtomicIfUnchangedWithMode is the compare-before-publish edit primitive
// with an expected mode and an explicit mode for the replacement. It performs
// the same freshness check as WriteAtomicIfUnchanged, then reuses the atomic
// publication path so callers that already performed a preflight check get a
// second content-and-mode check right before the replacement is prepared.
func WriteAtomicIfUnchangedWithMode(root, path string, expected []byte, expectedMode os.FileMode, data []byte, mode os.FileMode) error {
	return WriteAtomicIfUnchangedWithModeAndHooks(root, path, expected, expectedMode, data, mode, WriteHooks{})
}

// WriteAtomicIfUnchangedWithModeAndHooks is the mode-preserving compare-and-
// publish primitive with repeated authority checks around its descriptor-
// anchored publication. A check after opening the destination parent ensures
// pathname replacement before publication is rejected; replacement after
// that check remains anchored to the already-open parent directory.
func WriteAtomicIfUnchangedWithModeAndHooks(root, path string, expected []byte, expectedMode os.FileMode, data []byte, mode os.FileMode, hooks WriteHooks) error {
	return writeAtomicIfUnchangedWithModeHook(root, path, expected, expectedMode, true, data, mode, true, hooks)
}

// WriteAtomicIfMissingWithMode publishes data only while the target remains
// absent. It is the creation-side counterpart to
// WriteAtomicIfUnchangedWithMode and is used when undo restores a file that
// the turn deleted.
func WriteAtomicIfMissingWithMode(root, path string, data []byte, mode os.FileMode) error {
	return WriteAtomicIfMissingWithModeAndHooks(root, path, data, mode, WriteHooks{})
}

// WriteAtomicIfMissingWithModeAndHooks creates a file only while the target
// remains absent, with the same repeated authority checks as WriteAtomicWithHooks.
func WriteAtomicIfMissingWithModeAndHooks(root, path string, data []byte, mode os.FileMode, hooks WriteHooks) error {
	rel, err := Relative(root, path)
	if err != nil {
		return err
	}
	if err := hooks.check(); err != nil {
		return err
	}
	current, err := openReadForOperation(root, path, hooks.CheckRootIdentity)
	if err == nil {
		_ = current.Close()
		return fmt.Errorf("%w: %s already exists", ErrContentConflict, rel)
	}
	if errors.Is(err, ErrWorkspaceParentNotExist) {
		return err
	}
	if !isWorkspaceNotExist(err) {
		return err
	}
	return writeAtomicWithHook(root, path, data, mode, true, hooks)
}

// WriteAtomicIfUnchangedWithPublishHook is the compare-before-publish edit
// primitive with the same pre-publication recovery hook as
// WriteAtomicWithPublishHook.
func WriteAtomicIfUnchangedWithPublishHook(root, path string, expected, data []byte, hook func(os.FileMode) error) error {
	return WriteAtomicIfUnchangedWithHooks(root, path, expected, data, WriteHooks{PreparePublish: hook})
}

// WriteAtomicIfUnchangedWithHooks preserves edit freshness checks and repeats
// the same authority checks as WriteAtomicWithHooks.
func WriteAtomicIfUnchangedWithHooks(root, path string, expected, data []byte, hooks WriteHooks) error {
	return writeAtomicIfUnchangedWithModeHook(root, path, expected, 0, false, data, 0, false, hooks)
}

func writeAtomicIfUnchangedWithModeHook(root, path string, expected []byte, expectedMode os.FileMode, checkMode bool, data []byte, mode os.FileMode, setMode bool, hooks WriteHooks) error {
	rel, err := Relative(root, path)
	if err != nil {
		return err
	}
	if err := hooks.check(); err != nil {
		return err
	}
	current, err := openReadForOperation(root, path, hooks.CheckRootIdentity)
	if err != nil {
		if errors.Is(err, ErrWorkspaceParentNotExist) {
			return err
		}
		if isWorkspaceNotExist(err) {
			return fmt.Errorf("%w: %s is missing", ErrContentConflict, rel)
		}
		return err
	}
	info, statErr := current.Stat()
	if statErr != nil {
		_ = current.Close()
		return fmt.Errorf("stat workspace file %q for edit: %w", rel, statErr)
	}
	currentContent, readErr := io.ReadAll(io.LimitReader(current, int64(len(expected))+1))
	closeErr := current.Close()
	if readErr != nil {
		return fmt.Errorf("read workspace file %q for edit: %w", rel, readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close workspace file %q after edit check: %w", rel, closeErr)
	}
	if !bytes.Equal(currentContent, expected) || (checkMode && comparableMode(info.Mode()) != comparableMode(expectedMode)) {
		return fmt.Errorf("%w: %s", ErrContentConflict, rel)
	}
	return writeAtomicWithHook(root, path, data, mode, setMode, hooks)
}

func writeWorkspaceAll(file *os.File, data []byte) error {
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

// Remove removes the final regular-file name below root without following a
// symlink, hard link, or reparse point. The opened target identity is compared
// with the current final name before deletion. The final Unix unlink remains a
// pathname operation, so a replacement after that check is still outside this
// helper's universal same-UID race guarantee.
func Remove(root, path string) error {
	return RemoveWithChecks(root, path, nil, nil)
}

// RemoveWithCheck removes a regular file only after check succeeds against
// the descriptor-anchored parent that will perform the removal.
func RemoveWithCheck(root, path string, check func() error) error {
	return RemoveWithChecks(root, path, check, nil)
}

// RemoveWithChecks removes a regular file after checking caller authority and
// the identity of the opened workspace root that anchors the removal.
func RemoveWithChecks(root, path string, check func() error, checkRootIdentity RootIdentityCheck) error {
	return removeWithChecks(root, path, check, checkRootIdentity)
}

// RemoveIfUnchanged removes a regular file only when its current content
// still equals expected. A mismatch leaves the workspace untouched and
// returns ErrContentConflict. Like the write compare primitive, the check and
// pathname removal are a best-effort boundary for uncooperative same-UID
// writers; callers should also hold their project run lock when available.
func RemoveIfUnchanged(root, path string, expected []byte, expectedMode os.FileMode) error {
	return RemoveIfUnchangedWithChecks(root, path, expected, expectedMode, nil, nil)
}

// RemoveIfUnchangedWithCheck removes a regular file only when its bytes and
// mode still match expected and check succeeds against the descriptor-anchored
// parent that will perform the removal.
func RemoveIfUnchangedWithCheck(root, path string, expected []byte, expectedMode os.FileMode, check func() error) error {
	return RemoveIfUnchangedWithChecks(root, path, expected, expectedMode, check, nil)
}

// RemoveIfUnchangedWithChecks removes a regular file only when its bytes and
// mode still match expected, caller authority remains valid, and the opened
// workspace root has the expected identity.
func RemoveIfUnchangedWithChecks(root, path string, expected []byte, expectedMode os.FileMode, check func() error, checkRootIdentity RootIdentityCheck) error {
	return removeIfUnchanged(root, path, expected, expectedMode, nil, check, checkRootIdentity)
}

func removeIfUnchangedWithHook(root, path string, expected []byte, expectedMode os.FileMode, beforeRemove func() error) error {
	return removeIfUnchanged(root, path, expected, expectedMode, beforeRemove, nil, nil)
}

func removeIfUnchanged(root, path string, expected []byte, expectedMode os.FileMode, beforeRemove, check func() error, checkRootIdentity RootIdentityCheck) error {
	rel, err := Relative(root, path)
	if err != nil {
		return err
	}
	current, err := openReadForOperation(root, path, checkRootIdentity)
	if err != nil {
		if errors.Is(err, ErrWorkspaceParentNotExist) {
			return err
		}
		if isWorkspaceNotExist(err) {
			return fmt.Errorf("%w: %s is missing", ErrContentConflict, rel)
		}
		return err
	}
	info, statErr := current.Stat()
	if statErr != nil {
		_ = current.Close()
		return fmt.Errorf("stat workspace file %q for removal: %w", rel, statErr)
	}
	currentContent, readErr := io.ReadAll(io.LimitReader(current, int64(len(expected))+1))
	if readErr != nil {
		closeErr := current.Close()
		return errors.Join(fmt.Errorf("read workspace file %q for removal: %w", rel, readErr), closeErr)
	}
	if !bytes.Equal(currentContent, expected) || comparableMode(info.Mode()) != comparableMode(expectedMode) {
		closeErr := current.Close()
		return errors.Join(fmt.Errorf("%w: %s", ErrContentConflict, rel), closeErr)
	}
	if beforeRemove != nil {
		if hookErr := beforeRemove(); hookErr != nil {
			closeErr := current.Close()
			return errors.Join(fmt.Errorf("prepare workspace removal %q: %w", rel, hookErr), closeErr)
		}
	}
	removeErr := removeIfSameWithChecks(root, path, current, check, checkRootIdentity)
	closeErr := current.Close()
	return errors.Join(removeErr, closeErr)
}

func comparableMode(mode os.FileMode) os.FileMode {
	return mode.Perm() | mode&(fs.ModeSetuid|fs.ModeSetgid|fs.ModeSticky)
}
