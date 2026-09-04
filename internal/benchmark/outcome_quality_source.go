package benchmark

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/saiaathish/picogent/internal/gitobs"
	"github.com/saiaathish/picogent/internal/verify"
)

// OutcomeQualitySourceBinding connects one report target to the source tree
// that will execute it. Workspace paths are runtime-only inputs and are not
// serialized into the outcome-quality report.
type OutcomeQualitySourceBinding struct {
	Target    OutcomeQualityTarget
	Workspace string
}

// ValidateOutcomeQualitySourcePair verifies the preflight identity required
// before a comparative run. Each target must point at a distinct, absolute,
// clean Git worktree whose committed HEAD matches its declared source head.
//
// This is source-tree evidence, not binary provenance or live-provider
// quality evidence. A caller still owns the build/launch step and must keep
// the shared input and policy identical for both targets.
func ValidateOutcomeQualitySourcePair(ctx context.Context, baseline, candidate OutcomeQualitySourceBinding) error {
	if ctx == nil {
		ctx = context.Background()
	}
	baselineWorkspace, err := canonicalOutcomeQualityWorkspace(baseline.Workspace)
	if err != nil {
		return fmt.Errorf("baseline workspace: %w", err)
	}
	candidateWorkspace, err := canonicalOutcomeQualityWorkspace(candidate.Workspace)
	if err != nil {
		return fmt.Errorf("candidate workspace: %w", err)
	}
	if baselineWorkspace == candidateWorkspace {
		return fmt.Errorf("baseline and candidate must use distinct workspaces")
	}

	if err := validateOutcomeQualitySourceTargetPair(baseline.Target, candidate.Target); err != nil {
		return err
	}
	if err := validateOutcomeQualitySourceBinding(ctx, "baseline", baseline.Target, baselineWorkspace); err != nil {
		return err
	}
	if err := validateOutcomeQualitySourceBinding(ctx, "candidate", candidate.Target, candidateWorkspace); err != nil {
		return err
	}
	return nil
}

func validateOutcomeQualitySourceTargetPair(baseline, candidate OutcomeQualityTarget) error {
	if err := validateOutcomeQualityTarget("baseline", baseline); err != nil {
		return err
	}
	if err := validateOutcomeQualityTarget("candidate", candidate); err != nil {
		return err
	}
	if strings.EqualFold(baseline.SourceHead, candidate.SourceHead) {
		return fmt.Errorf("baseline and candidate must use different source heads")
	}
	if baseline.Host != candidate.Host || baseline.GoVersion != candidate.GoVersion || baseline.ToolVersion != candidate.ToolVersion {
		return fmt.Errorf("baseline and candidate must share host, Go version, and tool version")
	}
	return nil
}

func outcomeQualityTargetsEqual(left, right OutcomeQualityTarget) bool {
	return strings.EqualFold(left.SourceHead, right.SourceHead) &&
		left.Host == right.Host &&
		left.GoVersion == right.GoVersion &&
		left.ToolVersion == right.ToolVersion
}

func canonicalOutcomeQualityWorkspace(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("workspace is required")
	}
	if !filepath.IsAbs(raw) {
		return "", fmt.Errorf("workspace must be an absolute path")
	}
	info, err := os.Stat(raw)
	if err != nil {
		return "", fmt.Errorf("workspace is unavailable")
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace is not a directory")
	}
	resolved, err := filepath.EvalSymlinks(raw)
	if err != nil {
		return "", fmt.Errorf("workspace cannot be resolved")
	}
	return filepath.Clean(resolved), nil
}

func validateOutcomeQualitySourceBinding(ctx context.Context, name string, target OutcomeQualityTarget, workspace string) error {
	evidence := verify.CollectProvenance(ctx, workspace, target.SourceHead)
	if evidence.Match != verify.ManifestPass {
		reason := strings.TrimSpace(evidence.Reason)
		if reason == "" {
			reason = "committed HEAD does not match the declared source head"
		}
		return fmt.Errorf("%s source is not at its declared clean head: %s", name, reason)
	}
	if evidence.Tree != "CLEAN" {
		return fmt.Errorf("%s source worktree is not clean", name)
	}
	status, err := gitobs.Output(ctx, workspace, "status", "--porcelain=v1", "--untracked-files=all", "--ignored")
	if err != nil || status.Truncated {
		return fmt.Errorf("%s source clean-tree status is unavailable", name)
	}
	if strings.TrimSpace(status.Output) != "" {
		return fmt.Errorf("%s source worktree is not clean", name)
	}
	return nil
}
