package runtimeboundary

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectRecordsConnectivityWithoutClaimingQuality(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("test requires a git checkout")
	}
	workspace := t.TempDir()
	seedDocs(t, workspace)
	sha := commitAll(t, workspace)
	artifact := filepath.Join(t.TempDir(), "live-provider.json")
	writeLiveEvidence(t, artifact, validLiveEvidence(sha))

	report, err := Collect(Options{
		Workspace:    workspace,
		CandidateSHA: sha,
		Environ: func(key string) string {
			switch key {
			case LiveEvidenceEnv:
				return "1"
			case LiveArtifactEnv:
				return artifact
			default:
				return ""
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	connectivity := claimByID(t, report, "live-provider-connectivity")
	if connectivity.Verdict != VerdictPass {
		t.Fatalf("connectivity verdict = %s reason=%s", connectivity.Verdict, connectivity.Reason)
	}
	if connectivity.ObservedAt != "2026-09-06T12:00:00Z" {
		t.Fatalf("connectivity observed_at = %q", connectivity.ObservedAt)
	}
	if !strings.Contains(connectivity.Provenance, "sha256:") {
		t.Fatalf("connectivity provenance = %q", connectivity.Provenance)
	}

	quality := claimByID(t, report, "live-provider-quality")
	if quality.Verdict != VerdictUnverified {
		t.Fatalf("quality verdict = %s reason=%s", quality.Verdict, quality.Reason)
	}
	if !strings.Contains(quality.Reason, "no live-provider quality evidence artifact") {
		t.Fatalf("quality reason = %q", quality.Reason)
	}
}

func TestCollectFailsClosedForMissingLiveArtifact(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("test requires a git checkout")
	}
	workspace := t.TempDir()
	seedDocs(t, workspace)
	sha := commitAll(t, workspace)
	artifact := filepath.Join(t.TempDir(), "missing.json")

	report, err := Collect(Options{
		Workspace:    workspace,
		CandidateSHA: sha,
		Environ: func(key string) string {
			switch key {
			case LiveEvidenceEnv:
				return "1"
			case LiveArtifactEnv:
				return artifact
			default:
				return ""
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	connectivity := claimByID(t, report, "live-provider-connectivity")
	if connectivity.Verdict != VerdictFail || !strings.Contains(connectivity.Reason, "missing") {
		t.Fatalf("connectivity claim = %+v", connectivity)
	}
	quality := claimByID(t, report, "live-provider-quality")
	if quality.Verdict != VerdictUnverified {
		t.Fatalf("quality verdict = %s reason=%s", quality.Verdict, quality.Reason)
	}
}

func TestCollectProjectsQualitySeparatelyFromConnectivity(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("test requires a git checkout")
	}
	workspace := t.TempDir()
	seedDocs(t, workspace)
	sha := commitAll(t, workspace)
	connectivityArtifact := filepath.Join(t.TempDir(), "live-provider.json")
	qualityArtifact := filepath.Join(t.TempDir(), "live-provider-quality.json")
	writeLiveEvidence(t, connectivityArtifact, validLiveEvidence(sha))
	writeLiveProviderQualityEvidence(t, qualityArtifact, validLiveProviderQualityEvidence(sha))

	report, err := Collect(Options{
		Workspace:    workspace,
		CandidateSHA: sha,
		Environ: func(key string) string {
			switch key {
			case LiveEvidenceEnv, LiveQualityEvidenceEnv:
				return "1"
			case LiveArtifactEnv:
				return connectivityArtifact
			case LiveQualityArtifactEnv:
				return qualityArtifact
			default:
				return ""
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	connectivity := claimByID(t, report, "live-provider-connectivity")
	if connectivity.Verdict != VerdictPass {
		t.Fatalf("connectivity claim = %+v", connectivity)
	}
	quality := claimByID(t, report, "live-provider-quality")
	if quality.Verdict != VerdictPass {
		t.Fatalf("quality claim = %+v", quality)
	}
	if !strings.Contains(quality.Provenance, LiveQualityEvidenceEnv) {
		t.Fatalf("quality provenance = %q", quality.Provenance)
	}
}

func TestCollectFailsClosedForMissingLiveQualityArtifact(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("test requires a git checkout")
	}
	workspace := t.TempDir()
	seedDocs(t, workspace)
	sha := commitAll(t, workspace)
	artifact := filepath.Join(t.TempDir(), "missing-quality.json")

	report, err := Collect(Options{
		Workspace:    workspace,
		CandidateSHA: sha,
		Environ: func(key string) string {
			switch key {
			case LiveQualityEvidenceEnv:
				return "1"
			case LiveQualityArtifactEnv:
				return artifact
			default:
				return ""
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	quality := claimByID(t, report, "live-provider-quality")
	if quality.Verdict != VerdictFail || !strings.Contains(quality.Reason, "missing") {
		t.Fatalf("quality claim = %+v", quality)
	}
	connectivity := claimByID(t, report, "live-provider-connectivity")
	if connectivity.Verdict != VerdictUnverified {
		t.Fatalf("connectivity claim = %+v", connectivity)
	}
}

func TestLoadLiveProviderEvidenceRejectsMalformedTrailingSHAAndSecretFields(t *testing.T) {
	workspace := t.TempDir()
	sha := strings.Repeat("a", 40)
	valid := validLiveEvidence(sha)
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
	missingAssertions := map[string]any{}
	if err := json.Unmarshal(validJSON, &missingAssertions); err != nil {
		t.Fatal(err)
	}
	delete(missingAssertions, "tools_used")
	missingAssertionsJSON, err := json.Marshal(missingAssertions)
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
		{name: "sha-mismatch", data: mustJSON(t, validLiveEvidence(strings.Repeat("b", 40))), want: "candidate_sha"},
		{name: "secret-shaped-unknown-field", data: secretJSON, want: "unknown field"},
		{name: "missing-assertion", data: missingAssertionsJSON, want: "explicitly assert"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			artifact := filepath.Join(t.TempDir(), "live-provider.json")
			if err := os.WriteFile(artifact, tc.data, 0o600); err != nil {
				t.Fatal(err)
			}
			_, _, err := loadLiveProviderEvidence(workspace, artifact, sha)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
			if strings.Contains(err.Error(), "secret-value") {
				t.Fatalf("error leaked secret-shaped field value: %v", err)
			}
		})
	}
}

func TestLoadLiveProviderEvidenceRejectsWorkspaceAndResolvedSymlinkPaths(t *testing.T) {
	workspace := t.TempDir()
	sha := strings.Repeat("c", 40)
	inside := filepath.Join(workspace, "live-provider.json")
	writeLiveEvidence(t, inside, validLiveEvidence(sha))

	linkedTarget := filepath.Join(workspace, "evidence")
	if err := os.Mkdir(linkedTarget, 0o755); err != nil {
		t.Fatal(err)
	}
	linkedParent := filepath.Join(t.TempDir(), "workspace-link")
	if err := os.Symlink(linkedTarget, linkedParent); err != nil {
		t.Fatal(err)
	}
	throughLink := filepath.Join(linkedParent, "live-provider.json")

	for _, artifact := range []string{inside, throughLink} {
		if _, _, err := loadLiveProviderEvidence(workspace, artifact, sha); err == nil || !strings.Contains(err.Error(), "inside workspace") {
			t.Fatalf("artifact %q error = %v, want workspace containment failure", artifact, err)
		}
	}
}

func validLiveEvidence(sha string) LiveProviderEvidence {
	return LiveProviderEvidence{
		Schema:           LiveEvidenceSchema,
		CandidateSHA:     sha,
		Provider:         "codex",
		Environment:      "task-owned-disposable",
		PromptSHA256:     strings.Repeat("1", 64),
		ResultSHA256:     strings.Repeat("2", 64),
		ObservedAt:       "2026-09-06T12:00:00Z",
		Status:           string(VerdictPass),
		ToolsUsed:        false,
		MutationObserved: false,
	}
}

func writeLiveEvidence(t *testing.T, path string, evidence LiveProviderEvidence) {
	t.Helper()
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
