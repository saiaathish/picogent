package benchmark

import (
	"context"
	"strings"
	"testing"
)

func TestOutcomeQualityAgentExecutorUsesAgentTaskstateAndVerificationSeams(t *testing.T) {
	scenario := DefaultOutcomeQualityScenarios()[0]
	input := OutcomeQualityInput{
		Prompt:               "Fix the fixture and verify the result",
		Files:                []OutcomeQualityInputFile{{Path: "nested/fixture.txt", Content: "before\n"}},
		ExpectedChangedPaths: []string{"nested/fixture.txt"},
	}
	normalized, err := normalizeOutcomeQualityInput(input)
	if err != nil {
		t.Fatal(err)
	}
	request := OutcomeQualityExecutionRequest{
		Scenario:    scenario,
		Variant:     OutcomeVariantCandidate,
		Repetition:  1,
		InputSHA256: outcomeQualityInputDigest(normalized),
		Input:       normalized,
		Target:      testOutcomeQualityRunnerConfig(2).Candidate,
		Policy:      testOutcomeQualityRunnerConfig(2).Policy,
	}

	execution, err := NewOutcomeQualityAgentExecutor().Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("execute scripted agent: %v", err)
	}
	if len(execution.Unverified) != 0 {
		t.Fatalf("unexpected unverified reasons: %v", execution.Unverified)
	}
	metrics := execution.Metrics
	if metrics.OutcomeSuccess != OutcomeAssessmentPass || metrics.Correctness != OutcomeAssessmentPass {
		t.Fatalf("success metrics = %#v", metrics)
	}
	if metrics.VerificationQuality != OutcomeVerificationPass || metrics.Evidence != EvidenceCurrent {
		t.Fatalf("verification metrics = %#v", metrics)
	}
	if metrics.ModelCalls != 4 || metrics.ToolCalls != 3 || metrics.Tokens <= 0 {
		t.Fatalf("agent counts = %#v, want four model calls, three tool calls, and tokens", metrics)
	}
	if metrics.UserQuestions != 1 {
		t.Fatalf("permission questions=%d, want one verification permission prompt", metrics.UserQuestions)
	}
	if metrics.ChangedLines != 1 || metrics.UnnecessaryChanges != 0 || metrics.RepairCount != 0 {
		t.Fatalf("change metrics = %#v", metrics)
	}
	if metrics.ContextGrowthBytes <= 0 {
		t.Fatalf("context growth=%d, want a measured positive growth", metrics.ContextGrowthBytes)
	}
}

func TestRunOutcomeQualityMatrixWithScriptedAgent(t *testing.T) {
	cfg := testOutcomeQualityRunnerConfig(2)
	report, err := RunOutcomeQualityMatrix(context.Background(), cfg, NewOutcomeQualityAgentExecutor())
	if err != nil {
		t.Fatalf("run scripted outcome matrix: %v", err)
	}
	if report.Status != OutcomeReportComplete {
		t.Fatalf("report status=%q, want complete; unverified=%v", report.Status, report.Unverified)
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("scripted matrix report: %v", err)
	}
	if len(report.Observations) != len(report.Scenarios)*2*cfg.Policy.Repetitions {
		t.Fatalf("observations=%d, want complete matrix", len(report.Observations))
	}
	for index, observation := range report.Observations {
		if observation.Metrics.OutcomeSuccess != OutcomeAssessmentPass || observation.Metrics.Correctness != OutcomeAssessmentPass {
			t.Fatalf("observation %d did not pass: %#v", index, observation)
		}
		if observation.Metrics.Evidence != EvidenceCurrent || observation.Metrics.VerificationQuality != OutcomeVerificationPass {
			t.Fatalf("observation %d lacks current verification: %#v", index, observation.Metrics)
		}
		if strings.TrimSpace(observation.SourceHead) == "" {
			t.Fatalf("observation %d has no source head", index)
		}
	}
}

func TestOutcomeQualityAgentExecutorRejectsMismatchedInputDigest(t *testing.T) {
	scenario := DefaultOutcomeQualityScenarios()[0]
	input := DefaultOutcomeQualityInput(scenario)
	request := OutcomeQualityExecutionRequest{
		Scenario:    scenario,
		Variant:     OutcomeVariantBaseline,
		Repetition:  1,
		InputSHA256: strings.Repeat("0", 64),
		Input:       input,
		Target:      testOutcomeQualityRunnerConfig(2).Baseline,
		Policy:      testOutcomeQualityRunnerConfig(2).Policy,
	}
	_, err := NewOutcomeQualityAgentExecutor().Execute(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "does not match request input_sha256") {
		t.Fatalf("digest error=%v, want request/input mismatch", err)
	}
}
