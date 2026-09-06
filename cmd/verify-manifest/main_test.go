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
	if code := run(context.Background(), []string{"--workspace", dir, "--expected-sha", head, "--target", "main.go"}, &stdout, &stderr); code != 0 {
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
	targetedCheck := false
	for _, check := range manifest.Checks {
		if check.Status == verify.ManifestPass {
			passingCheck = true
		}
		if check.Scope == verify.ScopeTargeted {
			if check.Status != verify.ManifestPass {
				t.Fatalf("targeted check = %+v", check)
			}
			targetedCheck = true
		}
	}
	if !passingCheck {
		t.Fatalf("manifest checks = %+v", manifest.Checks)
	}
	if !targetedCheck {
		t.Fatalf("manifest omitted targeted check: %+v", manifest.Checks)
	}
}

func TestRunRejectsEmptyTarget(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"--target", " "}, &stdout, &stderr); code != 2 {
		t.Fatalf("run exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "target must not be empty") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunRejectsCoverProfileInsideWorkspace(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.test/verify-manifest\n\ngo 1.25\n")
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{
		"--workspace", dir,
		"--target", ".",
		"--coverprofile", filepath.Join(dir, "coverage.out"),
	}, &stdout, &stderr)
	if code == 0 || !strings.Contains(stderr.String(), "coverprofile layout") {
		t.Fatalf("exit=%d stderr=%q stdout=%q", code, stderr.String(), stdout.String())
	}
}

func TestRunCollectsExternalCoverProfile(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.test/verify-manifest\n\ngo 1.25\n")
	writeFile(t, dir, "pkg/pkg.go", "package pkg\n\nfunc Value() int { return 2 }\n")
	writeFile(t, dir, "pkg/pkg_test.go", "package pkg\n\nimport \"testing\"\n\nfunc TestValue(t *testing.T) {\n\tif Value() != 2 {\n\t\tt.Fatal(Value())\n\t}\n}\n")
	gitRun(t, dir, "init", "--quiet")
	gitRun(t, dir, "config", "user.name", "Picogent Test")
	gitRun(t, dir, "config", "user.email", "picogent@example.test")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "--quiet", "-m", "initial")
	head := strings.TrimSpace(gitRun(t, dir, "rev-parse", "--verify", "HEAD^{commit}"))
	evidence := t.TempDir()
	profile := filepath.Join(evidence, "verification-coverage.out")

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{
		"--workspace", dir,
		"--expected-sha", head,
		"--target", "pkg",
		"--coverprofile", profile,
		"--targeted-timeout", "30s",
		"--timeout", "30s",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	var manifest verify.Manifest
	if err := json.Unmarshal(stdout.Bytes(), &manifest); err != nil {
		t.Fatalf("manifest JSON: %v\n%s", err, stdout.String())
	}
	if _, err := os.Stat(profile); err != nil {
		t.Fatalf("coverprofile missing: %v", err)
	}
	found := false
	for _, check := range manifest.Checks {
		if check.Scope == verify.ScopeTargeted && check.Coverage.Status == verify.ManifestPass && check.Coverage.Percent != nil {
			found = true
		}
	}
	if !found {
		t.Fatalf("targeted coverage missing: %+v", manifest.Checks)
	}
}

func TestRunDirtyTreeStaysUnverifiedWithCoverage(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	writeFile(t, dir, "go.mod", "module example.test/verify-manifest\n\ngo 1.25\n")
	writeFile(t, dir, "pkg/pkg.go", "package pkg\n\nfunc Value() int { return 3 }\n")
	writeFile(t, dir, "pkg/pkg_test.go", "package pkg\n\nimport \"testing\"\n\nfunc TestValue(t *testing.T) {}\n")
	gitRun(t, dir, "init", "--quiet")
	gitRun(t, dir, "config", "user.name", "Picogent Test")
	gitRun(t, dir, "config", "user.email", "picogent@example.test")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "--quiet", "-m", "initial")
	head := strings.TrimSpace(gitRun(t, dir, "rev-parse", "--verify", "HEAD^{commit}"))
	writeFile(t, dir, "dirty.txt", "uncommitted\n")
	evidence := t.TempDir()
	profile := filepath.Join(evidence, "verification-coverage.out")

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{
		"--workspace", dir,
		"--expected-sha", head,
		"--target", "pkg",
		"--coverprofile", profile,
		"--timeout", "30s",
		"--targeted-timeout", "30s",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	var manifest verify.Manifest
	if err := json.Unmarshal(stdout.Bytes(), &manifest); err != nil {
		t.Fatalf("manifest JSON: %v\n%s", err, stdout.String())
	}
	if manifest.Head.Tree != "DIRTY" {
		t.Fatalf("expected dirty tree, got %+v", manifest.Head)
	}
	if manifest.Status == verify.ManifestPass {
		t.Fatalf("dirty tree must not PASS: %+v", manifest)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
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
