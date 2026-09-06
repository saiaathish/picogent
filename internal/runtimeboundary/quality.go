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
	LiveProviderQualityEvidenceSchema   = "picogent.v4.live-provider-quality-evidence.v1"
	MaxLiveProviderQualityEvidenceBytes = 48 << 10
	LiveProviderQualityEvidenceEnv      = "PICOGENT_LIVE_PROVIDER_QUALITY_EVIDENCE"
	LiveProviderQualityArtifactEnv      = "PICOGENT_LIVE_PROVIDER_QUALITY_ARTIFACT"
	liveProviderQualityCampaign         = "fixed-no-tool-v1"
	maxLiveProviderQualityLatencyMS     = int64(120000)
)

var fixedLiveProviderQualityCaseIDs = []string{
	"exact-token",
	"bounded-summary",
	"constraint-following",
}

// LiveProviderQualityEvidence is a secret-free record of a bounded quality
// campaign. Prompts and provider results are represented only by digests.
type LiveProviderQualityEvidence struct {
	Schema          string                            `json:"schema"`
	Campaign        string                            `json:"campaign"`
	CandidateSHA    string                            `json:"candidate_sha"`
	Provider        string                            `json:"provider"`
	Environment     string                            `json:"environment"`
	LatencyBudgetMS int64                             `json:"latency_budget_ms"`
	LatencyMaxMS    int64                             `json:"latency_max_ms"`
	ObservedAt      string                            `json:"observed_at"`
	Verdict         Verdict                           `json:"verdict"`
	Cases           []LiveProviderQualityCaseEvidence `json:"cases"`
}

// LiveProviderQualityCaseEvidence is one fixed no-tool challenge observation.
type LiveProviderQualityCaseEvidence struct {
	ID               string  `json:"id"`
	PromptSHA256     string  `json:"prompt_sha256"`
	ResultSHA256     string  `json:"result_sha256"`
	LatencyMS        int64   `json:"latency_ms"`
	Verdict          Verdict `json:"verdict"`
	ToolsUsed        bool    `json:"tools_used"`
	MutationObserved bool    `json:"mutation_observed"`
}

func loadLiveProviderQualityEvidence(workspace, artifactPath, expectedSHA string) (LiveProviderQualityEvidence, string, error) {
	var evidence LiveProviderQualityEvidence
	if err := ValidateArtifactPath(workspace, artifactPath); err != nil {
		return evidence, "", fmt.Errorf("live-provider quality evidence path: %w", err)
	}
	data, err := securefile.ReadFileLimited(artifactPath, MaxLiveProviderQualityEvidenceBytes)
	if err != nil {
		if errors.Is(err, securefile.ErrReadLimit) {
			return evidence, "", errors.New("live-provider quality evidence exceeds size limit")
		}
		return evidence, "", fmt.Errorf("read live-provider quality evidence: %w", err)
	}
	if len(data) == 0 {
		return evidence, "", errors.New("live-provider quality evidence is empty")
	}

	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&evidence); err != nil {
		return LiveProviderQualityEvidence{}, "", fmt.Errorf("decode live-provider quality evidence: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return LiveProviderQualityEvidence{}, "", errors.New("live-provider quality evidence has trailing JSON")
	}
	if err := requireLiveProviderQualityAssertions(data); err != nil {
		return LiveProviderQualityEvidence{}, "", err
	}
	if err := validateLiveProviderQualityEvidence(evidence, expectedSHA); err != nil {
		return LiveProviderQualityEvidence{}, "", err
	}
	digest := sha256.Sum256(data)
	return evidence, hex.EncodeToString(digest[:]), nil
}

func requireLiveProviderQualityAssertions(data []byte) error {
	var assertions struct {
		Cases []struct {
			ToolsUsed        *bool `json:"tools_used"`
			MutationObserved *bool `json:"mutation_observed"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &assertions); err != nil {
		return fmt.Errorf("decode live-provider quality assertions: %w", err)
	}
	if assertions.Cases == nil {
		return errors.New("live-provider quality evidence must explicitly include cases")
	}
	for i, observed := range assertions.Cases {
		if observed.ToolsUsed == nil || observed.MutationObserved == nil {
			return fmt.Errorf("live-provider quality case %d must explicitly assert tools_used and mutation_observed", i)
		}
	}
	return nil
}

func validateLiveProviderQualityEvidence(evidence LiveProviderQualityEvidence, expectedSHA string) error {
	if evidence.Schema != LiveProviderQualityEvidenceSchema {
		return fmt.Errorf("live-provider quality evidence schema %q does not match %q", evidence.Schema, LiveProviderQualityEvidenceSchema)
	}
	if evidence.Campaign != liveProviderQualityCampaign {
		return fmt.Errorf("live-provider quality evidence campaign %q does not match %q", evidence.Campaign, liveProviderQualityCampaign)
	}
	if !validCommitSHA(evidence.CandidateSHA) || evidence.CandidateSHA != strings.TrimSpace(expectedSHA) {
		return errors.New("live-provider quality evidence candidate_sha does not match expected SHA")
	}
	if !validProviderName(evidence.Provider) {
		return errors.New("live-provider quality evidence provider is invalid")
	}
	if evidence.Environment != "task-owned-disposable" {
		return errors.New("live-provider quality evidence environment is not task-owned-disposable")
	}
	if evidence.LatencyBudgetMS < 1 || evidence.LatencyBudgetMS > maxLiveProviderQualityLatencyMS {
		return errors.New("live-provider quality evidence latency budget is outside the bounded range")
	}
	if _, err := time.Parse(time.RFC3339, evidence.ObservedAt); err != nil {
		return fmt.Errorf("live-provider quality evidence observed_at is invalid: %w", err)
	}
	if !evidence.Verdict.valid() {
		return fmt.Errorf("live-provider quality evidence verdict is invalid: %q", evidence.Verdict)
	}

	expectedCases := make(map[string]struct{}, len(fixedLiveProviderQualityCaseIDs))
	for _, id := range fixedLiveProviderQualityCaseIDs {
		expectedCases[id] = struct{}{}
	}
	seen := make(map[string]struct{}, len(evidence.Cases))
	maxLatency := int64(0)
	for _, observed := range evidence.Cases {
		if _, ok := expectedCases[observed.ID]; !ok {
			return fmt.Errorf("live-provider quality evidence case %q is not part of the fixed campaign", observed.ID)
		}
		if _, duplicate := seen[observed.ID]; duplicate {
			return fmt.Errorf("live-provider quality evidence case %q is duplicated", observed.ID)
		}
		seen[observed.ID] = struct{}{}
		if !validDigest(observed.PromptSHA256) || !validDigest(observed.ResultSHA256) {
			return fmt.Errorf("live-provider quality evidence case %q has an invalid prompt/result digest", observed.ID)
		}
		if observed.LatencyMS < 1 || observed.LatencyMS > evidence.LatencyBudgetMS {
			return fmt.Errorf("live-provider quality evidence case %q latency is outside the budget", observed.ID)
		}
		if !observed.Verdict.valid() {
			return fmt.Errorf("live-provider quality evidence case %q verdict is invalid: %q", observed.ID, observed.Verdict)
		}
		if observed.LatencyMS > maxLatency {
			maxLatency = observed.LatencyMS
		}
	}
	if evidence.LatencyMaxMS != maxLatency {
		return fmt.Errorf("live-provider quality evidence latency_max_ms %d does not match observed maximum %d", evidence.LatencyMaxMS, maxLatency)
	}

	if evidence.Verdict != VerdictPass {
		return nil
	}
	if len(evidence.Cases) != len(fixedLiveProviderQualityCaseIDs) {
		return errors.New("live-provider quality PASS requires complete fixed-case coverage")
	}
	for _, id := range fixedLiveProviderQualityCaseIDs {
		if _, ok := seen[id]; !ok {
			return fmt.Errorf("live-provider quality PASS is missing fixed case %q", id)
		}
	}
	for _, observed := range evidence.Cases {
		if observed.Verdict != VerdictPass {
			return fmt.Errorf("live-provider quality PASS contains non-PASS case %q", observed.ID)
		}
		if observed.ToolsUsed || observed.MutationObserved {
			return fmt.Errorf("live-provider quality PASS contains tool or mutation activity in case %q", observed.ID)
		}
	}
	return nil
}
