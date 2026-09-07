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

func TestRetainReportRejectsSymlinkParent(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	linkRoot := t.TempDir()
	link := filepath.Join(linkRoot, "linked-parent")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	artifact := filepath.Join(link, "runtime-boundary-matrix.json")
	err := RetainReport(workspace, artifact, sampleReport(strings.Repeat("d", 40)))
	if err == nil {
		t.Fatal("retain accepted a symlink parent")
	}
	if _, err := os.Stat(filepath.Join(outside, "runtime-boundary-matrix.json")); !os.IsNotExist(err) {
		t.Fatalf("symlink parent received a write: %v", err)
	}
}

func TestRetainReportRejectsSymlinkParentIntoWorkspace(t *testing.T) {
	workspace := t.TempDir()
	linkRoot := t.TempDir()
	link := filepath.Join(linkRoot, "into-workspace")
	if err := os.Symlink(workspace, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	artifact := filepath.Join(link, "runtime-boundary-matrix.json")
	if err := RetainReport(workspace, artifact, sampleReport(strings.Repeat("e", 40))); err == nil {
		t.Fatal("retain accepted a parent symlink into the workspace")
	}
	if _, err := os.Stat(filepath.Join(workspace, "runtime-boundary-matrix.json")); !os.IsNotExist(err) {
		t.Fatalf("workspace received retained artifact through symlink parent: %v", err)
	}
}

func TestRetainReportRejectsSymlinkArtifactTarget(t *testing.T) {
	workspace := t.TempDir()
	artifactDir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(artifactDir, "runtime-boundary-matrix.json")
	if err := os.Symlink(outside, artifact); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := RetainReport(workspace, artifact, sampleReport(strings.Repeat("f", 40))); err == nil {
		t.Fatal("retain accepted an existing symlink artifact path")
	}
	got, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep\n" {
		t.Fatalf("symlink target changed to %q", got)
	}
}

func TestLoadReportRejectsSymlinkParent(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	linkRoot := t.TempDir()
	link := filepath.Join(linkRoot, "linked-parent")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	sha := strings.Repeat("1", 40)
	artifact := filepath.Join(outside, "runtime-boundary-matrix.json")
	if err := RetainReport(workspace, artifact, sampleReport(sha)); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadReport(filepath.Join(link, filepath.Base(artifact)), sha); err == nil {
		t.Fatal("load accepted a symlinked artifact parent")
	}
}

func sampleReport(sha string) Report {
	return Report{
		Schema:             Schema,
		CandidateSHA:       sha,
		BehaviorSHA:        sha,
		BehaviorProvenance: BehaviorProvenanceExact,
		HeadMatch:          "PASS",
		Tree:               "CLEAN",
		HostOS:             "darwin",
		HostArch:           "arm64",
		GoVersion:          "go1.25",
		GeneratedAt:        time.Unix(1700000000, 0).UTC().Format(time.RFC3339),
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
