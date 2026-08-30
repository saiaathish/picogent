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

	"github.com/saiaathish/picogent/internal/verify"
)

func TestRunEmitsExactHeadManifest(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.test/verify-manifest\n\ngo 1.25\n")
	writeFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	writeFile(t, dir, "main_test.go", "package main\n\nimport \"testing\"\n\nfunc TestSmoke(t *testing.T) {}\n")
	gitRun(t, dir, "init", "--quiet")
	gitRun(t, dir, "config", "user.name", "Picogent Test")
	gitRun(t, dir, "config", "user.email", "picogent@example.test")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "--quiet", "-m", "initial")
	head := strings.TrimSpace(gitRun(t, dir, "rev-parse", "--verify", "HEAD^{commit}"))

	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"--workspace", dir, "--expected-sha", head}, &stdout, &stderr); code != 0 {
		t.Fatalf("run exit code = %d, stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
	var manifest verify.Manifest
	if err := json.Unmarshal(stdout.Bytes(), &manifest); err != nil {
		t.Fatalf("manifest JSON: %v\n%s", err, stdout.String())
	}
	if manifest.Schema != verify.ManifestSchema || manifest.Head.SHA != head || manifest.Head.ExpectedSHA != head || manifest.Head.Match != verify.ManifestPass || manifest.Head.Tree != "CLEAN" {
		t.Fatalf("manifest provenance = %+v", manifest.Head)
	}
	passingCheck := false
	for _, check := range manifest.Checks {
		if check.Status == verify.ManifestPass {
			passingCheck = true
			break
		}
	}
	if !passingCheck {
		t.Fatalf("manifest checks = %+v", manifest.Checks)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}
