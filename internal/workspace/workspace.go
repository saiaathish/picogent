// Package workspace provides descriptor-safe access to workspace-relative
// regular files. Callers must still perform their own permission decision;
// this package protects the subsequent filesystem operation from symlink and
// reparse-point path substitution.
package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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

// OpenRead opens a regular file below root without following path-component
// symlinks or reparse points.
func OpenRead(root, path string) (*os.File, error) {
	return open(root, path, openRead)
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

// Remove removes the final regular-file name below root without following a
// symlink or reparse point. It is intentionally name-based at the final
// component, so an attacker can cause a safe failure or removal of the
// replaced name inside the workspace, but can never redirect deletion outside
// the descriptor-anchored root.
func Remove(root, path string) error {
	return remove(root, path)
}
