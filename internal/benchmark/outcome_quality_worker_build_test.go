package benchmark

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildOutcomeQualityWorkerUsesValidatedSourceAndExternalOutput(t *testing.T) {
	root := cleanOutcomeQualitySourceClone(t)
	head := outcomeQualityGitHead(t, root)
	build, err := BuildOutcomeQualityWorker(context.Background(), OutcomeQualitySourceBinding{
		Target:    outcomeQualitySourceTarget(head),
		Workspace: root,
	}, t.TempDir())
	if err != nil {
		t.Fatalf("build worker: %v", err)
	}
	defer build.Close()
	executor := build.ProcessExecutor()
	if executor == nil || executor.Command == "" {
		t.Fatal("build returned no process executor")
	}
	if outcomeQualityPathWithin(executor.Command, root) {
		t.Fatalf("worker binary %q is inside source workspace %q", executor.Command, root)
	}
	if _, err := os.Stat(executor.Command); err != nil {
		t.Fatalf("built worker: %v", err)
	}

	request := outcomeQualityWorkerTestRequest(t)
	request.Target = outcomeQualitySourceTarget(head)
	execution, err := executor.Execute(context.Background(), requestToOutcomeQualityExecutionRequest(request))
	if err != nil {
		t.Fatalf("execute built worker: %v", err)
	}
	if execution.Metrics.OutcomeSuccess != OutcomeAssessmentPass || execution.Metrics.Evidence != EvidenceCurrent {
		t.Fatalf("built worker execution=%#v", execution)
	}
}

func TestBuildOutcomeQualityWorkerRejectsTempParentInsideSource(t *testing.T) {
	root := cleanOutcomeQualitySourceClone(t)
	head := outcomeQualityGitHead(t, root)
	_, err := BuildOutcomeQualityWorker(context.Background(), OutcomeQualitySourceBinding{
		Target:    outcomeQualitySourceTarget(head),
		Workspace: root,
	}, root)
	if err == nil || !strings.Contains(err.Error(), "outside source workspace") {
		t.Fatalf("temp-parent error=%v, want outside-workspace rejection", err)
	}
}

func TestBuildOutcomeQualityWorkerReportsMissingAdapter(t *testing.T) {
	root, head := newOutcomeQualityGitRepo(t, "legacy\n")
	_, err := BuildOutcomeQualityWorker(context.Background(), OutcomeQualitySourceBinding{
		Target:    outcomeQualitySourceTarget(head),
		Workspace: root,
	}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "build outcome-quality worker") {
		t.Fatalf("missing-adapter error=%v, want bounded build failure", err)
	}
}

func outcomeQualityModuleRoot(t *testing.T) string {
	t.Helper()
	working, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := working
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatal("could not find module root")
		}
		root = parent
	}
}

func cleanOutcomeQualitySourceClone(t *testing.T) string {
	t.Helper()
	root := outcomeQualityModuleRoot(t)
	clone := filepath.Join(t.TempDir(), "source")
	command := exec.Command("git", "clone", "--quiet", "--no-hardlinks", root, clone)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("clone clean source: %v\n%s", err, output)
	}
	t.Cleanup(func() { _ = os.RemoveAll(clone) })
	return clone
}

func outcomeQualityGitHead(t *testing.T, root string) string {
	t.Helper()
	output, err := exec.Command("git", "-C", root, "rev-parse", "--verify", "HEAD").Output()
	if err != nil {
		t.Fatalf("git head: %v", err)
	}
	return strings.TrimSpace(string(output))
}
