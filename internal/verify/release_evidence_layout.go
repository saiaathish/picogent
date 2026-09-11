package verify

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateReleaseEvidenceDirectory enforces the provenance boundary used by
// the release-evidence workflow. Generated evidence must be kept outside the
// checked-out workspace until CollectProvenance has captured the tree state.
//
// This is a lexical layout guard, not a claim about hostile filesystem
// writers or symlink races. The workflow owns the temporary directory and
// should still fail closed when it cannot create or write it.
func ValidateReleaseEvidenceDirectory(workspace, evidenceDir string) error {
	if workspace != strings.TrimSpace(workspace) {
		return fmt.Errorf("release evidence workspace must not have surrounding whitespace")
	}
	if evidenceDir != strings.TrimSpace(evidenceDir) {
		return fmt.Errorf("release evidence directory must not have surrounding whitespace")
	}
	if workspace == "" {
		return fmt.Errorf("release evidence workspace is required")
	}
	if evidenceDir == "" {
		return fmt.Errorf("release evidence directory is required")
	}
	if !filepath.IsAbs(workspace) {
		return fmt.Errorf("release evidence workspace must be absolute")
	}
	if !filepath.IsAbs(evidenceDir) {
		return fmt.Errorf("release evidence directory must be absolute")
	}

	workspaceAbs, err := filepath.Abs(filepath.Clean(workspace))
	if err != nil {
		return fmt.Errorf("resolve release evidence workspace: %w", err)
	}
	workspaceAbs, err = resolveKnownReleasePath(workspaceAbs)
	if err != nil {
		return fmt.Errorf("resolve release evidence workspace links: %w", err)
	}
	evidenceAbs, err := filepath.Abs(filepath.Clean(evidenceDir))
	if err != nil {
		return fmt.Errorf("resolve release evidence directory: %w", err)
	}
	evidenceAbs, err = resolveKnownReleasePath(evidenceAbs)
	if err != nil {
		return fmt.Errorf("resolve release evidence directory links: %w", err)
	}
	relative, err := filepath.Rel(workspaceAbs, evidenceAbs)
	if err != nil {
		return fmt.Errorf("compare release evidence paths: %w", err)
	}
	if relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		return fmt.Errorf("release evidence directory %q is inside workspace %q", evidenceAbs, workspaceAbs)
	}
	return nil
}

// resolveKnownReleasePath resolves every existing path component while
// preserving a not-yet-created leaf. This catches a workspace alias or an
// evidence parent symlink before the lexical containment check, while still
// allowing the workflow to create its fresh runner-temporary directory later.
func resolveKnownReleasePath(path string) (string, error) {
	path = filepath.Clean(path)
	var suffix []string
	for current := path; ; current = filepath.Dir(current) {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return path, nil
		}
		suffix = append(suffix, filepath.Base(current))
	}
}
