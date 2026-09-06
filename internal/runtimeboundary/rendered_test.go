package runtimeboundary

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCollectRecordsLocalRenderedEvidenceWithoutClaimingCrossPlatform(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("test requires git")
	}
	workspace := t.TempDir()
	seedDocs(t, workspace)
	sha := commitAll(t, workspace)
	artifact := filepath.Join(t.TempDir(), "rendered-platform.json")
	writeRenderedEvidence(t, artifact, validRenderedEvidence(sha))

	report, err := Collect(Options{
		Workspace:    workspace,
		CandidateSHA: sha,
		Environ: func(key string) string {
			switch key {
			case RenderedEvidenceEnv:
				return "1"
			case RenderedArtifactEnv:
				return artifact
			default:
				return ""
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	local := claimByID(t, report, "rendered-platform-local")
	if local.Verdict != VerdictPass {
		t.Fatalf("local rendered verdict = %s reason=%s", local.Verdict, local.Reason)
	}
	crossPlatform := claimByID(t, report, "rendered-cross-platform")
	if crossPlatform.Verdict != VerdictUnverified {
		t.Fatalf("cross-platform rendered verdict = %s reason=%s", crossPlatform.Verdict, crossPlatform.Reason)
	}
}

func TestCollectRejectsRenderedFlagWithoutArtifact(t *testing.T) {
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
			if key == RenderedEvidenceEnv {
				return "1"
			}
			return ""
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	local := claimByID(t, report, "rendered-platform-local")
	if local.Verdict != VerdictFail || !strings.Contains(local.Reason, RenderedArtifactEnv) {
		t.Fatalf("local rendered claim = %+v", local)
	}
}

func TestLoadRenderedPlatformEvidenceRejectsMalformedTrailingSHAUnknownAndDirtyRecords(t *testing.T) {
	workspace := t.TempDir()
	sha := strings.Repeat("a", 40)
	valid := validRenderedEvidence(sha)
	validJSON, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	var unknownFields map[string]any
	if err := json.Unmarshal(validJSON, &unknownFields); err != nil {
		t.Fatal(err)
	}
	unknownFields["dom_dump"] = "secret-value"
	unknownJSON, err := json.Marshal(unknownFields)
	if err != nil {
		t.Fatal(err)
	}
	var missingAssertion map[string]any
	if err := json.Unmarshal(validJSON, &missingAssertion); err != nil {
		t.Fatal(err)
	}
	delete(missingAssertion, "source_tree_modified")
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
		{name: "sha-mismatch", data: mustRenderedJSON(t, validRenderedEvidence(strings.Repeat("b", 40))), want: "candidate_sha"},
		{name: "unknown-field", data: unknownJSON, want: "unknown field"},
		{name: "missing-assertion", data: missingAssertionJSON, want: "explicitly assert"},
		{name: "dirty-source", data: mustRenderedJSON(t, func() RenderedPlatformEvidence {
			evidence := valid
			evidence.SourceTreeModified = true
			return evidence
		}()), want: "source tree must be clean"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			artifact := filepath.Join(t.TempDir(), "rendered-platform.json")
			if err := os.WriteFile(artifact, tc.data, 0o600); err != nil {
				t.Fatal(err)
			}
			_, _, err := loadRenderedPlatformEvidence(workspace, artifact, sha, runtime.GOOS, runtime.GOARCH)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
			if strings.Contains(err.Error(), "secret-value") {
				t.Fatalf("error leaked raw rendered evidence: %v", err)
			}
		})
	}
}

func TestLoadRenderedPlatformEvidenceRejectsWorkspaceAndResolvedSymlinkPaths(t *testing.T) {
	workspace := t.TempDir()
	sha := strings.Repeat("c", 40)
	inside := filepath.Join(workspace, "rendered-platform.json")
	writeRenderedEvidence(t, inside, validRenderedEvidence(sha))

	linkedTarget := filepath.Join(workspace, "rendered-evidence")
	if err := os.Mkdir(linkedTarget, 0o755); err != nil {
		t.Fatal(err)
	}
	linkedParent := filepath.Join(t.TempDir(), "workspace-link")
	if err := os.Symlink(linkedTarget, linkedParent); err != nil {
		t.Fatal(err)
	}
	throughLink := filepath.Join(linkedParent, "rendered-platform.json")

	for _, artifact := range []string{inside, throughLink} {
		if _, _, err := loadRenderedPlatformEvidence(workspace, artifact, sha, runtime.GOOS, runtime.GOARCH); err == nil || !strings.Contains(err.Error(), "inside workspace") {
			t.Fatalf("artifact %q error = %v, want workspace containment failure", artifact, err)
		}
	}
}

func TestLoadRenderedPlatformEvidenceRejectsWrongHostAndRawScreenshotPath(t *testing.T) {
	workspace := t.TempDir()
	sha := strings.Repeat("d", 40)
	wrongPlatform := "linux"
	if runtime.GOOS == wrongPlatform {
		wrongPlatform = "darwin"
	}
	wrongHost := validRenderedEvidence(sha)
	wrongHost.Platform = wrongPlatform
	wrongHostJSON := mustRenderedJSON(t, wrongHost)
	rawScreenshot := validRenderedEvidence(sha)
	rawScreenshot.ScreenshotSHA256 = "/tmp/rendered.png"
	rawScreenshotJSON := mustRenderedJSON(t, rawScreenshot)

	wrongArchitecture := "amd64"
	if runtime.GOARCH == wrongArchitecture {
		wrongArchitecture = "arm64"
	}
	wrongArch := validRenderedEvidence(sha)
	wrongArch.Architecture = wrongArchitecture
	wrongArchJSON := mustRenderedJSON(t, wrongArch)

	cases := []struct {
		name string
		data []byte
		want string
	}{
		{name: "wrong-platform", data: wrongHostJSON, want: "platform does not match"},
		{name: "wrong-architecture", data: wrongArchJSON, want: "architecture does not match"},
		{name: "raw-screenshot-path", data: rawScreenshotJSON, want: "screenshot digest"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			artifact := filepath.Join(t.TempDir(), "rendered-platform.json")
			if err := os.WriteFile(artifact, tc.data, 0o600); err != nil {
				t.Fatal(err)
			}
			_, _, err := loadRenderedPlatformEvidence(workspace, artifact, sha, runtime.GOOS, runtime.GOARCH)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func validRenderedEvidence(sha string) RenderedPlatformEvidence {
	return RenderedPlatformEvidence{
		Schema:             RenderedEvidenceSchema,
		CandidateSHA:       sha,
		Platform:           runtime.GOOS,
		Architecture:       runtime.GOARCH,
		Environment:        "task-owned-disposable",
		Browser:            "browseros-neo",
		Fixture:            "rendered-recovery",
		ObservationSHA256:  strings.Repeat("1", 64),
		ScreenshotSHA256:   "UNRECORDED",
		ObservedAt:         "2026-09-06T12:00:00Z",
		Verdict:            VerdictPass,
		SourceTreeModified: false,
	}
}

func writeRenderedEvidence(t *testing.T, path string, evidence RenderedPlatformEvidence) {
	t.Helper()
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustRenderedJSON(t *testing.T, value RenderedPlatformEvidence) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
