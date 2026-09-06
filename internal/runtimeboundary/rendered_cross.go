package runtimeboundary

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/securefile"
)

const (
	RenderedCrossEvidenceSchema   = "picogent.v4.rendered-cross-platform-evidence.v1"
	MaxRenderedCrossEvidenceBytes = 64 << 10
	RenderedCrossEvidenceEnv      = "PICOGENT_RENDERED_CROSS_PLATFORM_EVIDENCE"
	RenderedCrossArtifactEnv      = "PICOGENT_RENDERED_CROSS_PLATFORM_ARTIFACT"
)

var requiredRenderedCrossPlatforms = []string{"darwin", "linux", "windows"}

// RenderedCrossPlatformEvidence aggregates per-platform rendered observations
// bound to one candidate SHA. PASS requires every required desktop platform.
type RenderedCrossPlatformEvidence struct {
	Schema             string                              `json:"schema"`
	CandidateSHA       string                              `json:"candidate_sha"`
	Environment        string                              `json:"environment"`
	RequiredPlatforms  []string                            `json:"required_platforms"`
	Platforms          []RenderedCrossPlatformEntryEvidence `json:"platforms"`
	ObservedAt         string                              `json:"observed_at"`
	Verdict            Verdict                             `json:"verdict"`
	SourceTreeModified bool                                `json:"source_tree_modified"`
}

// RenderedCrossPlatformEntryEvidence is one platform's digest-only observation.
type RenderedCrossPlatformEntryEvidence struct {
	Platform          string  `json:"platform"`
	Architecture      string  `json:"architecture"`
	Browser           string  `json:"browser"`
	Fixture           string  `json:"fixture"`
	ObservationSHA256 string  `json:"observation_sha256"`
	ScreenshotSHA256  string  `json:"screenshot_sha256"`
	ObservedAt        string  `json:"observed_at"`
	Verdict           Verdict `json:"verdict"`
}

func loadRenderedCrossPlatformEvidence(workspace, artifactPath, expectedSHA string) (RenderedCrossPlatformEvidence, string, error) {
	var evidence RenderedCrossPlatformEvidence
	if err := ValidateArtifactPath(workspace, artifactPath); err != nil {
		return evidence, "", fmt.Errorf("rendered-cross-platform evidence path: %w", err)
	}
	data, err := securefile.ReadFileLimited(artifactPath, MaxRenderedCrossEvidenceBytes)
	if err != nil {
		if errors.Is(err, securefile.ErrReadLimit) {
			return evidence, "", errors.New("rendered-cross-platform evidence exceeds size limit")
		}
		return evidence, "", fmt.Errorf("read rendered-cross-platform evidence: %w", err)
	}
	if len(data) == 0 {
		return evidence, "", errors.New("rendered-cross-platform evidence is empty")
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&evidence); err != nil {
		return RenderedCrossPlatformEvidence{}, "", fmt.Errorf("decode rendered-cross-platform evidence: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return RenderedCrossPlatformEvidence{}, "", errors.New("rendered-cross-platform evidence has trailing JSON")
	}
	if err := requireRenderedCrossAssertions(data); err != nil {
		return RenderedCrossPlatformEvidence{}, "", err
	}
	if err := validateRenderedCrossPlatformEvidence(evidence, expectedSHA); err != nil {
		return RenderedCrossPlatformEvidence{}, "", err
	}
	digest := sha256.Sum256(data)
	return evidence, hex.EncodeToString(digest[:]), nil
}

func requireRenderedCrossAssertions(data []byte) error {
	var assertions struct {
		SourceTreeModified *bool `json:"source_tree_modified"`
	}
	if err := json.Unmarshal(data, &assertions); err != nil {
		return fmt.Errorf("decode rendered-cross-platform assertions: %w", err)
	}
	if assertions.SourceTreeModified == nil {
		return errors.New("rendered-cross-platform evidence must explicitly assert source_tree_modified")
	}
	return nil
}

func validateRenderedCrossPlatformEvidence(evidence RenderedCrossPlatformEvidence, expectedSHA string) error {
	if evidence.Schema != RenderedCrossEvidenceSchema {
		return fmt.Errorf("rendered-cross-platform evidence schema %q does not match %q", evidence.Schema, RenderedCrossEvidenceSchema)
	}
	if !validCommitSHA(evidence.CandidateSHA) || evidence.CandidateSHA != strings.TrimSpace(expectedSHA) {
		return errors.New("rendered-cross-platform evidence candidate_sha does not match expected SHA")
	}
	if evidence.Environment != "task-owned-disposable" {
		return errors.New("rendered-cross-platform evidence environment is not task-owned-disposable")
	}
	if _, err := time.Parse(time.RFC3339, evidence.ObservedAt); err != nil {
		return fmt.Errorf("rendered-cross-platform evidence observed_at is invalid: %w", err)
	}
	if !evidence.Verdict.valid() {
		return fmt.Errorf("rendered-cross-platform evidence verdict is invalid: %q", evidence.Verdict)
	}
	if evidence.SourceTreeModified {
		return errors.New("rendered-cross-platform evidence source tree must be clean")
	}
	if err := requireExactStringSet(evidence.RequiredPlatforms, requiredRenderedCrossPlatforms); err != nil {
		return fmt.Errorf("rendered-cross-platform required_platforms: %w", err)
	}
	if len(evidence.Platforms) == 0 {
		return errors.New("rendered-cross-platform evidence has no platform entries")
	}

	seen := make(map[string]struct{}, len(evidence.Platforms))
	for _, entry := range evidence.Platforms {
		if !validRenderedPlatform(entry.Platform) {
			return fmt.Errorf("rendered-cross-platform entry platform %q is invalid", entry.Platform)
		}
		if _, dup := seen[entry.Platform]; dup {
			return fmt.Errorf("rendered-cross-platform entry platform %q is duplicated", entry.Platform)
		}
		seen[entry.Platform] = struct{}{}
		if !validEvidenceIdentifier(entry.Architecture) {
			return fmt.Errorf("rendered-cross-platform entry %q architecture is invalid", entry.Platform)
		}
		if !validEvidenceIdentifier(entry.Browser) || !validEvidenceIdentifier(entry.Fixture) {
			return fmt.Errorf("rendered-cross-platform entry %q browser or fixture is invalid", entry.Platform)
		}
		if !validDigest(entry.ObservationSHA256) {
			return fmt.Errorf("rendered-cross-platform entry %q observation digest is invalid", entry.Platform)
		}
		if entry.ScreenshotSHA256 != "UNRECORDED" && !validDigest(entry.ScreenshotSHA256) {
			return fmt.Errorf("rendered-cross-platform entry %q screenshot digest is invalid", entry.Platform)
		}
		if _, err := time.Parse(time.RFC3339, entry.ObservedAt); err != nil {
			return fmt.Errorf("rendered-cross-platform entry %q observed_at is invalid: %w", entry.Platform, err)
		}
		if !entry.Verdict.valid() {
			return fmt.Errorf("rendered-cross-platform entry %q verdict is invalid: %q", entry.Platform, entry.Verdict)
		}
	}

	if evidence.Verdict != VerdictPass {
		return nil
	}
	for _, required := range requiredRenderedCrossPlatforms {
		if _, ok := seen[required]; !ok {
			return fmt.Errorf("rendered-cross-platform PASS is missing required platform %q", required)
		}
	}
	for _, entry := range evidence.Platforms {
		if _, required := requiredPlatformSet()[entry.Platform]; !required {
			return fmt.Errorf("rendered-cross-platform PASS contains unsupported platform %q", entry.Platform)
		}
		if entry.Verdict != VerdictPass {
			return fmt.Errorf("rendered-cross-platform PASS contains non-PASS platform %q", entry.Platform)
		}
	}
	if len(evidence.Platforms) != len(requiredRenderedCrossPlatforms) {
		return errors.New("rendered-cross-platform PASS requires exactly the required desktop platforms")
	}
	return nil
}

func requiredPlatformSet() map[string]struct{} {
	out := make(map[string]struct{}, len(requiredRenderedCrossPlatforms))
	for _, platform := range requiredRenderedCrossPlatforms {
		out[platform] = struct{}{}
	}
	return out
}

func requireExactStringSet(got, want []string) error {
	if len(got) != len(want) {
		return errors.New("set length mismatch")
	}
	seen := make(map[string]struct{}, len(got))
	for _, value := range got {
		if _, ok := seen[value]; ok {
			return fmt.Errorf("duplicate value %q", value)
		}
		seen[value] = struct{}{}
	}
	for _, value := range want {
		if _, ok := seen[value]; !ok {
			return fmt.Errorf("missing value %q", value)
		}
	}
	return nil
}
