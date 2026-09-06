package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunBuildsArtifacts(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	workspace := t.TempDir()
	write(t, workspace, "go.mod", "module example.test/releaseartifact\n\ngo 1.25\n")
	write(t, workspace, "LICENSE", "MIT\n")
	write(t, workspace, "cmd/picogent/main.go", "package main\n\nvar version = \"dev\"\n\nfunc main() {}\n")
	git(t, workspace, "init", "--quiet")
	git(t, workspace, "config", "user.name", "Picogent Test")
	git(t, workspace, "config", "user.email", "picogent@example.test")
	git(t, workspace, "add", ".")
	git(t, workspace, "commit", "--quiet", "-m", "initial")
	sha := strings.TrimSpace(git(t, workspace, "rev-parse", "HEAD"))
	out := t.TempDir()

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{
		"--workspace", workspace,
		"--output-dir", out,
		"--candidate-sha", sha,
		"--source-date-epoch", "1700000000",
		"--targets", "linux/amd64",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(out, "release-artifact-manifest.json")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `"status": "PASS"`) {
		t.Fatalf("stdout=%s", stdout.String())
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
