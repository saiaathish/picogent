package slash

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCatalogIncludesDocumentedBuiltins(t *testing.T) {
	items := Catalog(t.TempDir())
	want := []string{
		"commit", "review", "status", "diff", "undo", "compact", "memory",
		"goal", "agent", "ask", "plan", "debug", "clear",
	}
	got := map[string]Item{}
	for _, it := range items {
		got[it.Name] = it
	}
	for _, name := range want {
		if _, ok := got[name]; !ok {
			t.Fatalf("Catalog missing %q (have %#v)", name, items)
		}
	}
	if got["goal"].Insert != "/goal " {
		t.Fatalf("goal insert=%q, want %q", got["goal"].Insert, "/goal ")
	}
}

func TestResolveUndo(t *testing.T) {
	kind, payload := Resolve(t.TempDir(), "/undo")
	if kind != Local || payload != "undo" {
		t.Fatalf("Resolve(/undo) = (%v, %q), want (%v, %q)", kind, payload, Local, "undo")
	}
}

func TestGitDiffUsesWorkspaceAndRedactsSecrets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX helper script")
	}
	repo := initGitDiffRepo(t)
	marker := filepath.Join(t.TempDir(), "helper-ran")
	t.Setenv("PICOGENT_TEST_SLASH_GITDIFF_MARKER", marker)
	helper := filepath.Join(t.TempDir(), "external-diff")
	writeGitDiffFile(t, helper, "#!/bin/sh\n: > \"$PICOGENT_TEST_SLASH_GITDIFF_MARKER\"\n")
	if err := os.Chmod(helper, 0o700); err != nil {
		t.Fatal(err)
	}
	runGitDiff(t, repo, "config", "diff.external", helper)

	const secret = "slash-diff-secret"
	writeGitDiffFile(t, filepath.Join(repo, "content.txt"), "after\napi_key=\""+secret+"\"\n")
	got := GitDiff(repo)
	if strings.Contains(got, secret) {
		t.Fatalf("git diff leaked secret %q: %q", secret, got)
	}
	if !strings.Contains(got, "[REDACTED]") || !strings.Contains(got, "after") {
		t.Fatalf("git diff lost redaction or ordinary output: %q", got)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("repository-configured diff helper ran: %v", err)
	}
}

func TestGitDiffPreservesNoDiffAndDisplayBound(t *testing.T) {
	repo := initGitDiffRepo(t)
	if got := GitDiff(repo); got != "(no diff)" {
		t.Fatalf("clean repository diff = %q, want no-diff marker", got)
	}

	writeGitDiffFile(t, filepath.Join(repo, "content.txt"), strings.Repeat("changed\n", 2000))
	got := GitDiff(repo)
	const suffix = "\n… truncated …"
	if !strings.HasSuffix(got, suffix) {
		t.Fatalf("large git diff missing truncation marker: suffix=%q", got[max(0, len(got)-len(suffix)):])
	}
	if want := 8000 + len(suffix); len(got) != want {
		t.Fatalf("large git diff bytes = %d, want %d", len(got), want)
	}
}

func initGitDiffRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGitDiff(t, repo, "init", "--quiet")
	runGitDiff(t, repo, "config", "user.name", "Picogent Test")
	runGitDiff(t, repo, "config", "user.email", "picogent@example.test")
	writeGitDiffFile(t, filepath.Join(repo, "content.txt"), "before\n")
	runGitDiff(t, repo, "add", "content.txt")
	runGitDiff(t, repo, "commit", "--quiet", "-m", "initial")
	return repo
}

func writeGitDiffFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runGitDiff(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return string(out)
}
