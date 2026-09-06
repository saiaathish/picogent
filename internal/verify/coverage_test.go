package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseCoverProfileSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "coverage.out")
	content := "mode: set\nexample.com/pkg/file.go:1.1,2.2 2 1\nexample.com/pkg/file.go:3.1,4.2 2 0\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got := ParseCoverProfile(path)
	if got.Status != ManifestPass || got.Percent == nil || *got.Percent != 50 {
		t.Fatalf("coverage = %+v", got)
	}
}

func TestParseCoverProfileFailClosed(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		path    string
		content string
		reason  string
	}{
		{name: "missing", path: filepath.Join(dir, "missing.out"), reason: "coverprofile was not written"},
		{name: "empty", path: filepath.Join(dir, "empty.out"), content: "", reason: "coverprofile is empty"},
		{name: "bad mode", path: filepath.Join(dir, "bad-mode.out"), content: "not-a-mode\n", reason: "coverprofile is missing a mode line"},
		{name: "malformed", path: filepath.Join(dir, "bad.out"), content: "mode: set\nbad-line\n", reason: "coverprofile contains a malformed record"},
		{name: "no statements", path: filepath.Join(dir, "zero.out"), content: "mode: set\n", reason: "coverprofile has no statements"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.content != "" || strings.HasSuffix(tc.path, "empty.out") {
				if err := os.WriteFile(tc.path, []byte(tc.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got := ParseCoverProfile(tc.path)
			if got.Status != ManifestUnverified || !strings.Contains(got.Reason, tc.reason) {
				t.Fatalf("coverage = %+v", got)
			}
		})
	}
}

func TestRunPipelineCollectsTargetedCoverProfile(t *testing.T) {
	dir := t.TempDir()
	writeVerifyFile(t, dir, "go.mod", "module example.test/cover\n\ngo 1.25\n")
	writeVerifyFile(t, dir, "pkg/pkg.go", "package pkg\n\nfunc Value() int { return 1 }\n")
	writeVerifyFile(t, dir, "pkg/pkg_test.go", "package pkg\n\nimport \"testing\"\n\nfunc TestValue(t *testing.T) {\n\tif Value() != 1 {\n\t\tt.Fatal(\"bad\")\n\t}\n}\n")
	evidenceDir := t.TempDir()
	profile := filepath.Join(evidenceDir, "verification-coverage.out")
	if err := ValidateReleaseEvidenceDirectory(dir, evidenceDir); err != nil {
		t.Fatal(err)
	}

	result := RunPipeline(t.Context(), dir, Options{
		Targets:         []string{"pkg"},
		CoverProfile:    profile,
		TargetedTimeout: 30 * time.Second,
		Timeout:         30 * time.Second,
	})
	if result.Status != StatusPass {
		t.Fatalf("pipeline = %+v", result)
	}
	if len(result.Stages) != 2 || result.Stages[0].Status != StatusPass || len(result.Stages[0].Evidence) != 1 {
		t.Fatalf("stages = %+v", result.Stages)
	}
	coverage := result.Stages[0].Evidence[0].Coverage
	if coverage.Status != ManifestPass || coverage.Percent == nil || *coverage.Percent <= 0 {
		t.Fatalf("targeted coverage = %+v", coverage)
	}
	if _, err := os.Stat(profile); err != nil {
		t.Fatalf("coverprofile missing: %v", err)
	}

	manifest := ManifestFromPipeline(result, HeadEvidence{
		SHA: strings.Repeat("a", 40), ExpectedSHA: strings.Repeat("a", 40), Match: ManifestPass, Tree: "CLEAN",
	})
	// Broader coverage remains unverified, so overall PASS is not claimed.
	if manifest.Status != ManifestUnverified || !strings.Contains(manifest.Reason, "coverage") {
		t.Fatalf("manifest must stay UNVERIFIED without broader coverage: %+v", manifest)
	}
	targeted := false
	for _, check := range manifest.Checks {
		if check.Scope == ScopeTargeted && check.Coverage.Status == ManifestPass {
			targeted = true
		}
	}
	if !targeted {
		t.Fatalf("targeted covered check missing: %+v", manifest.Checks)
	}
}

func TestRunPipelineCoverProfileMissingTargetIsInconclusive(t *testing.T) {
	dir := t.TempDir()
	writeVerifyFile(t, dir, "go.mod", "module example.test/cover\n\ngo 1.25\n")
	profile := filepath.Join(t.TempDir(), "verification-coverage.out")
	result := RunPipeline(t.Context(), dir, Options{
		Targets:      []string{"src/main.rs"},
		CoverProfile: profile,
	})
	if result.Status != StatusInconclusive {
		t.Fatalf("missing target pipeline = %+v", result)
	}
}

func TestRunPipelineCoverProfileTimeoutIsInconclusive(t *testing.T) {
	dir := t.TempDir()
	writeVerifyFile(t, dir, "go.mod", "module example.test/cover\n\ngo 1.25\n")
	writeVerifyFile(t, dir, "slow/slow_test.go", "package slow\n\nimport \"testing\"\nimport \"time\"\n\nfunc TestSlow(t *testing.T) { time.Sleep(2 * time.Second) }\n")
	profile := filepath.Join(t.TempDir(), "verification-coverage.out")
	result := RunPipeline(t.Context(), dir, Options{
		Targets:         []string{"slow"},
		CoverProfile:    profile,
		TargetedTimeout: 50 * time.Millisecond,
		Timeout:         50 * time.Millisecond,
	})
	if result.Status != StatusInconclusive {
		t.Fatalf("timeout pipeline = %+v", result)
	}
}
