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
// directory outside the workspace. securefile owns parent creation and
// descriptor/handle-anchored publication; existing files fail closed. This is
// not a proof against every same-UID TOCTOU race on every OS.
func RetainReport(workspace, artifactPath string, report Report) error {
	workspaceAbs, artifactAbs, err := resolveArtifactPaths(workspace, artifactPath)
	if err != nil {
		return err
	}
	if err := ensureOutsideWorkspace(workspaceAbs, artifactAbs); err != nil {
		return err
	}
	report = normalizeReportBehavior(report)
	if err := validateRetainedReport(report, report.CandidateSHA); err != nil {
		return err
	}
	data, err := encodeReport(report)
	if err != nil {
		return err
	}
	if err := securefile.WriteExclusive(artifactPath, data, 0o600); err != nil {
		return fmt.Errorf("retain runtime matrix artifact: %w", err)
	}
	return nil
}

// LoadReport loads a retained matrix artifact and fail-closes on missing,
// malformed, oversized, schema-mismatched, or candidate-mismatched records.
// securefile.ReadFileLimited opens the parent through its descriptor/handle-
// anchored platform primitive, so the read does not reopen the artifact path
// after parent validation. This narrows pathname replacement races without
// claiming broad same-UID TOCTOU resistance on every supported platform.
func LoadReport(artifactPath, expectedSHA string) (Report, error) {
	var report Report
	if strings.TrimSpace(artifactPath) == "" {
		return report, errors.New("runtime matrix artifact path is required")
	}
	if !validCommitSHA(strings.TrimSpace(expectedSHA)) {
		return report, errors.New("expected candidate_sha must be a full commit id")
	}
	data, err := securefile.ReadFileLimited(artifactPath, MaxReportBytes)
	if err != nil {
		if errors.Is(err, securefile.ErrReadLimit) {
			return report, errors.New("runtime matrix artifact exceeds size limit")
		}
		if errors.Is(err, os.ErrNotExist) {
			return report, fmt.Errorf("runtime matrix artifact missing: %w", err)
		}
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
	report = normalizeReportBehavior(report)
	if err := validateRetainedReport(report, expectedSHA); err != nil {
		return Report{}, err
	}
	return report, nil
}

func normalizeReportBehavior(report Report) Report {
	// Reports produced before behavior-SHA continuity were exact-head-only.
	// Treat absence of both additive fields as that stricter legacy contract.
	if report.BehaviorSHA == "" && report.BehaviorProvenance == "" {
		report.BehaviorSHA = report.CandidateSHA
		report.BehaviorProvenance = BehaviorProvenanceExact
	}
	return report
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
	if !validCommitSHA(report.BehaviorSHA) {
		return errors.New("runtime matrix behavior_sha must be a full commit id")
	}
	switch report.BehaviorProvenance {
	case BehaviorProvenanceExact:
		if report.BehaviorSHA != report.CandidateSHA {
			return errors.New("runtime matrix exact behavior provenance requires matching candidate and behavior SHAs")
		}
	case BehaviorProvenanceDocsOnlyDescendant:
		if report.BehaviorSHA == report.CandidateSHA {
			return errors.New("runtime matrix docs-only behavior provenance requires distinct candidate and behavior SHAs")
		}
	default:
		return fmt.Errorf("runtime matrix behavior provenance %q is invalid", report.BehaviorProvenance)
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
