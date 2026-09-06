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
)

// ValidateArtifactPath requires an absolute artifact path outside the clean
// workspace. This is a lexical layout guard, not a hostile-filesystem claim.
func ValidateArtifactPath(workspace, artifactPath string) error {
	if workspace != strings.TrimSpace(workspace) {
		return errors.New("runtime matrix workspace must not have surrounding whitespace")
	}
	if artifactPath != strings.TrimSpace(artifactPath) {
		return errors.New("runtime matrix artifact path must not have surrounding whitespace")
	}
	if workspace == "" {
		return errors.New("runtime matrix workspace is required")
	}
	if artifactPath == "" {
		return errors.New("runtime matrix artifact path is required")
	}
	if !filepath.IsAbs(workspace) {
		return errors.New("runtime matrix workspace must be absolute")
	}
	if !filepath.IsAbs(artifactPath) {
		return errors.New("runtime matrix artifact path must be absolute")
	}

	workspaceAbs, err := filepath.Abs(filepath.Clean(workspace))
	if err != nil {
		return fmt.Errorf("resolve runtime matrix workspace: %w", err)
	}
	artifactAbs, err := filepath.Abs(filepath.Clean(artifactPath))
	if err != nil {
		return fmt.Errorf("resolve runtime matrix artifact path: %w", err)
	}
	relative, err := filepath.Rel(workspaceAbs, artifactAbs)
	if err != nil {
		return fmt.Errorf("compare runtime matrix paths: %w", err)
	}
	if relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		return fmt.Errorf("runtime matrix artifact %q is inside workspace %q", artifactAbs, workspaceAbs)
	}
	return nil
}

// RetainReport writes a matrix report exclusively to an external artifact path.
// Existing files fail closed so retained provenance cannot be silently replaced.
func RetainReport(workspace, artifactPath string, report Report) error {
	if err := ValidateArtifactPath(workspace, artifactPath); err != nil {
		return err
	}
	if err := validateRetainedReport(report, report.CandidateSHA); err != nil {
		return err
	}
	data, err := encodeReport(report)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(artifactPath), 0o700); err != nil {
		return fmt.Errorf("create runtime matrix artifact directory: %w", err)
	}
	file, err := os.OpenFile(artifactPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("runtime matrix artifact %q already exists; refuse overwrite", artifactPath)
		}
		return fmt.Errorf("create runtime matrix artifact: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		_ = os.Remove(artifactPath)
		return fmt.Errorf("write runtime matrix artifact: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(artifactPath)
		return fmt.Errorf("close runtime matrix artifact: %w", err)
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
