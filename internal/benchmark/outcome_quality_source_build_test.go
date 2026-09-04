package benchmark

import (
	"context"
	"strings"
	"testing"
)

func TestBuildOutcomeQualitySourcePairPreflightsBeforeBuilding(t *testing.T) {
	baselineWorkspace, baselineHead := newOutcomeQualityGitRepo(t, "baseline\n")
	candidateWorkspace, candidateHead := newOutcomeQualityGitRepo(t, "candidate\n")
	if err := writeOutcomeQualityDirtyFile(baselineWorkspace); err != nil {
		t.Fatal(err)
	}

	_, err := BuildOutcomeQualitySourcePair(context.Background(), OutcomeQualitySourcePairBuildConfig{
		Baseline:          OutcomeQualitySourceBinding{Target: outcomeQualitySourceTarget(baselineHead), Workspace: baselineWorkspace},
		Candidate:         OutcomeQualitySourceBinding{Target: outcomeQualitySourceTarget(candidateHead), Workspace: candidateWorkspace},
		TempParent:        t.TempDir(),
		LegacyProviderURL: "http://127.0.0.1:1",
	})
	if err == nil || !strings.Contains(err.Error(), "source-pair build preflight") || !strings.Contains(err.Error(), "worktree is not clean") {
		t.Fatalf("preflight error=%v, want clean-worktree rejection before build", err)
	}
}

func TestOutcomeQualitySourcePairBuildRejectsClosedOrNilBuild(t *testing.T) {
	var nilBuild *OutcomeQualitySourcePairBuild
	if _, err := nilBuild.RunMatrix(context.Background(), OutcomeQualitySourcePairConfig{}); err == nil || !strings.Contains(err.Error(), "build is nil") {
		t.Fatalf("nil build error=%v, want nil-build rejection", err)
	}
	if err := nilBuild.Close(); err != nil {
		t.Fatalf("nil build close: %v", err)
	}

	closed := &OutcomeQualitySourcePairBuild{}
	if _, err := closed.RunMatrix(context.Background(), OutcomeQualitySourcePairConfig{}); err == nil || !strings.Contains(err.Error(), "build is closed") {
		t.Fatalf("closed build error=%v, want closed-build rejection", err)
	}
	if err := closed.Close(); err != nil {
		t.Fatalf("closed build close: %v", err)
	}
	if closed.BaselineExecutor() != nil || closed.CandidateExecutor() != nil {
		t.Fatal("closed build returned an executor")
	}
}
