// Package workspace provides descriptor-safe access to workspace-relative
// files and directories. Callers must still perform their own permission
// decision; this package protects the subsequent filesystem operation from
// symlink and reparse-point path substitution.
package workspace

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ErrContentConflict reports that a file changed after an edit operation read
// it. The caller must re-read the file and recompute the edit.
var ErrContentConflict = errors.New("workspace file changed during edit")

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
// symlinks or reparse points.
func OpenRead(root, path string) (*os.File, error) {
	return open(root, path, openRead)
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
// path-component symlinks or reparse points. It does not truncate the file;
// callers can validate cancellation before truncating the returned handle.
func OpenWrite(root, path string) (*os.File, error) {
	return open(root, path, openWrite)
}

// OpenEdit opens an existing regular file below root for read/write without
// following path-component symlinks or reparse points.
func OpenEdit(root, path string) (*os.File, error) {
	return open(root, path, openEdit)
}

// WriteAtomic writes a complete file below root and publishes it as one
// filesystem replacement. Missing parent directories are created like
// OpenWrite. Existing regular files retain their permission bits; symlinked
// or non-regular targets are rejected. Ordinary path readers therefore see
// either the previous complete file or the new complete file, not a live
// truncate-and-rewrite. Like all pathname-based filesystem publication, this
// does not make a directory writable by an uncooperative same-UID attacker
// safe from every name-replacement race.
func WriteAtomic(root, path string, data []byte) error {
	return writeAtomic(root, path, data)
}

// WriteAtomicIfUnchanged publishes data only when the current regular file
// content still equals expected. A mismatch leaves the current file untouched
// and returns ErrContentConflict. The content check and publication are a
// best-effort compare-then-publish boundary, not an atomic cross-process CAS:
// another writer can replace the file after the check, and the later pathname
// lookup can observe a replacement workspace root. This does not provide a
// hostile same-UID filesystem race barrier or cross-process lock.
func WriteAtomicIfUnchanged(root, path string, expected, data []byte) error {
	rel, err := Relative(root, path)
	if err != nil {
		return err
	}
	current, err := OpenRead(root, path)
	if err != nil {
		if isWorkspaceNotExist(err) {
			return fmt.Errorf("%w: %s is missing", ErrContentConflict, rel)
		}
		return err
	}
	currentContent, readErr := io.ReadAll(io.LimitReader(current, int64(len(expected))+1))
	closeErr := current.Close()
	if readErr != nil {
		return fmt.Errorf("read workspace file %q for edit: %w", rel, readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close workspace file %q after edit check: %w", rel, closeErr)
	}
	if !bytes.Equal(currentContent, expected) {
		return fmt.Errorf("%w: %s", ErrContentConflict, rel)
	}
	return writeAtomic(root, path, data)
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
// symlink or reparse point. It is intentionally name-based at the final
// component, so an attacker can cause a safe failure or removal of the
// replaced name inside the workspace, but can never redirect deletion outside
// the descriptor-anchored root.
func Remove(root, path string) error {
	return remove(root, path)
}
