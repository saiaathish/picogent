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

	checkoutRef := "ref: ${{ github.event_name == 'pull_request' && github.event.pull_request.head.sha || github.sha }}"
	checkoutRepository := "repository: ${{ github.event_name == 'pull_request' && github.event.pull_request.head.repo.full_name || github.repository }}"
	jobs := []string{"test", "security", "release-evidence"}
	for _, job := range jobs {
		start := strings.Index(workflow, "\n  "+job+":")
		if start < 0 {
			t.Fatalf("release workflow is missing %s job", job)
		}
		section := workflow[start:]
		for _, nextJob := range jobs {
			if nextJob == job {
				continue
			}
			if next := strings.Index(workflow[start+1:], "\n  "+nextJob+":"); next >= 0 && next+1 < len(section) {
				section = section[:next+1]
			}
		}
		if !strings.Contains(section, checkoutRef) {
			t.Errorf("%s job must checkout the exact pull-request source ref", job)
		}
		if !strings.Contains(section, checkoutRepository) {
			t.Errorf("%s job must checkout the pull-request source repository", job)
		}
	}

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
