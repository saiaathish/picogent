package tools

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGlobMatch(t *testing.T) {
	cases := []struct {
		pattern string
		rel     string
		want    bool
	}{
		{"**/*.go", "internal/tools/search.go", true},
		{"**/*.go", "search.go", true},
		{"**/*.go", "vendor/x.go", true},
		{"**/foo/**/bar", "foo/a/bar", true},
		{"**/foo/**/bar", "foo/bar", true},
		{"foo/**/bar.go", "foo/internal/bar.go", true},
		{"foo/**/bar.go", "other/foo/bar.go", false},
		{"*.go", "search.go", true},
		{"*.go", "tools/search.go", false},
	}
	for _, tc := range cases {
		got := globMatch(tc.pattern, tc.rel)
		if got != tc.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", tc.pattern, tc.rel, got, tc.want)
		}
	}
}

func TestGlobMatchNoStackOverflow(t *testing.T) {
	// Patterns that previously recursed forever when the **/ prefix branch missed.
	patterns := []string{
		"**/foo/**/bar",
		"**/**/x.go",
		"**/a/**/b/**/c.go",
	}
	rels := []string{"foo/x/bar", "deep/nested/x.go", "a/1/b/2/c.go", "nope.txt"}
	for _, pattern := range patterns {
		for _, rel := range rels {
			_ = globMatch(pattern, rel)
		}
	}
}

func TestSanitizedCommandEnvOmitsCredentialsAndStartupHooks(t *testing.T) {
	t.Setenv("PICOGENT_TEST_API_KEY", "do-not-leak")
	t.Setenv("BASH_ENV", "/tmp/evil-profile")
	t.Setenv("PICOGENT_TEST_SAFE", "kept")
	env := sanitizedCommandEnv()
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "PICOGENT_TEST_API_KEY=") || strings.Contains(joined, "BASH_ENV=") {
		t.Fatalf("sensitive environment leaked: %s", joined)
	}
	if !strings.Contains(joined, "PICOGENT_TEST_SAFE=kept") {
		t.Fatalf("ordinary environment was unexpectedly removed: %s", joined)
	}
	if os.Getenv("PATH") != "" && !hasEnvKey(env, "PATH") {
		t.Fatal("sanitized environment removed PATH")
	}
}

func TestGitOutRedactsCredentialShapedDiff(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	repo := t.TempDir()
	runToolsGit(t, repo, "init", "--quiet")
	runToolsGit(t, repo, "config", "user.name", "Picogent Test")
	runToolsGit(t, repo, "config", "user.email", "picogent@example.test")
	path := filepath.Join(repo, "config.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runToolsGit(t, repo, "add", "config.txt")
	runToolsGit(t, repo, "commit", "--quiet", "-m", "initial")
	if err := os.WriteFile(path, []byte("api_key=diff-secret\npassword=another-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := gitOut(context.Background(), repo, "diff")
	if err != nil {
		t.Fatalf("gitOut: %v", err)
	}
	for _, secret := range []string{"diff-secret", "another-secret"} {
		if strings.Contains(got, secret) {
			t.Fatalf("git output retained secret %q: %q", secret, got)
		}
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("git output did not include redaction marker: %q", got)
	}
}

func runToolsGit(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return string(out)
}
