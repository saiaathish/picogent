package tools

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/procenv"
)

func TestRipgrepArgumentsKeepUntrustedValuesAttached(t *testing.T) {
	for _, value := range []string{"--help", "-m", "--pre=program", "--glob=*.go", "--", "", "-e needle", "needle\n--pre=program"} {
		args := ripgrepArguments(value, value)
		want := []string{"--no-config", "--line-number", "--no-heading", "--color", "never", "-m", "50", "--max-filesize", "1M"}
		if value != "" {
			want = append(want, "--glob="+value)
		}
		want = append(want, "--regexp="+value, "--", ".")
		if !reflect.DeepEqual(args, want) {
			t.Fatalf("untrusted value escaped its attached argument: got %q, want %q", args, want)
		}
	}
}

func TestWalkGrepKeepsOrdinaryMatchesAndBoundsLargeFiles(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "fixture.txt"), []byte("ordinary needle\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := walkGrep(ws, "ordinary needle", "*.txt")
	if err != nil || !strings.Contains(got, "fixture.txt:1:ordinary needle") {
		t.Fatalf("ordinary fallback search failed: %q %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(ws, "large.txt"), []byte(strings.Repeat("large marker\n", 100000)), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = walkGrep(ws, "large marker", "*.txt")
	if err != nil || got != "no matches" {
		t.Fatal("fallback search did not skip its bounded large-file input")
	}
}

func TestRunRipgrepTreatsOptionShapedPatternsAsData(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep is not installed; portable argument tests cover this boundary")
	}
	ws := t.TempDir()
	patterns := []string{"--help", "-m", "--pre=missing-program", "--glob=*.go", "--"}
	// grep accepts regular expressions: '*' quantifies the preceding '=', so
	// this glob-shaped pattern legitimately matches '--glob=.go'.
	fixture := "--help\n-m\n--pre=missing-program\n--glob=.go\n--\n"
	if err := os.WriteFile(filepath.Join(ws, "fixture.txt"), []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, pattern := range patterns {
		t.Run(pattern, func(t *testing.T) {
			got, err := runRipgrep(context.Background(), ws, pattern, "")
			if err != nil || !strings.Contains(got, "fixture.txt:") {
				t.Fatalf("pattern became a command option instead of matching the fixture: err=%v", err)
			}
		})
	}
	if err := os.WriteFile(filepath.Join(ws, "--help"), []byte("ordinary needle\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := runRipgrep(context.Background(), ws, "ordinary needle", "--help")
	if err != nil || !strings.Contains(got, "--help:") {
		t.Fatalf("option-shaped glob was not kept as a value: err=%v", err)
	}
}

func TestWalkGrepDoesNotFollowOutsideWorkspaceSymlink(t *testing.T) {
	ws := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.txt")
	const marker = "OUTSIDE_GREP_SENTINEL"
	if err := os.WriteFile(outside, []byte(marker+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(ws, "linked.txt")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	got, err := walkGrep(ws, marker, "")
	if err != nil || got != "no matches" {
		t.Fatal("fallback search read a path outside its workspace")
	}
}

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
	env := procenv.Sanitized()
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "PICOGENT_TEST_API_KEY=") || strings.Contains(joined, "BASH_ENV=") {
		t.Fatalf("sensitive environment leaked: %s", joined)
	}
	if !strings.Contains(joined, "PICOGENT_TEST_SAFE=kept") {
		t.Fatalf("ordinary environment was unexpectedly removed: %s", joined)
	}
	if os.Getenv("PATH") != "" && !strings.Contains(joined, "PATH=") {
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
