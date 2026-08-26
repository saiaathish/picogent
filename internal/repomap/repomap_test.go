package repomap

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectNodeRepository(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "package.json", `{
  "packageManager": "pnpm@9.0.0",
  "scripts": {"build":"next build","test":"vitest","lint":"eslint ."},
  "dependencies": {"next":"15.0.0","react":"19.0.0"},
  "devDependencies": {"vite":"6.0.0"}
}`)
	write(t, dir, "pnpm-lock.yaml", "lockfileVersion: '9.0'\n")
	write(t, dir, "src/index.tsx", "export default 1\n")
	write(t, dir, "tests/index.test.ts", "test('x', () => {})\n")
	write(t, dir, "AGENTS.md", "# Rules\n")
	write(t, dir, "node_modules/ignored/index.rb", "puts 'ignored'\n")

	m, err := Inspect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, m.Languages, "TypeScript")
	assertContains(t, m.Frameworks, "Next.js")
	assertContains(t, m.Frameworks, "React")
	assertContains(t, m.PackageManagers, "pnpm")
	assertContains(t, m.Commands.Build, "pnpm run build")
	assertContains(t, m.Commands.Test, "pnpm test")
	assertContains(t, m.Commands.Lint, "pnpm run lint")
	assertContains(t, m.SourceRoots, "src")
	assertContains(t, m.TestRoots, "tests")
	assertContains(t, m.Rules, "AGENTS.md")
	if contains(m.Languages, "Ruby") {
		t.Fatalf("ignored dependency changed languages: %v", m.Languages)
	}
	formatted := Format(m)
	if len(formatted) > MaxOutputBytes {
		t.Fatalf("repo map is %d bytes", len(formatted))
	}
	var decoded Map
	if err := json.Unmarshal([]byte(formatted), &decoded); err != nil {
		t.Fatalf("format is not JSON: %v\n%s", err, formatted)
	}
}

func TestInspectGoAndMakeCommands(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.test/repo\n\ngo 1.25\n")
	write(t, dir, "internal/auth/auth.go", "package auth\n")
	write(t, dir, "internal/auth/auth_test.go", "package auth\n")
	write(t, dir, "Makefile", "build:\n\tgo build ./...\n\ntest:\n\tgo test ./...\n\nlint:\n\tgo vet ./...\n")

	m, err := Inspect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, m.Languages, "Go")
	assertContains(t, m.PackageManagers, "go")
	assertContains(t, m.Commands.Build, "go build ./...")
	assertContains(t, m.Commands.Build, "make build")
	assertContains(t, m.Commands.Test, "go test ./...")
	assertContains(t, m.Commands.Lint, "go vet ./...")
	assertContains(t, m.TestRoots, "internal/auth")
}

func TestInspectGitState(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "--quiet", dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	write(t, dir, "go.mod", "module x\n")
	m, err := Inspect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !m.Git.Repository || m.Git.Clean || m.Git.Untracked != 1 {
		t.Fatalf("unexpected git state: %+v", m.Git)
	}
}

func TestCaptureGitProvenance(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	gitRun(t, dir, "init", "--quiet")
	gitRun(t, dir, "config", "user.name", "Picogent Test")
	gitRun(t, dir, "config", "user.email", "picogent@example.test")
	write(t, dir, "go.mod", "module example.test/repo\n\ngo 1.25\n")
	write(t, dir, "README.md", "initial\n")
	gitRun(t, dir, "add", "go.mod", "README.md")
	gitRun(t, dir, "commit", "--quiet", "-m", "initial")
	wantHead := strings.TrimSpace(gitRun(t, dir, "rev-parse", "--verify", "HEAD^{commit}"))
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "mv", "README.md", "docs/README.md")
	write(t, dir, "go.mod", "module example.test/repo\n\ngo 1.25\n\n// changed\n")
	write(t, dir, "nested/new.txt", "untracked\n")

	snapshot, err := Capture(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Root != filepath.Clean(dir) || snapshot.GitRoot != resolvedPath(t, dir) {
		t.Fatalf("roots = %q, %q; want %q", snapshot.Root, snapshot.GitRoot, resolvedPath(t, dir))
	}
	if !snapshot.HeadKnown || snapshot.Head != wantHead || len(snapshot.Head) != 40 {
		t.Fatalf("head provenance = %#v; want %s", snapshot, wantHead)
	}
	if !snapshot.DirtyKnown {
		t.Fatal("dirty paths should be known for a git worktree")
	}
	assertContains(t, snapshot.DirtyPaths, "go.mod")
	assertContains(t, snapshot.DirtyPaths, "nested/new.txt")
	assertContains(t, snapshot.DirtyPaths, "README.md")
	assertContains(t, snapshot.DirtyPaths, "docs/README.md")
	assertContains(t, snapshot.ManifestPaths, "go.mod")

	var formatted struct {
		Provenance struct {
			Head  string   `json:"head"`
			Tree  string   `json:"tree"`
			Dirty []string `json:"dirty_paths"`
		} `json:"provenance"`
	}
	if err := json.Unmarshal([]byte(FormatSnapshot(snapshot)), &formatted); err != nil {
		t.Fatal(err)
	}
	if formatted.Provenance.Head != wantHead || formatted.Provenance.Tree != "DIRTY" {
		t.Fatalf("formatted provenance = %#v", formatted.Provenance)
	}
	assertContains(t, formatted.Provenance.Dirty, "nested/new.txt")
}

func TestCaptureSubdirectoryScopesDirtyPaths(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	gitRun(t, dir, "init", "--quiet")
	gitRun(t, dir, "config", "user.name", "Picogent Test")
	gitRun(t, dir, "config", "user.email", "picogent@example.test")
	write(t, dir, "README.md", "initial\n")
	write(t, dir, "service/go.mod", "module example.test/service\n\ngo 1.25\n")
	write(t, dir, "service/package.json", "{}\n")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "--quiet", "-m", "initial")
	write(t, dir, "README.md", "changed outside workspace\n")
	write(t, dir, "service/go.mod", "module example.test/service\n\ngo 1.25\n\n// changed\n")

	snapshot, err := Capture(context.Background(), filepath.Join(dir, "service"))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.GitRoot != resolvedPath(t, dir) || !snapshot.HeadKnown {
		t.Fatalf("git provenance = %#v", snapshot)
	}
	assertContains(t, snapshot.DirtyPaths, "go.mod")
	if contains(snapshot.DirtyPaths, "../README.md") || contains(snapshot.DirtyPaths, "README.md") {
		t.Fatalf("outside-workspace dirty path leaked: %v", snapshot.DirtyPaths)
	}
	assertContains(t, snapshot.ManifestPaths, "go.mod")
	if contains(snapshot.ManifestPaths, "../go.mod") {
		t.Fatalf("outside-workspace manifest leaked: %v", snapshot.ManifestPaths)
	}
}

func TestManifestPathsAreBoundedAndExcludeDependencies(t *testing.T) {
	deep := strings.Repeat("level/", maxManifestDepth) + "package.json"
	files := []string{
		"go.mod",
		"services/api/package.json",
		"node_modules/ignored/package.json",
		"vendor/ignored/go.mod",
		deep,
	}
	paths, truncated := manifestPaths(files)
	if !truncated {
		t.Fatal("deep manifest should provide truncation evidence")
	}
	assertContains(t, paths, "go.mod")
	assertContains(t, paths, "services/api/package.json")
	if contains(paths, "node_modules/ignored/package.json") || contains(paths, "vendor/ignored/go.mod") || contains(paths, deep) {
		t.Fatalf("excluded manifest in %v", paths)
	}
}

func TestCompareReportsProvenanceChanges(t *testing.T) {
	before := Snapshot{
		Root:          "/workspace",
		Head:          "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		HeadKnown:     true,
		DirtyKnown:    true,
		DirtyPaths:    []string{"old.go"},
		ManifestPaths: []string{"go.mod"},
	}
	after := before
	after.Head = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	after.DirtyPaths = []string{"new.go"}
	after.ManifestPaths = []string{"go.mod", "package.json"}
	delta := Compare(before, after)
	if !delta.HeadChanged || !delta.DirtyPathsChanged || !delta.ManifestPathsChanged || !delta.RequiresRefresh {
		t.Fatalf("delta = %#v", delta)
	}
	assertContains(t, delta.AddedDirtyPaths, "new.go")
	assertContains(t, delta.RemovedDirtyPaths, "old.go")
	assertContains(t, delta.AddedManifestPaths, "package.json")
}

func TestFormatSnapshotIsBoundedAndTruthful(t *testing.T) {
	values := make([]string, 200)
	for i := range values {
		values[i] = "file-" + strings.Repeat("x", 700) + string(rune(i))
	}
	snapshot := Snapshot{
		Summary: Map{
			Version: 1,
			Root:    strings.Repeat("r", 2_000),
			Commands: Commands{
				Build: values,
				Test:  values,
				Lint:  values,
			},
			Manifests: values,
		},
		Root:          strings.Repeat("r", 2_000),
		Head:          strings.Repeat("a", 40),
		HeadKnown:     true,
		DirtyKnown:    true,
		DirtyPaths:    values,
		ManifestPaths: values,
	}
	formatted := FormatSnapshot(snapshot)
	if len(formatted) > MaxOutputBytes || !json.Valid([]byte(formatted)) {
		t.Fatalf("invalid bounded snapshot (%d bytes): %s", len(formatted), formatted)
	}
	if !strings.Contains(formatted, `"output_truncated": true`) || !strings.Contains(formatted, `"tree": "DIRTY"`) {
		t.Fatalf("snapshot lost truncation/provenance evidence: %s", formatted)
	}
}

func TestFormatSnapshotRedactsGitDerivedText(t *testing.T) {
	snapshot := Snapshot{
		Summary: Map{
			Root: "/workspace",
			Git:  GitState{Repository: true, Branch: "feature/api_key=branch-secret"},
		},
		Root:          "/workspace",
		Head:          strings.Repeat("a", 40),
		HeadKnown:     true,
		DirtyKnown:    true,
		DirtyPaths:    []string{"api_key=file-secret.txt"},
		ManifestPaths: []string{"password=manifest-secret.json"},
	}
	out := FormatSnapshot(snapshot)
	for _, secret := range []string{"branch-secret", "file-secret", "manifest-secret"} {
		if strings.Contains(out, secret) {
			t.Fatalf("formatted repo map retained secret %q: %s", secret, out)
		}
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("formatted repo map did not include redaction marker: %s", out)
	}
}

func TestCaptureNonGitIsUnverified(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "package.json", "{}\n")
	snapshot, err := Capture(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.HeadKnown || snapshot.DirtyKnown || snapshot.GitRoot != "" {
		t.Fatalf("non-git provenance = %#v", snapshot)
	}
	formatted := FormatSnapshot(snapshot)
	var decoded struct {
		Provenance struct {
			Tree string `json:"tree"`
		} `json:"provenance"`
	}
	if err := json.Unmarshal([]byte(formatted), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Provenance.Tree != "UNVERIFIED" {
		t.Fatalf("non-git tree = %q", decoded.Provenance.Tree)
	}
}

func TestFormatIsBoundedAndDeterministic(t *testing.T) {
	values := make([]string, 200)
	for i := range values {
		values[i] = strings.Repeat(string(rune('a'+i%26)), 4) + strings.Repeat("x", 700) + string(rune(i))
	}
	m := Map{
		Version:         1,
		Root:            strings.Repeat("r", 2_000),
		Languages:       values,
		Frameworks:      values,
		PackageManagers: values,
		Commands:        Commands{Build: values, Test: values, Lint: values},
		Manifests:       values,
		SourceRoots:     values,
		TestRoots:       values,
		Rules:           values,
	}
	one, two := Format(m), Format(m)
	if one != two {
		t.Fatal("format is not deterministic")
	}
	if len(one) > MaxOutputBytes {
		t.Fatalf("formatted map is %d bytes", len(one))
	}
	if !strings.Contains(one, `"output_truncated": true`) {
		t.Fatalf("missing truncation evidence: %s", one)
	}
	if !json.Valid([]byte(one)) {
		t.Fatal("bounded output must remain valid JSON")
	}
}

func write(t *testing.T, root, name, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertContains(t *testing.T, values []string, want string) {
	t.Helper()
	if !contains(values, want) {
		t.Fatalf("%q not in %v", want, values)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

func resolvedPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(resolved)
}
