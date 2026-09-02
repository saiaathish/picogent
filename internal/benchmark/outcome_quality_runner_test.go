package benchmark

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestRunOutcomeQualityMatrixBindsSharedInputsAndOrdering(t *testing.T) {
	const repetitions = 2
	cfg := testOutcomeQualityRunnerConfig(repetitions)
	seen := make([]OutcomeQualityExecutionRequest, 0, len(DefaultOutcomeQualityScenarios())*2*repetitions)
	report, err := RunOutcomeQualityMatrix(context.Background(), cfg, OutcomeQualityExecutorFunc(func(_ context.Context, request OutcomeQualityExecutionRequest) (OutcomeQualityExecution, error) {
		seen = append(seen, request)
		return OutcomeQualityExecution{Metrics: passingOutcomeQualityMetrics()}, nil
	}))
	if err != nil {
		t.Fatalf("run matrix: %v", err)
	}
	if report.Status != OutcomeReportComplete {
		t.Fatalf("report status=%q, want complete", report.Status)
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("validate matrix report: %v", err)
	}
	expected := len(DefaultOutcomeQualityScenarios()) * 2 * repetitions
	if len(seen) != expected || len(report.Observations) != expected {
		t.Fatalf("requests=%d observations=%d, want %d", len(seen), len(report.Observations), expected)
	}
	for index, request := range seen {
		observation := report.Observations[index]
		if request.Scenario.ID != observation.ScenarioID || request.Variant != observation.Variant || request.Repetition != observation.Repetition {
			t.Fatalf("request/observation %d mismatch: request=%#v observation=%#v", index, request, observation)
		}
		if request.InputSHA256 != report.Scenarios[index/(2*repetitions)].InputSHA256 {
			t.Fatalf("request %d input digest=%q does not match scenario digest", index, request.InputSHA256)
		}
		if request.Input.Prompt == "" || len(request.Input.Files) == 0 {
			t.Fatalf("request %d did not receive bounded fixture input: %#v", index, request.Input)
		}
	}
	for scenarioIndex, scenario := range report.Scenarios {
		first := seen[scenarioIndex*2*repetitions]
		second := seen[(scenarioIndex*2+1)*repetitions]
		if first.InputSHA256 != second.InputSHA256 || first.InputSHA256 != scenario.InputSHA256 {
			t.Fatalf("scenario %d input was not shared across variants", scenarioIndex)
		}
	}
}

func TestRunOutcomeQualityMatrixDowngradesInvalidExecution(t *testing.T) {
	report, err := RunOutcomeQualityMatrix(context.Background(), testOutcomeQualityRunnerConfig(2), OutcomeQualityExecutorFunc(func(_ context.Context, _ OutcomeQualityExecutionRequest) (OutcomeQualityExecution, error) {
		metrics := passingOutcomeQualityMetrics()
		metrics.Evidence = EvidenceStale
		return OutcomeQualityExecution{Metrics: metrics}, nil
	}))
	if err != nil {
		t.Fatalf("run matrix with invalid execution: %v", err)
	}
	if report.Status != OutcomeReportInconclusive || len(report.Unverified) == 0 {
		t.Fatalf("invalid execution report=%#v, want inconclusive reason", report)
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("downgraded report must remain valid: %v", err)
	}
	for index, observation := range report.Observations {
		if observation.Metrics.OutcomeSuccess != OutcomeAssessmentInconclusive || observation.Metrics.Evidence != EvidenceUnverified {
			t.Fatalf("observation %d was not fail-closed: %#v", index, observation.Metrics)
		}
	}
}

func TestRunOutcomeQualityMatrixRecordsExecutorFailureWithoutPassingIt(t *testing.T) {
	failed := true
	report, err := RunOutcomeQualityMatrix(context.Background(), testOutcomeQualityRunnerConfig(2), OutcomeQualityExecutorFunc(func(_ context.Context, _ OutcomeQualityExecutionRequest) (OutcomeQualityExecution, error) {
		if failed {
			failed = false
			return OutcomeQualityExecution{}, fmt.Errorf("scripted fixture unavailable")
		}
		return OutcomeQualityExecution{Metrics: passingOutcomeQualityMetrics()}, nil
	}))
	if err != nil {
		t.Fatalf("run matrix with one failed execution: %v", err)
	}
	if report.Status != OutcomeReportInconclusive || len(report.Unverified) == 0 {
		t.Fatalf("failure report=%#v, want inconclusive reason", report)
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("failure report: %v", err)
	}
	if report.Observations[0].Metrics.OutcomeSuccess == OutcomeAssessmentPass {
		t.Fatal("failed execution was recorded as passing")
	}
}

func TestRunOutcomeQualityMatrixStopsOnCancellationWithValidPartialReport(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	report, err := RunOutcomeQualityMatrix(ctx, testOutcomeQualityRunnerConfig(2), OutcomeQualityExecutorFunc(func(_ context.Context, _ OutcomeQualityExecutionRequest) (OutcomeQualityExecution, error) {
		cancel()
		return OutcomeQualityExecution{Metrics: passingOutcomeQualityMetrics()}, nil
	}))
	if err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("cancellation error=%v, want context cancellation", err)
	}
	if report.Status != OutcomeReportInconclusive || len(report.Observations) != 1 || len(report.Unverified) == 0 {
		t.Fatalf("cancellation report=%#v, want one valid partial observation", report)
	}
	if validateErr := report.Validate(); validateErr != nil {
		t.Fatalf("partial cancellation report: %v", validateErr)
	}
}

func TestRunOutcomeQualityMatrixRejectsInvalidConfiguration(t *testing.T) {
	cases := []struct {
		name string
		edit func(*OutcomeQualityRunnerConfig)
		want string
	}{
		{name: "same heads", edit: func(cfg *OutcomeQualityRunnerConfig) { cfg.Candidate.SourceHead = cfg.Baseline.SourceHead }, want: "different source heads"},
		{name: "different host", edit: func(cfg *OutcomeQualityRunnerConfig) { cfg.Candidate.Host = "linux/amd64" }, want: "share host"},
		{name: "one repetition", edit: func(cfg *OutcomeQualityRunnerConfig) { cfg.Policy.Repetitions = 1 }, want: "repetitions"},
		{name: "unsafe fixture", edit: func(cfg *OutcomeQualityRunnerConfig) {
			cfg.ScenarioInput = func(OutcomeQualityScenario) (OutcomeQualityInput, error) {
				return OutcomeQualityInput{Prompt: "finish", Files: []OutcomeQualityInputFile{{Path: "../outside", Content: "x"}}}, nil
			}
		}, want: "outside"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testOutcomeQualityRunnerConfig(2)
			tc.edit(&cfg)
			_, err := RunOutcomeQualityMatrix(context.Background(), cfg, OutcomeQualityExecutorFunc(func(context.Context, OutcomeQualityExecutionRequest) (OutcomeQualityExecution, error) {
				return OutcomeQualityExecution{Metrics: passingOutcomeQualityMetrics()}, nil
			}))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("configuration error=%v, want substring %q", err, tc.want)
			}
		})
	}
}

func testOutcomeQualityRunnerConfig(repetitions int) OutcomeQualityRunnerConfig {
	return OutcomeQualityRunnerConfig{
		Baseline: OutcomeQualityTarget{
			SourceHead:  strings.Repeat("a", 40),
			Host:        "darwin/arm64",
			GoVersion:   "go1.25.0",
			ToolVersion: OutcomeQualityRunnerToolVersion,
		},
		Candidate: OutcomeQualityTarget{
			SourceHead:  strings.Repeat("b", 40),
			Host:        "darwin/arm64",
			GoVersion:   "go1.25.0",
			ToolVersion: OutcomeQualityRunnerToolVersion,
		},
		Policy: OutcomeQualityPolicy{
			Repetitions:   repetitions,
			TimeoutMillis: 30_000,
			MaxTokens:     16_000,
			MaxModelCalls: 64,
			MaxToolCalls:  256,
			MaxTurns:      32,
		},
		Command: "go test ./internal/benchmark -run '^TestOutcomeQuality' -count=2",
	}
}

func passingOutcomeQualityMetrics() OutcomeQualityMetrics {
	return OutcomeQualityMetrics{
		OutcomeSuccess:      OutcomeAssessmentPass,
		Correctness:         OutcomeAssessmentPass,
		VerificationQuality: OutcomeVerificationPass,
		Evidence:            EvidenceCurrent,
	}
}
