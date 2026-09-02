package verify

import (
	"fmt"
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
	workspace = strings.TrimSpace(workspace)
	evidenceDir = strings.TrimSpace(evidenceDir)
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
	evidenceAbs, err := filepath.Abs(filepath.Clean(evidenceDir))
	if err != nil {
		return fmt.Errorf("resolve release evidence directory: %w", err)
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
