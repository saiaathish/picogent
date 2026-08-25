package tools

import (
	"fmt"
	"path/filepath"
	"strings"
)

// secureWorkspaceRelative converts the already-resolved path used by the
// permission layer into a path relative to the workspace descriptor root.
// The platform-specific openers then use that relative path without following
// any path component supplied by a hostile workspace.
func secureWorkspaceRelative(workspace, abs string) (string, error) {
	if strings.TrimSpace(workspace) == "" || strings.TrimSpace(abs) == "" {
		return "", fmt.Errorf("workspace path is empty")
	}
	root, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	target, err := filepath.Abs(abs)
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
	if _, err := securePathParts(rel); err != nil {
		return "", err
	}
	return filepath.Clean(rel), nil
}

func securePathParts(rel string) ([]string, error) {
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
