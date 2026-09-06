package runtimeboundary

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadLiveProviderQualityEvidenceAcceptsCompletePass(t *testing.T) {
	workspace := t.TempDir()
	sha := strings.Repeat("a", 40)
	artifact := filepath.Join(t.TempDir(), "live-provider-quality.json")
	writeLiveProviderQualityEvidence(t, artifact, validLiveProviderQualityEvidence(sha))

	evidence, digest, err := loadLiveProviderQualityEvidence(workspace, artifact, sha)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.Verdict != VerdictPass || len(evidence.Cases) != len(fixedLiveProviderQualityCaseIDs) {
		t.Fatalf("evidence = %+v", evidence)
	}
	if len(digest) != 64 {
		t.Fatalf("artifact digest length = %d", len(digest))
	}
}

func TestLoadLiveProviderQualityEvidencePreservesIncompleteVerdict(t *testing.T) {
	workspace := t.TempDir()
	sha := strings.Repeat("b", 40)
	evidence := validLiveProviderQualityEvidence(sha)
	evidence.Verdict = VerdictInconclusive
	evidence.Cases = []LiveProviderQualityCaseEvidence{evidence.Cases[0]}
	evidence.LatencyMaxMS = evidence.Cases[0].LatencyMS
	artifact := filepath.Join(t.TempDir(), "live-provider-quality.json")
	writeLiveProviderQualityEvidence(t, artifact, evidence)

	loaded, _, err := loadLiveProviderQualityEvidence(workspace, artifact, sha)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Verdict != VerdictInconclusive || len(loaded.Cases) != 1 {
		t.Fatalf("loaded evidence = %+v", loaded)
	}
}

func TestLoadLiveProviderQualityEvidenceRejectsAdversarialArtifacts(t *testing.T) {
	workspace := t.TempDir()
	sha := strings.Repeat("c", 40)
	valid := validLiveProviderQualityEvidence(sha)
	validJSON, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}

	var secretFields map[string]any
	if err := json.Unmarshal(validJSON, &secretFields); err != nil {
		t.Fatal(err)
	}
	secretFields["api_key"] = "secret-value"
	secretJSON, err := json.Marshal(secretFields)
	if err != nil {
		t.Fatal(err)
	}

	var missingAssertion map[string]any
	if err := json.Unmarshal(validJSON, &missingAssertion); err != nil {
		t.Fatal(err)
	}
	missingCases := missingAssertion["cases"].([]any)
	delete(missingCases[0].(map[string]any), "tools_used")
	missingAssertionJSON, err := json.Marshal(missingAssertion)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		data []byte
		want string
	}{
		{name: "malformed", data: []byte("{"), want: "decode"},
		{name: "trailing-json", data: append(validJSON, []byte("\n{}\n")...), want: "trailing JSON"},
		{name: "stale-sha", data: mustQualityJSON(t, validLiveProviderQualityEvidence(strings.Repeat("d", 40))), want: "candidate_sha"},
		{name: "secret-shaped-unknown-field", data: secretJSON, want: "unknown field"},
		{name: "missing-assertion", data: missingAssertionJSON, want: "explicitly assert"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			artifact := filepath.Join(t.TempDir(), "live-provider-quality.json")
			if err := os.WriteFile(artifact, tc.data, 0o600); err != nil {
				t.Fatal(err)
			}
			_, _, err := loadLiveProviderQualityEvidence(workspace, artifact, sha)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
			if strings.Contains(err.Error(), "secret-value") {
				t.Fatalf("error leaked secret-shaped field value: %v", err)
			}
		})
	}
}

func TestLoadLiveProviderQualityEvidenceRejectsOversizedAndContainedArtifacts(t *testing.T) {
	workspace := t.TempDir()
	sha := strings.Repeat("e", 40)
	inside := filepath.Join(workspace, "live-provider-quality.json")
	writeLiveProviderQualityEvidence(t, inside, validLiveProviderQualityEvidence(sha))

	tooLarge := filepath.Join(t.TempDir(), "oversized.json")
	if err := os.WriteFile(tooLarge, append([]byte("{"), make([]byte, MaxLiveProviderQualityEvidenceBytes+1)...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadLiveProviderQualityEvidence(workspace, tooLarge, sha); err == nil || !strings.Contains(err.Error(), "exceeds size limit") {
		t.Fatalf("oversized artifact error = %v", err)
	}
	if _, _, err := loadLiveProviderQualityEvidence(workspace, inside, sha); err == nil || !strings.Contains(err.Error(), "inside workspace") {
		t.Fatalf("contained artifact error = %v", err)
	}
}

func TestLoadLiveProviderQualityEvidenceRejectsIncompletePass(t *testing.T) {
	workspace := t.TempDir()
	sha := strings.Repeat("f", 40)
	evidence := validLiveProviderQualityEvidence(sha)
	evidence.Cases = evidence.Cases[:1]
	evidence.LatencyMaxMS = evidence.Cases[0].LatencyMS
	artifact := filepath.Join(t.TempDir(), "live-provider-quality.json")
	writeLiveProviderQualityEvidence(t, artifact, evidence)

	if _, _, err := loadLiveProviderQualityEvidence(workspace, artifact, sha); err == nil || !strings.Contains(err.Error(), "complete fixed-case coverage") {
		t.Fatalf("incomplete PASS error = %v", err)
	}
}

func validLiveProviderQualityEvidence(sha string) LiveProviderQualityEvidence {
	return LiveProviderQualityEvidence{
		Schema:          LiveProviderQualityEvidenceSchema,
		Campaign:        liveProviderQualityCampaign,
		CandidateSHA:    sha,
		Provider:        "codex",
		Environment:     "task-owned-disposable",
		LatencyBudgetMS: 5000,
		LatencyMaxMS:    300,
		ObservedAt:      "2026-09-06T12:00:00Z",
		Verdict:         VerdictPass,
		Cases: []LiveProviderQualityCaseEvidence{
			{ID: "exact-token", PromptSHA256: strings.Repeat("1", 64), ResultSHA256: strings.Repeat("2", 64), LatencyMS: 100, Verdict: VerdictPass, ToolsUsed: false, MutationObserved: false},
			{ID: "bounded-summary", PromptSHA256: strings.Repeat("3", 64), ResultSHA256: strings.Repeat("4", 64), LatencyMS: 300, Verdict: VerdictPass, ToolsUsed: false, MutationObserved: false},
			{ID: "constraint-following", PromptSHA256: strings.Repeat("5", 64), ResultSHA256: strings.Repeat("6", 64), LatencyMS: 200, Verdict: VerdictPass, ToolsUsed: false, MutationObserved: false},
		},
	}
}

func writeLiveProviderQualityEvidence(t *testing.T, path string, evidence LiveProviderQualityEvidence) {
	t.Helper()
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustQualityJSON(t *testing.T, value LiveProviderQualityEvidence) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
