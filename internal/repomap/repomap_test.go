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
