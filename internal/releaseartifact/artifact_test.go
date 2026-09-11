package releaseartifact

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBuildProducesDeterministicArtifacts(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	workspace := t.TempDir()
	seedWorkspace(t, workspace)
	sha := commitAll(t, workspace, "initial")
	epoch := int64(1700000000)

	out1 := t.TempDir()
	out2 := t.TempDir()
	first, err := Build(Options{
		Workspace:       workspace,
		OutputDir:       out1,
		CandidateSHA:    sha,
		SourceDateEpoch: epoch,
		Targets:         []Target{{GOOS: "linux", GOARCH: "amd64"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(Options{
		Workspace:       workspace,
		OutputDir:       out2,
		CandidateSHA:    sha,
		SourceDateEpoch: epoch,
		Targets:         []Target{{GOOS: "linux", GOARCH: "amd64"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != "PASS" || second.Status != "PASS" {
		t.Fatalf("status first=%s second=%s", first.Status, second.Status)
	}
	if first.Targets[0].Binary.SHA256 != second.Targets[0].Binary.SHA256 {
		t.Fatalf("binary digest drifted: %s vs %s", first.Targets[0].Binary.SHA256, second.Targets[0].Binary.SHA256)
	}
	if first.Targets[0].Package.SHA256 != second.Targets[0].Package.SHA256 {
		t.Fatalf("package digest drifted")
	}
	if first.Targets[0].SBOM.SHA256 != second.Targets[0].SBOM.SHA256 {
		t.Fatalf("sbom digest drifted")
	}
	assertFileOutside(t, workspace, filepath.Join(out1, first.Targets[0].Binary.Name))
	assertSPDX(t, filepath.Join(out1, first.Targets[0].SBOM.Name))
}

func TestBuildFailsClosedOnDirtyTree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	workspace := t.TempDir()
	seedWorkspace(t, workspace)
	sha := commitAll(t, workspace, "initial")
	if err := os.WriteFile(filepath.Join(workspace, "dirty.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Build(Options{
		Workspace:       workspace,
		OutputDir:       t.TempDir(),
		CandidateSHA:    sha,
		SourceDateEpoch: 1700000000,
		Targets:         []Target{{GOOS: "linux", GOARCH: "amd64"}},
	})
	if err == nil || !strings.Contains(err.Error(), "clean") {
		t.Fatalf("expected dirty failure, got %v", err)
	}
}

func TestBuildRejectsOutputInsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	_, err := Build(Options{
		Workspace:       workspace,
		OutputDir:       filepath.Join(workspace, "out"),
		CandidateSHA:    strings.Repeat("a", 40),
		SourceDateEpoch: 1700000000,
		Targets:         []Target{{GOOS: "linux", GOARCH: "amd64"}},
	})
	if err == nil || !strings.Contains(err.Error(), "inside workspace") {
		t.Fatalf("expected layout failure, got %v", err)
	}
}

func TestBuildRejectsSymlinkedOutputDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	workspace := t.TempDir()
	seedWorkspace(t, workspace)
	sha := commitAll(t, workspace, "initial")
	outside := t.TempDir()
	outputDir := filepath.Join(t.TempDir(), "release-output")
	if err := os.Symlink(outside, outputDir); err != nil {
		t.Fatal(err)
	}

	_, err := Build(Options{
		Workspace:       workspace,
		OutputDir:       outputDir,
		CandidateSHA:    sha,
		SourceDateEpoch: 1700000000,
		Targets:         []Target{{GOOS: "linux", GOARCH: "amd64"}},
	})
	if err == nil {
		t.Fatal("Build accepted a symlinked output directory")
	}
}

func TestPublishArtifactRejectsSymlinkedTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	source := filepath.Join(t.TempDir(), "source.bin")
	if err := os.WriteFile(source, []byte("safe artifact"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.bin")
	if err := os.WriteFile(outside, []byte("must survive"), 0o600); err != nil {
		t.Fatal(err)
	}
	outputDir := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(outputDir, "artifact.bin")); err != nil {
		t.Fatal(err)
	}

	if err := publishArtifact(outputDir, "artifact.bin", source, 0o644); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("publish symlink = %v, want symbolic-link rejection", err)
	}
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != "must survive" {
		t.Fatalf("outside target after publish = %q, %v", data, err)
	}
}

func TestBuildFailsOnSHAMismatch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	workspace := t.TempDir()
	seedWorkspace(t, workspace)
	_ = commitAll(t, workspace, "initial")
	_, err := Build(Options{
		Workspace:       workspace,
		OutputDir:       t.TempDir(),
		CandidateSHA:    strings.Repeat("b", 40),
		SourceDateEpoch: 1700000000,
		Targets:         []Target{{GOOS: "linux", GOARCH: "amd64"}},
	})
	if err == nil {
		t.Fatal("expected sha mismatch failure")
	}
}

func seedWorkspace(t *testing.T, workspace string) {
	t.Helper()
	write(t, workspace, "go.mod", "module example.test/releaseartifact\n\ngo 1.25\n")
	write(t, workspace, "LICENSE", "MIT\n")
	write(t, workspace, "cmd/picogent/main.go", "package main\n\nvar version = \"dev\"\n\nfunc main() {}\n")
	git(t, workspace, "init", "--quiet")
	git(t, workspace, "config", "user.name", "Picogent Test")
	git(t, workspace, "config", "user.email", "picogent@example.test")
}

func commitAll(t *testing.T, workspace, message string) string {
	t.Helper()
	git(t, workspace, "add", ".")
	git(t, workspace, "commit", "--quiet", "-m", message)
	return strings.TrimSpace(git(t, workspace, "rev-parse", "HEAD"))
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

func assertFileOutside(t *testing.T, workspace, path string) {
	t.Helper()
	rel, err := filepath.Rel(workspace, path)
	if err != nil {
		t.Fatal(err)
	}
	if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
		t.Fatalf("artifact %s is inside workspace", path)
	}
}

func assertSPDX(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["spdxVersion"] != SPDXVersion {
		t.Fatalf("spdxVersion = %v", doc["spdxVersion"])
	}
	packages, _ := doc["packages"].([]any)
	if len(packages) < 1 {
		t.Fatalf("expected packages, got %v", doc["packages"])
	}
}
