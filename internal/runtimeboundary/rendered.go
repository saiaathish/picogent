package runtimeboundary

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/securefile"
)

const (
	RenderedEvidenceSchema   = "picogent.v4.rendered-platform-evidence.v1"
	MaxRenderedEvidenceBytes = 24 << 10
	RenderedEvidenceEnv      = "PICOGENT_RENDERED_PLATFORM_EVIDENCE"
	RenderedArtifactEnv      = "PICOGENT_RENDERED_PLATFORM_ARTIFACT"
)

// RenderedPlatformEvidence is a secret-free record of one task-owned rendered
// observation on the host that produced the matrix. It stores digests, not
// screenshots, DOM output, URLs, credentials, or browser transcripts.
type RenderedPlatformEvidence struct {
	Schema             string  `json:"schema"`
	CandidateSHA       string  `json:"candidate_sha"`
	Platform           string  `json:"platform"`
	Architecture       string  `json:"architecture"`
	Environment        string  `json:"environment"`
	Browser            string  `json:"browser"`
	Fixture            string  `json:"fixture"`
	ObservationSHA256  string  `json:"observation_sha256"`
	ScreenshotSHA256   string  `json:"screenshot_sha256"`
	ObservedAt         string  `json:"observed_at"`
	Verdict            Verdict `json:"verdict"`
	SourceTreeModified bool    `json:"source_tree_modified"`
}

func loadRenderedPlatformEvidence(workspace, artifactPath, expectedSHA, expectedPlatform, expectedArchitecture string) (RenderedPlatformEvidence, string, error) {
	var evidence RenderedPlatformEvidence
	if err := ValidateArtifactPath(workspace, artifactPath); err != nil {
		return evidence, "", fmt.Errorf("rendered-platform evidence path: %w", err)
	}
	data, err := securefile.ReadFileLimited(artifactPath, MaxRenderedEvidenceBytes)
	if err != nil {
		if errors.Is(err, securefile.ErrReadLimit) {
			return evidence, "", errors.New("rendered-platform evidence exceeds size limit")
		}
		return evidence, "", fmt.Errorf("read rendered-platform evidence: %w", err)
	}
	if len(data) == 0 {
		return evidence, "", errors.New("rendered-platform evidence is empty")
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&evidence); err != nil {
		return RenderedPlatformEvidence{}, "", fmt.Errorf("decode rendered-platform evidence: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return RenderedPlatformEvidence{}, "", errors.New("rendered-platform evidence has trailing JSON")
	}
	if err := requireRenderedPlatformAssertions(data); err != nil {
		return RenderedPlatformEvidence{}, "", err
	}
	if err := validateRenderedPlatformEvidence(evidence, expectedSHA, expectedPlatform, expectedArchitecture); err != nil {
		return RenderedPlatformEvidence{}, "", err
	}
	digest := sha256.Sum256(data)
	return evidence, hex.EncodeToString(digest[:]), nil
}

func requireRenderedPlatformAssertions(data []byte) error {
	var assertions struct {
		SourceTreeModified *bool `json:"source_tree_modified"`
	}
	if err := json.Unmarshal(data, &assertions); err != nil {
		return fmt.Errorf("decode rendered-platform assertions: %w", err)
	}
	if assertions.SourceTreeModified == nil {
		return errors.New("rendered-platform evidence must explicitly assert source_tree_modified")
	}
	return nil
}

func validateRenderedPlatformEvidence(evidence RenderedPlatformEvidence, expectedSHA, expectedPlatform, expectedArchitecture string) error {
	if evidence.Schema != RenderedEvidenceSchema {
		return fmt.Errorf("rendered-platform evidence schema %q does not match %q", evidence.Schema, RenderedEvidenceSchema)
	}
	if !validCommitSHA(evidence.CandidateSHA) || evidence.CandidateSHA != strings.TrimSpace(expectedSHA) {
		return errors.New("rendered-platform evidence candidate_sha does not match expected SHA")
	}
	if !validRenderedPlatform(evidence.Platform) || evidence.Platform != expectedPlatform {
		return errors.New("rendered-platform evidence platform does not match the observed host")
	}
	if !validEvidenceIdentifier(evidence.Architecture) || evidence.Architecture != expectedArchitecture {
		return errors.New("rendered-platform evidence architecture does not match the observed host")
	}
	if evidence.Environment != "task-owned-disposable" {
		return errors.New("rendered-platform evidence environment is not task-owned-disposable")
	}
	if !validEvidenceIdentifier(evidence.Browser) || !validEvidenceIdentifier(evidence.Fixture) {
		return errors.New("rendered-platform evidence browser or fixture is invalid")
	}
	if !validDigest(evidence.ObservationSHA256) {
		return errors.New("rendered-platform evidence observation digest is invalid")
	}
	if evidence.ScreenshotSHA256 != "UNRECORDED" && !validDigest(evidence.ScreenshotSHA256) {
		return errors.New("rendered-platform evidence screenshot digest is invalid")
	}
	if _, err := time.Parse(time.RFC3339, evidence.ObservedAt); err != nil {
		return fmt.Errorf("rendered-platform evidence observed_at is invalid: %w", err)
	}
	if !evidence.Verdict.valid() {
		return fmt.Errorf("rendered-platform evidence verdict is invalid: %q", evidence.Verdict)
	}
	if evidence.SourceTreeModified {
		return errors.New("rendered-platform evidence source tree must be clean")
	}
	return nil
}

func validRenderedPlatform(value string) bool {
	switch value {
	case "darwin", "linux", "windows":
		return true
	default:
		return false
	}
}

func validEvidenceIdentifier(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for i, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			if i == 0 && r == '.' {
				return false
			}
			continue
		}
		return false
	}
	return true
}

func currentRenderedPlatform() (string, string) {
	return runtime.GOOS, runtime.GOARCH
}
