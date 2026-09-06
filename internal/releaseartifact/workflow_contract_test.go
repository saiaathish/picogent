package releaseartifact

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReleaseArtifactsWorkflowContract(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
	data, err := os.ReadFile(filepath.Join(repoRoot, ".github", "workflows", "release-artifacts.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(data)
	for _, required := range []string{
		"go-version: \"1.25.14\"",
		"actions/checkout@11d5960a326750d5838078e36cf38b85af677262",
		"actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff",
		"actions/attest@1e69f48acb82d1966a394da916b4c1698aa569d6",
		"actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02",
		"go run ./cmd/release-evidence-layout",
		"go run ./cmd/release-artifact",
		"--targets linux/amd64,darwin/arm64,windows/amd64",
		"predicate-type: https://github.com/saiaathish/picogent/attestation/release-artifact/v1",
		"ARTIFACT_DIR: ${{ runner.temp }}/picogent-release-artifacts",
		"if-no-files-found: error",
		`"status":"UNVERIFIED"`,
		"ref: ${{ github.event_name == 'pull_request' && github.event.pull_request.head.sha || github.sha }}",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("workflow missing %q", required)
		}
	}
	for _, banned := range []string{"curl | bash", "npm login"} {
		if strings.Contains(workflow, banned) {
			t.Errorf("workflow contains banned fragment %q", banned)
		}
	}
	if strings.Contains(workflow, "path: artifacts/") {
		t.Fatal("workflow must not write checkout-relative artifacts/")
	}
}
