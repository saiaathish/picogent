package benchmark

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunOutcomeQualitySourcePairMatrixRoutesEachVariantToItsBoundExecutor(t *testing.T) {
	baselineWorkspace, baselineHead := newOutcomeQualityGitRepo(t, "baseline\n")
	candidateWorkspace, candidateHead := newOutcomeQualityGitRepo(t, "candidate\n")
	baselineTarget := outcomeQualitySourceTarget(baselineHead)
	candidateTarget := outcomeQualitySourceTarget(candidateHead)
	baseline := &recordingOutcomeQualityExecutor{binding: OutcomeQualitySourceBinding{Target: baselineTarget, Workspace: baselineWorkspace}}
	candidate := &recordingOutcomeQualityExecutor{binding: OutcomeQualitySourceBinding{Target: candidateTarget, Workspace: candidateWorkspace}}
	report, err := RunOutcomeQualitySourcePairMatrix(context.Background(), OutcomeQualitySourcePairConfig{
		Baseline:  OutcomeQualitySourceBinding{Target: baselineTarget, Workspace: baselineWorkspace},
		Candidate: OutcomeQualitySourceBinding{Target: candidateTarget, Workspace: candidateWorkspace},
		Policy:    testOutcomeQualityRunnerConfig(2).Policy,
		Command:   "outcome-quality-source-pair-test",
	}, baseline, candidate)
	if err != nil {
		t.Fatalf("run source-pair matrix: %v", err)
	}
	if report.Status != OutcomeReportComplete {
		t.Fatalf("report status=%q, want complete", report.Status)
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("validate source-pair report: %v", err)
	}
	wantCalls := len(DefaultOutcomeQualityScenarios()) * 2
	if len(baseline.requests) != wantCalls || len(candidate.requests) != wantCalls {
		t.Fatalf("baseline requests=%d candidate requests=%d, want %d each", len(baseline.requests), len(candidate.requests), wantCalls)
	}
	for _, request := range baseline.requests {
		if request.Variant != OutcomeVariantBaseline || request.Target != baselineTarget {
			t.Fatalf("baseline received %#v", request)
		}
	}
	for _, request := range candidate.requests {
		if request.Variant != OutcomeVariantCandidate || request.Target != candidateTarget {
			t.Fatalf("candidate received %#v", request)
		}
	}
	for index, observation := range report.Observations {
		wantVariant := OutcomeVariantBaseline
		if index%(2*testOutcomeQualityRunnerConfig(2).Policy.Repetitions) >= testOutcomeQualityRunnerConfig(2).Policy.Repetitions {
			wantVariant = OutcomeVariantCandidate
		}
		if observation.Variant != wantVariant {
			t.Fatalf("observation %d variant=%q, want %q", index, observation.Variant, wantVariant)
		}
	}
}

func TestRunOutcomeQualitySourcePairMatrixPreflightsBeforeDelegating(t *testing.T) {
	baselineWorkspace, baselineHead := newOutcomeQualityGitRepo(t, "baseline\n")
	candidateWorkspace, candidateHead := newOutcomeQualityGitRepo(t, "candidate\n")
	if err := writeOutcomeQualityDirtyFile(baselineWorkspace); err != nil {
		t.Fatal(err)
	}
	baseline := &recordingOutcomeQualityExecutor{binding: OutcomeQualitySourceBinding{Target: outcomeQualitySourceTarget(baselineHead), Workspace: baselineWorkspace}}
	candidate := &recordingOutcomeQualityExecutor{binding: OutcomeQualitySourceBinding{Target: outcomeQualitySourceTarget(candidateHead), Workspace: candidateWorkspace}}
	_, err := RunOutcomeQualitySourcePairMatrix(context.Background(), OutcomeQualitySourcePairConfig{
		Baseline:  OutcomeQualitySourceBinding{Target: outcomeQualitySourceTarget(baselineHead), Workspace: baselineWorkspace},
		Candidate: OutcomeQualitySourceBinding{Target: outcomeQualitySourceTarget(candidateHead), Workspace: candidateWorkspace},
		Policy:    testOutcomeQualityRunnerConfig(2).Policy,
		Command:   "outcome-quality-source-pair-test",
	}, baseline, candidate)
	if err == nil || !strings.Contains(err.Error(), "worktree is not clean") {
		t.Fatalf("preflight error=%v, want dirty-worktree rejection", err)
	}
	if len(baseline.requests) != 0 || len(candidate.requests) != 0 {
		t.Fatalf("executors were called during failed preflight: baseline=%d candidate=%d", len(baseline.requests), len(candidate.requests))
	}
}

func TestRunOutcomeQualitySourcePairMatrixRequiresBothExecutors(t *testing.T) {
	baselineWorkspace, baselineHead := newOutcomeQualityGitRepo(t, "baseline\n")
	candidateWorkspace, candidateHead := newOutcomeQualityGitRepo(t, "candidate\n")
	cfg := OutcomeQualitySourcePairConfig{
		Baseline:  OutcomeQualitySourceBinding{Target: outcomeQualitySourceTarget(baselineHead), Workspace: baselineWorkspace},
		Candidate: OutcomeQualitySourceBinding{Target: outcomeQualitySourceTarget(candidateHead), Workspace: candidateWorkspace},
		Policy:    testOutcomeQualityRunnerConfig(2).Policy,
		Command:   "outcome-quality-source-pair-test",
	}
	_, err := RunOutcomeQualitySourcePairMatrix(context.Background(), cfg, &recordingOutcomeQualityExecutor{binding: cfg.Baseline}, nil)
	if err == nil || !strings.Contains(err.Error(), "candidate executor is required") {
		t.Fatalf("missing candidate error=%v", err)
	}
}

type recordingOutcomeQualityExecutor struct {
	binding  OutcomeQualitySourceBinding
	requests []OutcomeQualityExecutionRequest
}

func (e *recordingOutcomeQualityExecutor) outcomeQualitySourceBinding() OutcomeQualitySourceBinding {
	if e == nil {
		return OutcomeQualitySourceBinding{}
	}
	return e.binding
}

func (e *recordingOutcomeQualityExecutor) validateOutcomeQualitySource(ctx context.Context) error {
	if e == nil {
		return fmt.Errorf("recording outcome-quality executor is nil")
	}
	workspace, err := canonicalOutcomeQualityWorkspace(e.binding.Workspace)
	if err != nil {
		return err
	}
	return validateOutcomeQualitySourceBinding(ctx, "recording", e.binding.Target, workspace)
}

func writeOutcomeQualityDirtyFile(root string) error {
	return os.WriteFile(filepath.Join(root, "dirty.txt"), []byte("dirty\n"), 0o600)
}

func (e *recordingOutcomeQualityExecutor) Execute(_ context.Context, request OutcomeQualityExecutionRequest) (OutcomeQualityExecution, error) {
	e.requests = append(e.requests, request)
	return OutcomeQualityExecution{Metrics: passingOutcomeQualityMetrics()}, nil
}
