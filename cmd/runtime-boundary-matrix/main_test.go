package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/runtimeboundary"
)

func TestRunEmitsMatrix(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	workspace := t.TempDir()
	write(t, workspace, "go.mod", "module example.test/runtimeboundary\n\ngo 1.25\n")
	write(t, workspace, "docs/V4-RENDERED-LONG-HORIZON-EVIDENCE.md", "# rendered\n")
	write(t, workspace, "docs/V4-RENDERED-RECOVERY-FIXTURE.md", "# recovery\n")
	write(t, workspace, "docs/V4-RENDERED-RECOVERY-API-BOUNDARY.md", "# api-boundary\n")
	write(t, workspace, "docs/V4-SECURITY-CAMPAIGN.md", "# security\n")
	write(t, workspace, "docs/V4-LONG-HORIZON-OUTCOME.md", "# long horizon\n")
	write(t, workspace, "docs/V4-RELEASE-AUDIT.md", "# audit\n")
	write(t, workspace, "docs/V4-SBOM-ARTIFACT-EVIDENCE.md", "# sbom\n")
	git(t, workspace, "init", "--quiet")
	git(t, workspace, "config", "user.name", "Picogent Test")
	git(t, workspace, "config", "user.email", "picogent@example.test")
	git(t, workspace, "add", ".")
	git(t, workspace, "commit", "--quiet", "-m", "initial")
	sha := strings.TrimSpace(git(t, workspace, "rev-parse", "HEAD"))

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"--workspace", workspace, "--candidate-sha", sha}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	var report runtimeboundary.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("json: %v\n%s", err, stdout.String())
	}
	if report.Schema != runtimeboundary.Schema || report.CandidateSHA != sha || len(report.Claims) == 0 {
		t.Fatalf("report = %+v", report)
	}
}

func write(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}
