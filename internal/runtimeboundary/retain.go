package runtimeboundary

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/saiaathish/picogent/internal/securefile"
)

// ValidateArtifactPath requires an absolute artifact path outside the clean
// workspace after resolving both trees. Symlinked parents are rejected by
// RetainReport before creation; this helper remains the lexical/resolved
// containment gate.
func ValidateArtifactPath(workspace, artifactPath string) error {
	workspaceAbs, artifactAbs, err := resolveArtifactPaths(workspace, artifactPath)
	if err != nil {
		return err
	}
	return ensureOutsideWorkspace(workspaceAbs, artifactAbs)
}

func resolveArtifactPaths(workspace, artifactPath string) (string, string, error) {
	if workspace != strings.TrimSpace(workspace) {
		return "", "", errors.New("runtime matrix workspace must not have surrounding whitespace")
	}
	if artifactPath != strings.TrimSpace(artifactPath) {
		return "", "", errors.New("runtime matrix artifact path must not have surrounding whitespace")
	}
	if workspace == "" {
		return "", "", errors.New("runtime matrix workspace is required")
	}
	if artifactPath == "" {
		return "", "", errors.New("runtime matrix artifact path is required")
	}
	if !filepath.IsAbs(workspace) {
		return "", "", errors.New("runtime matrix workspace must be absolute")
	}
	if !filepath.IsAbs(artifactPath) {
		return "", "", errors.New("runtime matrix artifact path must be absolute")
	}

	workspaceAbs, err := filepath.Abs(filepath.Clean(workspace))
	if err != nil {
		return "", "", fmt.Errorf("resolve runtime matrix workspace: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(workspaceAbs); err == nil {
		workspaceAbs = filepath.Clean(resolved)
	}
	artifactAbs, err := filepath.Abs(filepath.Clean(artifactPath))
	if err != nil {
		return "", "", fmt.Errorf("resolve runtime matrix artifact path: %w", err)
	}
	parent := filepath.Dir(artifactAbs)
	base := filepath.Base(artifactAbs)
	if resolvedParent, err := filepath.EvalSymlinks(parent); err == nil {
		artifactAbs = filepath.Join(filepath.Clean(resolvedParent), base)
	}
	return workspaceAbs, artifactAbs, nil
}

func ensureOutsideWorkspace(workspaceAbs, artifactAbs string) error {
	relative, err := filepath.Rel(workspaceAbs, artifactAbs)
	if err != nil {
		return fmt.Errorf("compare runtime matrix paths: %w", err)
	}
	if relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		return fmt.Errorf("runtime matrix artifact %q is inside workspace %q", artifactAbs, workspaceAbs)
	}
	return nil
}

// RetainReport writes a matrix report exclusively beneath a real parent
// directory outside the workspace. Parents are created with securefile so
// application-created symlink components are rejected; the exclusive create
// is anchored through os.OpenRoot so a later parent pathname swap cannot
// redirect the write after the directory handle is open. Existing files fail
// closed. This is not a proof against every same-UID TOCTOU race on every OS.
func RetainReport(workspace, artifactPath string, report Report) error {
	workspaceAbs, artifactAbs, err := resolveArtifactPaths(workspace, artifactPath)
	if err != nil {
		return err
	}
	if err := ensureOutsideWorkspace(workspaceAbs, artifactAbs); err != nil {
		return err
	}
	if err := validateRetainedReport(report, report.CandidateSHA); err != nil {
		return err
	}
	data, err := encodeReport(report)
	if err != nil {
		return err
	}

	// Use the caller-supplied absolute parent before symlink evaluation so a
	// hostile symlink component is rejected by securefile instead of followed.
	lexicalArtifact, err := filepath.Abs(filepath.Clean(artifactPath))
	if err != nil {
		return fmt.Errorf("resolve runtime matrix artifact path: %w", err)
	}
	parent := filepath.Dir(lexicalArtifact)
	base := filepath.Base(lexicalArtifact)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return errors.New("runtime matrix artifact basename is required")
	}
	if err := securefile.EnsureDir(parent, 0o700); err != nil {
		return fmt.Errorf("prepare runtime matrix artifact directory: %w", err)
	}
	parentResolved, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return fmt.Errorf("resolve runtime matrix artifact parent: %w", err)
	}
	parentResolved = filepath.Clean(parentResolved)
	finalAbs := filepath.Join(parentResolved, base)
	if err := ensureOutsideWorkspace(workspaceAbs, finalAbs); err != nil {
		return err
	}
	if err := rejectSymlinkAncestors(parentResolved); err != nil {
		return err
	}

	root, err := os.OpenRoot(parentResolved)
	if err != nil {
		return fmt.Errorf("open runtime matrix artifact parent: %w", err)
	}
	defer root.Close()

	file, err := root.OpenFile(base, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("runtime matrix artifact %q already exists; refuse overwrite", finalAbs)
		}
		return fmt.Errorf("create runtime matrix artifact: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = root.Remove(base)
		return fmt.Errorf("write runtime matrix artifact: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = root.Remove(base)
		return fmt.Errorf("close runtime matrix artifact: %w", err)
	}

	info, err := root.Lstat(base)
	if err != nil {
		_ = root.Remove(base)
		return fmt.Errorf("inspect retained runtime matrix artifact: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		_ = root.Remove(base)
		return errors.New("retained runtime matrix artifact is not a regular file")
	}
	return nil
}

func rejectSymlinkAncestors(path string) error {
	path = filepath.Clean(path)
	volume := filepath.VolumeName(path)
	rootPath := volume + string(filepath.Separator)
	if rootPath == string(filepath.Separator) && strings.HasPrefix(path, string(filepath.Separator)) {
		rootPath = string(filepath.Separator)
	}
	rest := strings.TrimPrefix(path, rootPath)
	if rest == path && volume == "" {
		return fmt.Errorf("runtime matrix parent %q is not absolute", path)
	}
	current := rootPath
	info, err := os.Lstat(current)
	if err != nil {
		return fmt.Errorf("inspect runtime matrix parent root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("runtime matrix parent root %q is not a real directory", current)
	}
	for _, part := range strings.FieldsFunc(rest, func(r rune) bool { return r == rune(filepath.Separator) }) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("inspect runtime matrix parent %q: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("runtime matrix parent %q is not a real directory", current)
		}
	}
	return nil
}

// LoadReport loads a retained matrix artifact and fail-closes on missing,
// malformed, oversized, schema-mismatched, or candidate-mismatched records.
func LoadReport(artifactPath, expectedSHA string) (Report, error) {
	var report Report
	if strings.TrimSpace(artifactPath) == "" {
		return report, errors.New("runtime matrix artifact path is required")
	}
	if !validCommitSHA(strings.TrimSpace(expectedSHA)) {
		return report, errors.New("expected candidate_sha must be a full commit id")
	}
	info, err := os.Lstat(artifactPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return report, fmt.Errorf("runtime matrix artifact missing: %w", err)
		}
		return report, fmt.Errorf("inspect runtime matrix artifact: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return report, errors.New("runtime matrix artifact must be a regular file")
	}
	if info.Size() > int64(MaxReportBytes) {
		return report, errors.New("runtime matrix artifact exceeds size limit")
	}
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		return report, fmt.Errorf("read runtime matrix artifact: %w", err)
	}
	if len(data) == 0 {
		return report, errors.New("runtime matrix artifact is empty")
	}
	if len(data) > MaxReportBytes {
		return report, errors.New("runtime matrix artifact exceeds size limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return Report{}, fmt.Errorf("decode runtime matrix artifact: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return Report{}, errors.New("runtime matrix artifact has trailing JSON")
	}
	if err := validateRetainedReport(report, expectedSHA); err != nil {
		return Report{}, err
	}
	return report, nil
}

func encodeReport(report Report) ([]byte, error) {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode runtime matrix artifact: %w", err)
	}
	data = append(data, '\n')
	if len(data) > MaxReportBytes {
		return nil, errors.New("runtime matrix artifact exceeds size limit")
	}
	return data, nil
}

func validateRetainedReport(report Report, expectedSHA string) error {
	if report.Schema != Schema {
		return fmt.Errorf("runtime matrix schema %q does not match %q", report.Schema, Schema)
	}
	if !validCommitSHA(report.CandidateSHA) {
		return errors.New("runtime matrix candidate_sha must be a full commit id")
	}
	expectedSHA = strings.TrimSpace(expectedSHA)
	if report.CandidateSHA != expectedSHA {
		return fmt.Errorf("runtime matrix candidate_sha %q does not match expected %q", report.CandidateSHA, expectedSHA)
	}
	if report.HeadMatch != "PASS" {
		return fmt.Errorf("runtime matrix head.match must be PASS, got %q", report.HeadMatch)
	}
	if report.Tree != "CLEAN" {
		return fmt.Errorf("runtime matrix tree must be CLEAN, got %q", report.Tree)
	}
	if len(report.Claims) == 0 {
		return errors.New("runtime matrix has no claims")
	}
	if len(report.Claims) > MaxClaims {
		return errors.New("runtime matrix exceeds claim cap")
	}
	seen := make(map[string]struct{}, len(report.Claims))
	for _, claim := range report.Claims {
		if strings.TrimSpace(claim.ID) == "" {
			return errors.New("runtime matrix claim id is required")
		}
		if _, ok := seen[claim.ID]; ok {
			return fmt.Errorf("runtime matrix has duplicate claim id %q", claim.ID)
		}
		seen[claim.ID] = struct{}{}
		if !claim.Verdict.valid() {
			return fmt.Errorf("runtime matrix claim %s has invalid verdict %q", claim.ID, claim.Verdict)
		}
	}
	return nil
}
