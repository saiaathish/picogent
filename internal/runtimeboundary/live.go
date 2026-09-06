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
	LiveEvidenceSchema   = "picogent.v4.live-provider-evidence.v1"
	MaxLiveEvidenceBytes = 16 << 10
)

// LiveProviderEvidence is a secret-free record of one task-owned provider
// connectivity observation. It records digests, not the prompt or response.
// The record is deliberately narrower than live-provider quality evidence.
type LiveProviderEvidence struct {
	Schema           string `json:"schema"`
	CandidateSHA     string `json:"candidate_sha"`
	Provider         string `json:"provider"`
	Environment      string `json:"environment"`
	PromptSHA256     string `json:"prompt_sha256"`
	ResultSHA256     string `json:"result_sha256"`
	ObservedAt       string `json:"observed_at"`
	Status           string `json:"status"`
	ToolsUsed        bool   `json:"tools_used"`
	MutationObserved bool   `json:"mutation_observed"`
}

func loadLiveProviderEvidence(workspace, artifactPath, expectedSHA string) (LiveProviderEvidence, string, error) {
	var evidence LiveProviderEvidence
	if err := ValidateArtifactPath(workspace, artifactPath); err != nil {
		return evidence, "", fmt.Errorf("live-provider evidence path: %w", err)
	}
	data, err := securefile.ReadFileLimited(artifactPath, MaxLiveEvidenceBytes)
	if err != nil {
		if errors.Is(err, securefile.ErrReadLimit) {
			return evidence, "", errors.New("live-provider evidence exceeds size limit")
		}
		return evidence, "", fmt.Errorf("read live-provider evidence: %w", err)
	}
	if len(data) == 0 {
		return evidence, "", errors.New("live-provider evidence is empty")
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&evidence); err != nil {
		return LiveProviderEvidence{}, "", fmt.Errorf("decode live-provider evidence: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return LiveProviderEvidence{}, "", errors.New("live-provider evidence has trailing JSON")
	}
	if err := requireLiveProviderAssertions(data); err != nil {
		return LiveProviderEvidence{}, "", err
	}
	if err := validateLiveProviderEvidence(evidence, expectedSHA); err != nil {
		return LiveProviderEvidence{}, "", err
	}
	digest := sha256.Sum256(data)
	return evidence, hex.EncodeToString(digest[:]), nil
}

func requireLiveProviderAssertions(data []byte) error {
	var assertions struct {
		ToolsUsed        *bool `json:"tools_used"`
		MutationObserved *bool `json:"mutation_observed"`
	}
	if err := json.Unmarshal(data, &assertions); err != nil {
		return fmt.Errorf("decode live-provider assertions: %w", err)
	}
	if assertions.ToolsUsed == nil || assertions.MutationObserved == nil {
		return errors.New("live-provider evidence must explicitly assert tools_used and mutation_observed")
	}
	return nil
}

func validateLiveProviderEvidence(evidence LiveProviderEvidence, expectedSHA string) error {
	if evidence.Schema != LiveEvidenceSchema {
		return fmt.Errorf("live-provider evidence schema %q does not match %q", evidence.Schema, LiveEvidenceSchema)
	}
	if !validCommitSHA(evidence.CandidateSHA) || evidence.CandidateSHA != strings.TrimSpace(expectedSHA) {
		return errors.New("live-provider evidence candidate_sha does not match expected SHA")
	}
	if !validProviderName(evidence.Provider) {
		return errors.New("live-provider evidence provider is invalid")
	}
	if evidence.Environment != "task-owned-disposable" {
		return errors.New("live-provider evidence environment is not task-owned-disposable")
	}
	if !validDigest(evidence.PromptSHA256) || !validDigest(evidence.ResultSHA256) {
		return errors.New("live-provider evidence prompt/result digest is invalid")
	}
	if _, err := time.Parse(time.RFC3339, evidence.ObservedAt); err != nil {
		return fmt.Errorf("live-provider evidence observed_at is invalid: %w", err)
	}
	if evidence.Status != string(VerdictPass) {
		return fmt.Errorf("live-provider evidence status must be PASS, got %q", evidence.Status)
	}
	if evidence.ToolsUsed || evidence.MutationObserved {
		return errors.New("live-provider connectivity evidence must not use tools or observe mutations")
	}
	return nil
}

func validProviderName(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for i, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		if i == 0 || r == '/' || r == ':' || r == ' ' {
			return false
		}
		return false
	}
	return true
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
