package runtimeboundary

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCollectKeepsCrossPlatformUnverifiedWithoutArtifact(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("test requires git")
	}
	workspace := t.TempDir()
	seedDocs(t, workspace)
	sha := commitAll(t, workspace)
	localArtifact := filepath.Join(t.TempDir(), "rendered-platform.json")
	writeRenderedEvidence(t, localArtifact, validRenderedEvidence(sha))

	report, err := Collect(Options{
		Workspace:    workspace,
		CandidateSHA: sha,
		Environ: func(key string) string {
			switch key {
			case RenderedEvidenceEnv:
				return "1"
			case RenderedArtifactEnv:
				return localArtifact
			default:
				return ""
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	cross := claimByID(t, report, "rendered-cross-platform")
	if cross.Verdict != VerdictUnverified {
		t.Fatalf("cross-platform verdict = %s reason=%s", cross.Verdict, cross.Reason)
	}
}

func TestCollectPassesCrossPlatformWhenAllRequiredPlatformsPass(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("test requires git")
	}
	workspace := t.TempDir()
	seedDocs(t, workspace)
	sha := commitAll(t, workspace)
	artifact := filepath.Join(t.TempDir(), "rendered-cross-platform.json")
	writeRenderedCrossEvidence(t, artifact, validRenderedCrossEvidence(sha, VerdictPass))

	report, err := Collect(Options{
		Workspace:    workspace,
		CandidateSHA: sha,
		Environ: func(key string) string {
			switch key {
			case RenderedCrossEvidenceEnv:
				return "1"
			case RenderedCrossArtifactEnv:
				return artifact
			default:
				return ""
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	cross := claimByID(t, report, "rendered-cross-platform")
	if cross.Verdict != VerdictPass {
		t.Fatalf("cross-platform verdict = %s reason=%s", cross.Verdict, cross.Reason)
	}
}

func TestCollectFailsCrossPlatformFlagWithoutArtifact(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("test requires git")
	}
	workspace := t.TempDir()
	seedDocs(t, workspace)
	sha := commitAll(t, workspace)
	report, err := Collect(Options{
		Workspace:    workspace,
		CandidateSHA: sha,
		Environ: func(key string) string {
			if key == RenderedCrossEvidenceEnv {
				return "1"
			}
			return ""
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	cross := claimByID(t, report, "rendered-cross-platform")
	if cross.Verdict != VerdictFail || !strings.Contains(cross.Reason, RenderedCrossArtifactEnv) {
		t.Fatalf("cross-platform claim = %+v", cross)
	}
}

func TestLoadRenderedCrossPlatformEvidenceRejectsIncompleteAndMalformed(t *testing.T) {
	workspace := t.TempDir()
	sha := strings.Repeat("e", 40)
	valid := validRenderedCrossEvidence(sha, VerdictPass)
	validJSON := mustRenderedCrossJSON(t, valid)

	missingLinux := validRenderedCrossEvidence(sha, VerdictPass)
	missingLinux.Platforms = missingLinux.Platforms[:2] // darwin+linux only if order is darwin,linux,windows
	// rebuild with only darwin+windows
	missingLinux.Platforms = []RenderedCrossPlatformEntryEvidence{
		valid.Platforms[0],
		valid.Platforms[2],
	}
	missingLinux.Verdict = VerdictPass

	incompleteOK := validRenderedCrossEvidence(sha, VerdictUnverified)
	incompleteOK.Platforms = []RenderedCrossPlatformEntryEvidence{valid.Platforms[0]}

	var unknown map[string]any
	if err := json.Unmarshal(validJSON, &unknown); err != nil {
		t.Fatal(err)
	}
	unknown["browser_log"] = "secret-value"
	unknownJSON, err := json.Marshal(unknown)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		data []byte
		want string
	}{
		{name: "malformed", data: []byte("{"), want: "decode"},
		{name: "trailing", data: append(append([]byte{}, validJSON...), []byte("\n{}\n")...), want: "trailing JSON"},
		{name: "sha-mismatch", data: mustRenderedCrossJSON(t, validRenderedCrossEvidence(strings.Repeat("f", 40), VerdictPass)), want: "candidate_sha"},
		{name: "unknown-field", data: unknownJSON, want: "unknown field"},
		{name: "pass-missing-platform", data: mustRenderedCrossJSON(t, missingLinux), want: "missing required platform"},
		{name: "workspace-path", data: nil, want: "inside workspace"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "workspace-path" {
				inside := filepath.Join(workspace, "cross.json")
				writeRenderedCrossEvidence(t, inside, valid)
				_, _, err := loadRenderedCrossPlatformEvidence(workspace, inside, sha)
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("error = %v, want %q", err, tc.want)
				}
				return
			}
			artifact := filepath.Join(t.TempDir(), "cross.json")
			if err := os.WriteFile(artifact, tc.data, 0o600); err != nil {
				t.Fatal(err)
			}
			_, _, err := loadRenderedCrossPlatformEvidence(workspace, artifact, sha)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
			if strings.Contains(err.Error(), "secret-value") {
				t.Fatalf("error leaked raw evidence: %v", err)
			}
		})
	}

	// Incomplete non-PASS records remain loadable.
	artifact := filepath.Join(t.TempDir(), "cross-incomplete.json")
	writeRenderedCrossEvidence(t, artifact, incompleteOK)
	got, _, err := loadRenderedCrossPlatformEvidence(workspace, artifact, sha)
	if err != nil {
		t.Fatal(err)
	}
	if got.Verdict != VerdictUnverified {
		t.Fatalf("incomplete verdict = %s", got.Verdict)
	}
}

func validRenderedCrossEvidence(sha string, verdict Verdict) RenderedCrossPlatformEvidence {
	entry := func(platform, arch string) RenderedCrossPlatformEntryEvidence {
		return RenderedCrossPlatformEntryEvidence{
			Platform:          platform,
			Architecture:      arch,
			Browser:           "browseros-neo",
			Fixture:           "rendered-recovery",
			ObservationSHA256: strings.Repeat("2", 64),
			ScreenshotSHA256:  "UNRECORDED",
			ObservedAt:        "2026-09-06T12:00:00Z",
			Verdict:           VerdictPass,
		}
	}
	platforms := []RenderedCrossPlatformEntryEvidence{
		entry("darwin", "arm64"),
		entry("linux", "amd64"),
		entry("windows", "amd64"),
	}
	if verdict != VerdictPass {
		platforms = platforms[:1]
		platforms[0].Verdict = verdict
	}
	return RenderedCrossPlatformEvidence{
		Schema:             RenderedCrossEvidenceSchema,
		CandidateSHA:       sha,
		Environment:        "task-owned-disposable",
		RequiredPlatforms:  append([]string{}, requiredRenderedCrossPlatforms...),
		Platforms:          platforms,
		ObservedAt:         "2026-09-06T12:00:00Z",
		Verdict:            verdict,
		SourceTreeModified: false,
	}
}

func writeRenderedCrossEvidence(t *testing.T, path string, evidence RenderedCrossPlatformEvidence) {
	t.Helper()
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustRenderedCrossJSON(t *testing.T, value RenderedCrossPlatformEvidence) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
