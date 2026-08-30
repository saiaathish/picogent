package gitobs

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCommandSanitizesInheritedGitControls(t *testing.T) {
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "diff.external")
	t.Setenv("GIT_CONFIG_VALUE_0", "/tmp/untrusted")
	t.Setenv("GIT_EXTERNAL_DIFF", "/tmp/untrusted")
	t.Setenv("GIT_TRACE", "/tmp/untrusted-trace")
	t.Setenv("PAGER", "/tmp/untrusted-pager")
	t.Setenv("PICOGENT_TEST_SECRET_TOKEN", "do-not-inherit")
	t.Setenv("PICOGENT_TEST_SAFE", "kept")

	cmd := Command(context.Background(), t.TempDir(), "status")
	for _, leaked := range []string{
		"GIT_CONFIG_COUNT=", "GIT_CONFIG_KEY_0=", "GIT_CONFIG_VALUE_0=",
		"GIT_TRACE=", "PAGER=", "PICOGENT_TEST_SECRET_TOKEN=",
	} {
		if envContains(cmd.Env, strings.TrimSuffix(leaked, "=")) {
			t.Fatalf("inherited Git control leaked through environment: %q", leaked)
		}
	}
	if envContainsExact(cmd.Env, "GIT_EXTERNAL_DIFF=/tmp/untrusted") {
		t.Fatal("inherited GIT_EXTERNAL_DIFF value leaked through environment")
	}
	if !envContainsExact(cmd.Env, "PICOGENT_TEST_SAFE=kept") {
		t.Fatal("ordinary environment was unexpectedly removed")
	}
	for _, required := range []string{
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_EXTERNAL_DIFF=",
	} {
		if !envContainsExact(cmd.Env, required) {
			t.Fatalf("safe Git environment missing %q; keys=%v", required, envKeys(cmd.Env))
		}
	}

	args := strings.Join(cmd.Args, " ")
	for _, required := range []string{
		"--no-pager", "--no-optional-locks", "core.fsmonitor=false", "core.hooksPath=",
		"credential.helper=", "core.askPass=", "diff.external=",
	} {
		if !strings.Contains(args, required) {
			t.Fatalf("safe Git arguments missing %q: %q", required, args)
		}
	}
}

func envContains(env []string, key string) bool {
	for _, entry := range env {
		if strings.HasPrefix(entry, key+"=") {
			return true
		}
	}
	return false
}

func envContainsExact(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}

func envKeys(env []string) []string {
	keys := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		keys = append(keys, key)
	}
	return keys
}

func TestCombinedDisablesRepositoryHelpers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX helper script")
	}

	for _, kind := range []string{"fsmonitor", "external diff", "textconv"} {
		t.Run(kind, func(t *testing.T) {
			repo := initRepo(t)
			marker := filepath.Join(t.TempDir(), "helper-ran")
			t.Setenv("PICOGENT_TEST_GITOBS_MARKER", marker)
			helper := writeHelper(t, "#!/bin/sh\n: > \"$PICOGENT_TEST_GITOBS_MARKER\"\n")

			switch kind {
			case "fsmonitor":
				runGit(t, repo, "config", "core.fsmonitor", helper)
				if _, err := Output(context.Background(), repo, "status", "--short"); err != nil {
					t.Fatalf("status: %v", err)
				}
			case "external diff":
				runGit(t, repo, "config", "diff.external", helper)
				writeFile(t, filepath.Join(repo, "content.txt"), "after\n")
				result, err := Combined(context.Background(), repo, "diff")
				if err != nil {
					t.Fatalf("diff: %v (%s)", err, result.Output)
				}
				if !strings.Contains(result.Output, "after") {
					t.Fatalf("ordinary diff output missing changed content: %q", result.Output)
				}
			case "textconv":
				writeFile(t, filepath.Join(repo, ".gitattributes"), "content.txt diff=untrusted\n")
				runGit(t, repo, "add", ".gitattributes")
				runGit(t, repo, "commit", "--quiet", "-m", "attributes")
				runGit(t, repo, "config", "diff.untrusted.textconv", helper)
				writeFile(t, filepath.Join(repo, "content.txt"), "after\n")
				result, err := Combined(context.Background(), repo, "diff")
				if err != nil {
					t.Fatalf("diff: %v (%s)", err, result.Output)
				}
				if !strings.Contains(result.Output, "after") {
					t.Fatalf("ordinary diff output missing changed content: %q", result.Output)
				}
			}

			if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("repository-configured helper ran: %v", err)
			}
		})
	}
}

func TestCombinedDisablesCommitHooks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX hook script")
	}
	repo := initRepo(t)
	marker := filepath.Join(t.TempDir(), "hook-ran")
	t.Setenv("PICOGENT_TEST_GITOBS_MARKER", marker)
	hooks := filepath.Join(repo, "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(hooks, "post-commit"), "#!/bin/sh\n: > \"$PICOGENT_TEST_GITOBS_MARKER\"\n")
	runGit(t, repo, "config", "core.hooksPath", hooks)
	result, err := Combined(context.Background(), repo, "commit", "--allow-empty", "-m", "safe")
	if err != nil {
		t.Fatalf("commit: %v (%s)", err, result.Output)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("post-commit hook ran: %v", err)
	}
}

func TestCombinedBoundsOutput(t *testing.T) {
	repo := initRepo(t)
	large := filepath.Join(repo, "large.txt")
	writeFile(t, large, strings.Repeat("before\n", MaxOutputBytes/len("before\n")+1024))
	runGit(t, repo, "add", "large.txt")
	runGit(t, repo, "commit", "--quiet", "-m", "large")
	writeFile(t, large, strings.Repeat("changed\n", MaxOutputBytes/len("changed\n")+1024))
	result, err := Combined(context.Background(), repo, "diff")
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if !result.Truncated || len(result.Output) > MaxOutputBytes {
		t.Fatalf("bounded result = truncated %v, bytes %d", result.Truncated, len(result.Output))
	}
}

func TestCombinedRedactsSecretsFromGitDiff(t *testing.T) {
	repo := initRepo(t)
	secrets := []string{
		"git-diff-api-secret",
		"git-diff-bearer-secret",
		"git-diff-url-secret",
		"git-diff-key-secret",
	}
	writeFile(t, filepath.Join(repo, "content.txt"), strings.Join([]string{
		"api_key=" + secrets[0],
		"Authorization: Bearer " + secrets[1],
		"https://user:" + secrets[2] + "@example.test/path",
		"-----BEGIN OPENSSH PRIVATE KEY-----" + secrets[3] + "-----END OPENSSH PRIVATE KEY-----",
	}, "\n")+"\n")

	result, err := Combined(context.Background(), repo, "diff", "--", "content.txt")
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	for _, secret := range secrets {
		if strings.Contains(result.Output, secret) {
			t.Fatalf("git diff leaked secret %q: %q", secret, result.Output)
		}
	}
	if !strings.Contains(result.Output, "[REDACTED]") || !strings.Contains(result.Output, "content.txt") {
		t.Fatalf("git diff did not preserve redacted output context: %q", result.Output)
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "config", "user.name", "Picogent Test")
	runGit(t, repo, "config", "user.email", "picogent@example.test")
	writeFile(t, filepath.Join(repo, "content.txt"), "before\n")
	runGit(t, repo, "add", "content.txt")
	runGit(t, repo, "commit", "--quiet", "-m", "initial")
	return repo
}

func writeHelper(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "untrusted-helper")
	writeExecutable(t, path, contents)
	return path
}

func writeExecutable(t *testing.T, path, contents string) {
	t.Helper()
	writeFile(t, path, contents)
	if err := os.Chmod(path, 0o700); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return string(out)
}
