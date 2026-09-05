package benchmark

import (
	"context"
	"testing"
)

func TestOutcomeQualityAgentExecutorThreeFileCapture(t *testing.T) {
	scenario := DefaultOutcomeQualityScenarios()[0]
	input, err := normalizeOutcomeQualityInput(outcomeQualityLegacyInput(scenario))
	if err != nil {
		t.Fatal(err)
	}
	config := testOutcomeQualityRunnerConfig(2)
	request := OutcomeQualityExecutionRequest{
		Scenario:    scenario,
		Variant:     OutcomeVariantCandidate,
		Repetition:  1,
		InputSHA256: outcomeQualityInputDigest(input),
		Input:       input,
		Target:      config.Candidate,
		Policy:      config.Policy,
	}

	execution, err := NewOutcomeQualityAgentExecutor().Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("execute three-file scripted agent: %v", err)
	}
	if len(execution.Unverified) != 0 {
		t.Fatalf("three-file proof unavailable: metrics=%#v reasons=%v", execution.Metrics, execution.Unverified)
	}
	if execution.Metrics.Evidence != EvidenceCurrent || execution.Metrics.VerificationQuality != OutcomeVerificationPass {
		t.Fatalf("three-file proof metrics=%#v", execution.Metrics)
	}
}
