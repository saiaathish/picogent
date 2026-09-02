package verify

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseEvidenceWorkflowUsesExternalArtifactDirectory(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller did not return the test path")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
	workflowPath := filepath.Join(repoRoot, ".github", "workflows", "ci.yml")
	data, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read release workflow: %v", err)
	}
	workflow := string(data)

	for _, required := range []string{
		"ARTIFACT_DIR: ${{ runner.temp }}/picogent-release-evidence",
		"go run ./cmd/release-evidence-layout",
		"--workspace \"$GITHUB_WORKSPACE\"",
		"--evidence-dir \"$ARTIFACT_DIR\"",
		"> \"$ARTIFACT_DIR/verification-manifest.json\"",
		"subject-checksums: ${{ runner.temp }}/picogent-release-evidence/release-evidence.sha256",
		"predicate-path: ${{ runner.temp }}/picogent-release-evidence/release-attestation-predicate.json",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("release workflow is missing required external-artifact contract %q", required)
		}
	}
	if strings.Contains(workflow, "artifacts/") {
		t.Fatal("release workflow still contains a checkout-relative artifacts/ path")
	}

	validation := strings.Index(workflow, "go run ./cmd/release-evidence-layout")
	manifest := strings.Index(workflow, "go run ./cmd/verify-manifest")
	if validation < 0 || manifest < 0 || validation > manifest {
		t.Fatal("release evidence layout must be validated before manifest generation")
	}
}
