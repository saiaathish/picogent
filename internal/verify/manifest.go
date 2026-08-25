package verify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	ManifestSchema       = "picogent.verify.v1"
	MaxManifestBytes     = 24 << 10
	maxManifestChecks    = 64
	maxManifestStringLen = 512
	maxGitOutputBytes    = 1 << 20
)

// ManifestStatus is deliberately separate from Status. UNVERIFIED is a
// release-evidence state and must not become a completion authorization state.
type ManifestStatus string

const (
	ManifestPass         ManifestStatus = "PASS"
	ManifestFail         ManifestStatus = "FAIL"
	ManifestInconclusive ManifestStatus = "INCONCLUSIVE"
	ManifestSkipped      ManifestStatus = "SKIPPED"
	ManifestUnverified   ManifestStatus = "UNVERIFIED"
)

type HeadEvidence struct {
	GitRoot     string         `json:"git_root,omitempty"`
	SHA         string         `json:"sha,omitempty"`
	ExpectedSHA string         `json:"expected_sha,omitempty"`
	Match       ManifestStatus `json:"match"`
	Tree        string         `json:"tree"`
	Reason      string         `json:"reason,omitempty"`
}

type PlatformEvidence struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
	Go   string `json:"go"`
}

type CoverageEvidence struct {
	Status  ManifestStatus `json:"status"`
	Percent *float64       `json:"percent,omitempty"`
	Reason  string         `json:"reason,omitempty"`
}

type CheckEvidence struct {
	Scope           Scope            `json:"scope"`
	Runner          string           `json:"runner,omitempty"`
	Command         string           `json:"command,omitempty"`
	Status          ManifestStatus   `json:"status"`
	Passed          int              `json:"passed,omitempty"`
	Failed          int              `json:"failed,omitempty"`
	DurationNS      int64            `json:"duration_ns"`
	Coverage        CoverageEvidence `json:"coverage"`
	OutputTruncated bool             `json:"output_truncated,omitempty"`
	Reason          string           `json:"reason,omitempty"`
}

type Manifest struct {
	Schema          string           `json:"schema"`
	Status          ManifestStatus   `json:"status"`
	Head            HeadEvidence     `json:"head"`
	Platform        PlatformEvidence `json:"platform"`
	DurationNS      int64            `json:"duration_ns"`
	Checks          []CheckEvidence  `json:"checks,omitempty"`
	ChecksTruncated bool             `json:"checks_truncated,omitempty"`
	Reason          string           `json:"reason,omitempty"`
}

// CollectProvenance gathers only fixed-argument, read-only Git facts. Missing
// or malformed evidence is represented in the result instead of returned as a
// successful-looking empty value.
func CollectProvenance(ctx context.Context, workspace, expectedSHA string) HeadEvidence {
	evidence := HeadEvidence{
		ExpectedSHA: boundedManifestString(strings.TrimSpace(expectedSHA)),
		Match:       ManifestUnverified,
		Tree:        "UNVERIFIED",
	}
	abs, err := absoluteWorkspace(workspace)
	if err != nil {
		evidence.Reason = "workspace is unavailable"
		return evidence
	}
	gitRoot, ok := gitText(ctx, abs, "rev-parse", "--show-toplevel")
	if !ok {
		evidence.Reason = "Git repository is unavailable"
		return evidence
	}
	gitRoot = strings.TrimSpace(gitRoot)
	if !filepath.IsAbs(gitRoot) {
		gitRoot, err = filepath.Abs(filepath.Join(abs, gitRoot))
		if err != nil {
			evidence.Reason = "Git root is unavailable"
			return evidence
		}
	}
	if resolved, resolveErr := filepath.EvalSymlinks(gitRoot); resolveErr == nil {
		gitRoot = resolved
	}
	evidence.GitRoot = boundedManifestString(filepath.Clean(gitRoot))

	head, headOK := gitText(ctx, abs, "rev-parse", "--verify", "HEAD^{commit}")
	head = strings.TrimSpace(head)
	if !headOK || !validManifestCommitID(head) {
		evidence.Reason = "committed HEAD is unavailable"
	} else {
		evidence.SHA = boundedManifestString(head)
		switch {
		case evidence.ExpectedSHA == "":
			evidence.Reason = "expected SHA was not provided"
		case !validManifestCommitID(evidence.ExpectedSHA):
			evidence.Reason = "expected SHA is not a full commit ID"
		case strings.EqualFold(head, evidence.ExpectedSHA):
			evidence.Match = ManifestPass
		default:
			evidence.Match = ManifestFail
			evidence.Reason = "HEAD does not match expected SHA"
		}
	}

	status, statusOK := gitText(ctx, abs, "status", "--porcelain=v1", "--untracked-files=normal")
	if !statusOK || len(status) >= maxGitOutputBytes {
		if evidence.Reason == "" {
			evidence.Reason = "Git worktree status is unavailable"
		}
		return evidence
	}
	if strings.TrimSpace(status) == "" {
		evidence.Tree = "CLEAN"
	} else {
		evidence.Tree = "DIRTY"
	}
	return evidence
}

// ManifestFromPipeline creates a bounded release-evidence projection without
// changing the pipeline result or any completion decision.
func ManifestFromPipeline(result PipelineResult, provenance HeadEvidence) Manifest {
	manifest := Manifest{
		Schema:     ManifestSchema,
		Head:       boundedHeadEvidence(provenance),
		Platform:   PlatformEvidence{OS: runtime.GOOS, Arch: runtime.GOARCH, Go: runtime.Version()},
		DurationNS: nonNegativeDuration(result.Duration),
	}
	for _, stage := range result.Stages {
		if len(stage.Evidence) == 0 {
			manifest.Checks = append(manifest.Checks, CheckEvidence{
				Scope:    stage.Scope,
				Status:   manifestStatus(stage.Status),
				Coverage: unverifiedCoverage(),
				Reason:   boundedManifestString(stage.Reason),
			})
			continue
		}
		for _, evidence := range stage.Evidence {
			scope := evidence.Scope
			if scope == "" {
				scope = stage.Scope
			}
			manifest.Checks = append(manifest.Checks, CheckEvidence{
				Scope:           scope,
				Runner:          boundedManifestString(evidence.Runner),
				Command:         boundedManifestString(evidence.Command),
				Status:          manifestStatus(evidence.Status),
				Passed:          nonNegativeInt(evidence.Passed),
				Failed:          nonNegativeInt(evidence.Failed),
				DurationNS:      nonNegativeDuration(evidence.Duration),
				Coverage:        unverifiedCoverage(),
				OutputTruncated: evidence.OutputTruncated,
				Reason:          boundedManifestString(evidence.Reason),
			})
		}
	}
	manifest.Checks, manifest.ChecksTruncated = boundChecks(manifest.Checks, false)
	manifest.Status, manifest.Reason = classifyManifest(result, manifest)
	return manifest
}

// WriteJSON writes a size-bounded, machine-readable manifest. Raw command
// output is never copied into the artifact.
func WriteJSON(w io.Writer, manifest Manifest) error {
	if w == nil {
		return errors.New("manifest writer is nil")
	}
	data, err := marshalManifest(manifest)
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

func marshalManifest(manifest Manifest) ([]byte, error) {
	manifest = boundedManifest(manifest)
	for {
		data, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return nil, err
		}
		if len(data)+1 <= MaxManifestBytes {
			return data, nil
		}
		if len(manifest.Checks) > 0 {
			manifest.Checks = manifest.Checks[:len(manifest.Checks)-1]
			manifest.ChecksTruncated = true
			continue
		}
		manifest.Reason = "manifest output truncated"
		minimal := Manifest{
			Schema:          manifest.Schema,
			Status:          ManifestUnverified,
			Head:            manifest.Head,
			Platform:        manifest.Platform,
			DurationNS:      manifest.DurationNS,
			ChecksTruncated: true,
			Reason:          manifest.Reason,
		}
		return json.MarshalIndent(minimal, "", "  ")
	}
}

func classifyManifest(result PipelineResult, manifest Manifest) (ManifestStatus, string) {
	switch result.Status {
	case StatusFail:
		return ManifestFail, firstReason(result.Reason, "verification failed")
	case StatusInconclusive:
		return ManifestInconclusive, firstReason(result.Reason, "verification is inconclusive")
	case StatusSkipped:
		return ManifestSkipped, firstReason(result.Reason, "verification was skipped")
	case StatusPass:
		if manifest.Head.Match == ManifestFail {
			return ManifestFail, firstReason(manifest.Head.Reason, "HEAD does not match expected SHA")
		}
		if manifest.Head.Match != ManifestPass {
			return ManifestUnverified, firstReason(manifest.Head.Reason, "HEAD provenance is unverified")
		}
		if manifest.Head.Tree != "CLEAN" {
			return ManifestUnverified, "worktree is not proven clean"
		}
		if len(manifest.Checks) == 0 {
			return ManifestUnverified, "no verification checks were recorded"
		}
		if manifest.ChecksTruncated {
			return ManifestUnverified, "verification checks were truncated"
		}
		for _, check := range manifest.Checks {
			if check.Status == ManifestFail {
				return ManifestFail, firstReason(check.Reason, "verification check failed")
			}
			if check.Status == ManifestInconclusive {
				return ManifestInconclusive, firstReason(check.Reason, "verification check is inconclusive")
			}
			if check.OutputTruncated {
				return ManifestUnverified, "verification output was truncated"
			}
			if check.Coverage.Status != ManifestPass {
				return ManifestUnverified, firstReason(check.Coverage.Reason, "required coverage is unverified")
			}
		}
		return ManifestPass, ""
	default:
		return ManifestUnverified, "pipeline returned an unknown status"
	}
}

func boundedManifest(manifest Manifest) Manifest {
	if manifest.Schema == "" {
		manifest.Schema = ManifestSchema
	}
	manifest.Schema = boundedManifestString(manifest.Schema)
	manifest.Status = normalizeManifestStatus(manifest.Status)
	manifest.Head = boundedHeadEvidence(manifest.Head)
	manifest.Platform.OS = boundedManifestString(manifest.Platform.OS)
	manifest.Platform.Arch = boundedManifestString(manifest.Platform.Arch)
	manifest.Platform.Go = boundedManifestString(manifest.Platform.Go)
	manifest.Reason = boundedManifestString(manifest.Reason)
	manifest.Checks, manifest.ChecksTruncated = boundChecks(manifest.Checks, manifest.ChecksTruncated)
	return manifest
}

func boundedHeadEvidence(evidence HeadEvidence) HeadEvidence {
	evidence.GitRoot = boundedManifestString(evidence.GitRoot)
	evidence.SHA = boundedManifestString(evidence.SHA)
	evidence.ExpectedSHA = boundedManifestString(evidence.ExpectedSHA)
	evidence.Reason = boundedManifestString(evidence.Reason)
	evidence.Match = normalizeManifestStatus(evidence.Match)
	if evidence.Tree != "CLEAN" && evidence.Tree != "DIRTY" {
		evidence.Tree = "UNVERIFIED"
	}
	return evidence
}

func boundChecks(checks []CheckEvidence, truncated bool) ([]CheckEvidence, bool) {
	checks = append([]CheckEvidence(nil), checks...)
	for i := range checks {
		checks[i].Runner = boundedManifestString(checks[i].Runner)
		checks[i].Command = boundedManifestString(checks[i].Command)
		checks[i].Reason = boundedManifestString(checks[i].Reason)
		checks[i].DurationNS = nonNegativeInt64(checks[i].DurationNS)
		checks[i].Passed = nonNegativeInt(checks[i].Passed)
		checks[i].Failed = nonNegativeInt(checks[i].Failed)
		checks[i].Status = normalizeManifestStatus(checks[i].Status)
		checks[i].Coverage.Reason = boundedManifestString(checks[i].Coverage.Reason)
		checks[i].Coverage.Status = normalizeManifestStatus(checks[i].Coverage.Status)
	}
	if len(checks) > maxManifestChecks {
		checks = checks[:maxManifestChecks]
		truncated = true
	}
	return checks, truncated
}

func normalizeManifestStatus(status ManifestStatus) ManifestStatus {
	switch status {
	case ManifestPass, ManifestFail, ManifestInconclusive, ManifestSkipped, ManifestUnverified:
		return status
	default:
		return ManifestUnverified
	}
}

func unverifiedCoverage() CoverageEvidence {
	return CoverageEvidence{Status: ManifestUnverified, Reason: "coverage not collected"}
}

func manifestStatus(status Status) ManifestStatus {
	switch status {
	case StatusPass:
		return ManifestPass
	case StatusFail:
		return ManifestFail
	case StatusInconclusive:
		return ManifestInconclusive
	case StatusSkipped:
		return ManifestSkipped
	default:
		return ManifestUnverified
	}
}

func firstReason(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return boundedManifestString(primary)
	}
	return fallback
}

func boundedManifestString(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\x00", ""))
	if len(value) <= maxManifestStringLen {
		return value
	}
	return value[:maxManifestStringLen-3] + "..."
}

func nonNegativeDuration(duration time.Duration) int64 {
	if duration < 0 {
		return 0
	}
	return duration.Nanoseconds()
}

func nonNegativeInt(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func nonNegativeInt64(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func absoluteWorkspace(workspace string) (string, error) {
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	st, err := os.Stat(abs)
	if err != nil || !st.IsDir() {
		return "", errors.New("workspace is not a directory")
	}
	return abs, nil
}

func gitText(ctx context.Context, workspace string, args ...string) (string, bool) {
	commandCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, "git", args...)
	cmd.Dir = workspace
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil || output.Len() > maxGitOutputBytes {
		return "", false
	}
	return output.String(), true
}

func validManifestCommitID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, r := range value {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}
