package runtimeboundary

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateArtifactPathRejectsInsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	inside := filepath.Join(workspace, "matrix.json")
	if err := ValidateArtifactPath(workspace, inside); err == nil || !strings.Contains(err.Error(), "inside workspace") {
		t.Fatalf("inside path error = %v", err)
	}
	outside := filepath.Join(t.TempDir(), "matrix.json")
	if err := ValidateArtifactPath(workspace, outside); err != nil {
		t.Fatal(err)
	}
}

func TestRetainAndLoadRoundTrip(t *testing.T) {
	workspace := t.TempDir()
	artifactDir := t.TempDir()
	artifact := filepath.Join(artifactDir, "runtime-boundary-matrix.json")
	sha := strings.Repeat("a", 40)
	report := sampleReport(sha)

	if err := RetainReport(workspace, artifact, report); err != nil {
		t.Fatal(err)
	}
	if err := RetainReport(workspace, artifact, report); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("overwrite error = %v", err)
	}
	loaded, err := LoadReport(artifact, sha)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CandidateSHA != sha || loaded.Schema != Schema || len(loaded.Claims) != 1 {
		t.Fatalf("loaded = %+v", loaded)
	}
}

func TestLoadReportFailClosed(t *testing.T) {
	workspace := t.TempDir()
	artifactDir := t.TempDir()
	sha := strings.Repeat("b", 40)
	report := sampleReport(sha)

	missing := filepath.Join(artifactDir, "missing.json")
	if _, err := LoadReport(missing, sha); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing error = %v", err)
	}

	artifact := filepath.Join(artifactDir, "matrix.json")
	if err := RetainReport(workspace, artifact, report); err != nil {
		t.Fatal(err)
	}
	other := strings.Repeat("c", 40)
	if _, err := LoadReport(artifact, other); err == nil || !strings.Contains(err.Error(), "does not match expected") {
		t.Fatalf("sha mismatch error = %v", err)
	}

	malformed := filepath.Join(artifactDir, "bad.json")
	if err := os.WriteFile(malformed, []byte(`{"schema":"nope"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadReport(malformed, sha); err == nil {
		t.Fatal("expected malformed failure")
	}

	trailing := filepath.Join(artifactDir, "trailing.json")
	good, err := encodeReport(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(trailing, append(good, []byte("{}\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadReport(trailing, sha); err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("trailing error = %v", err)
	}
}

func sampleReport(sha string) Report {
	return Report{
		Schema:       Schema,
		CandidateSHA: sha,
		HeadMatch:    "PASS",
		Tree:         "CLEAN",
		HostOS:       "darwin",
		HostArch:     "arm64",
		GoVersion:    "go1.25",
		GeneratedAt:  time.Unix(1700000000, 0).UTC().Format(time.RFC3339),
		Claims: []Claim{{
			ID:       "live-provider-quality",
			Category: CategoryLiveProvider,
			Title:    "Live provider",
			Setup:    "none",
			Artifact: "none",
			Verdict:  VerdictUnverified,
			Reason:   "unobserved",
		}},
		Summary:    map[string]int{string(VerdictUnverified): 1},
		Unverified: []string{"live-provider-quality"},
	}
}
