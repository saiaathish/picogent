package benchmark

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateOutcomeQualitySourcePairAcceptsDistinctCleanExactHeads(t *testing.T) {
	baselineWorkspace, baselineHead := newOutcomeQualityGitRepo(t, "baseline\n")
	candidateWorkspace, candidateHead := newOutcomeQualityGitRepo(t, "candidate\n")

	err := ValidateOutcomeQualitySourcePair(context.Background(),
		OutcomeQualitySourceBinding{Target: outcomeQualitySourceTarget(baselineHead), Workspace: baselineWorkspace},
		OutcomeQualitySourceBinding{Target: outcomeQualitySourceTarget(candidateHead), Workspace: candidateWorkspace},
	)
	if err != nil {
		t.Fatalf("validate source pair: %v", err)
	}
}

func TestValidateOutcomeQualitySourcePairRejectsSharedWorkspace(t *testing.T) {
	workspace, head := newOutcomeQualityGitRepo(t, "shared\n")
	err := ValidateOutcomeQualitySourcePair(context.Background(),
		OutcomeQualitySourceBinding{Target: outcomeQualitySourceTarget(head), Workspace: workspace},
		OutcomeQualitySourceBinding{Target: outcomeQualitySourceTarget(head), Workspace: workspace},
	)
	if err == nil || !strings.Contains(err.Error(), "distinct workspaces") {
		t.Fatalf("shared workspace error=%v, want distinct-workspace rejection", err)
	}
}

func TestValidateOutcomeQualitySourcePairRejectsDirtyWorkspace(t *testing.T) {
	baselineWorkspace, baselineHead := newOutcomeQualityGitRepo(t, "baseline\n")
	candidateWorkspace, candidateHead := newOutcomeQualityGitRepo(t, "candidate\n")
	if err := os.WriteFile(filepath.Join(baselineWorkspace, "untracked.txt"), []byte("dirty\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := ValidateOutcomeQualitySourcePair(context.Background(),
		OutcomeQualitySourceBinding{Target: outcomeQualitySourceTarget(baselineHead), Workspace: baselineWorkspace},
		OutcomeQualitySourceBinding{Target: outcomeQualitySourceTarget(candidateHead), Workspace: candidateWorkspace},
	)
	if err == nil || !strings.Contains(err.Error(), "worktree is not clean") {
		t.Fatalf("dirty workspace error=%v, want clean-tree rejection", err)
	}
}

func TestValidateOutcomeQualitySourcePairRejectsIgnoredWorkspaceFile(t *testing.T) {
	baselineWorkspace, baselineHead := newOutcomeQualityGitRepo(t, "baseline\n")
	if err := os.WriteFile(filepath.Join(baselineWorkspace, ".gitignore"), []byte("ignored.txt\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runOutcomeQualityTestGit(t, baselineWorkspace, "add", "--", ".gitignore")
	runOutcomeQualityTestGit(t, baselineWorkspace, "commit", "-m", "ignore fixture")
	baselineHead = runOutcomeQualityTestGit(t, baselineWorkspace, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(baselineWorkspace, "ignored.txt"), []byte("ignored\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	candidateWorkspace, candidateHead := newOutcomeQualityGitRepo(t, "candidate\n")

	err := ValidateOutcomeQualitySourcePair(context.Background(),
		OutcomeQualitySourceBinding{Target: outcomeQualitySourceTarget(baselineHead), Workspace: baselineWorkspace},
		OutcomeQualitySourceBinding{Target: outcomeQualitySourceTarget(candidateHead), Workspace: candidateWorkspace},
	)
	if err == nil || !strings.Contains(err.Error(), "worktree is not clean") {
		t.Fatalf("ignored file error=%v, want clean-tree rejection", err)
	}
}

func TestValidateOutcomeQualitySourcePairRejectsStaleDeclaredHead(t *testing.T) {
	baselineWorkspace, _ := newOutcomeQualityGitRepo(t, "baseline\n")
	candidateWorkspace, candidateHead := newOutcomeQualityGitRepo(t, "candidate\n")
	staleHead := strings.Repeat("a", 40)

	err := ValidateOutcomeQualitySourcePair(context.Background(),
		OutcomeQualitySourceBinding{Target: outcomeQualitySourceTarget(staleHead), Workspace: baselineWorkspace},
		OutcomeQualitySourceBinding{Target: outcomeQualitySourceTarget(candidateHead), Workspace: candidateWorkspace},
	)
	if err == nil || !strings.Contains(err.Error(), "declared clean head") {
		t.Fatalf("stale-head error=%v, want exact-head rejection", err)
	}
}

func outcomeQualitySourceTarget(head string) OutcomeQualityTarget {
	return OutcomeQualityTarget{
		SourceHead:  head,
		Host:        "test-host",
		GoVersion:   "go1.25.0",
		ToolVersion: OutcomeQualityRunnerToolVersion,
	}
}

func newOutcomeQualityGitRepo(t *testing.T, content string) (string, string) {
	t.Helper()
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "fixture.txt"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	runOutcomeQualityTestGit(t, workspace, "init")
	runOutcomeQualityTestGit(t, workspace, "config", "user.email", "picogent-test@example.invalid")
	runOutcomeQualityTestGit(t, workspace, "config", "user.name", "Picogent Test")
	runOutcomeQualityTestGit(t, workspace, "add", "--", "fixture.txt")
	runOutcomeQualityTestGit(t, workspace, "commit", "-m", "fixture")
	return workspace, runOutcomeQualityTestGit(t, workspace, "rev-parse", "HEAD")
}

func runOutcomeQualityTestGit(t *testing.T, workspace string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = workspace
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}
