package benchmark

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

const outcomeQualityWorkerChildEnv = "PICOGENT_OUTCOME_QUALITY_WORKER_CHILD"

func TestRunOutcomeQualityWorkerRoundTrip(t *testing.T) {
	request := outcomeQualityWorkerTestRequest(t)
	var output bytes.Buffer
	var seen OutcomeQualityExecutionRequest
	err := RunOutcomeQualityWorker(context.Background(), bytes.NewReader(mustMarshalOutcomeQualityWorkerRequest(t, request)), &output, OutcomeQualityExecutorFunc(func(_ context.Context, got OutcomeQualityExecutionRequest) (OutcomeQualityExecution, error) {
		seen = got
		return OutcomeQualityExecution{Metrics: passingOutcomeQualityMetrics()}, nil
	}))
	if err != nil {
		t.Fatalf("worker round trip: %v", err)
	}
	if seen.InputSHA256 != request.InputSHA256 || seen.Input.Files[0].Content != request.Input.Files[0].Content {
		t.Fatalf("worker normalized request = %#v", seen)
	}
	response, err := decodeOutcomeQualityWorkerResponse(bytes.NewReader(output.Bytes()))
	if err != nil {
		t.Fatalf("decode worker response: %v", err)
	}
	if response.SourceHead != request.Target.SourceHead || response.Metrics.OutcomeSuccess != OutcomeAssessmentPass {
		t.Fatalf("worker response = %#v", response)
	}
}

func TestRunOutcomeQualityWorkerRejectsUnknownAndTrailingJSON(t *testing.T) {
	request := outcomeQualityWorkerTestRequest(t)
	payload := mustMarshalOutcomeQualityWorkerRequest(t, request)
	unknown := append(append([]byte(nil), payload[:len(payload)-1]...), []byte(`,"unknown":true}`)...)
	trailing := append(append([]byte(nil), payload...), []byte(`{}`)...)
	for name, input := range map[string][]byte{"unknown field": unknown, "trailing value": trailing} {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			err := RunOutcomeQualityWorker(context.Background(), bytes.NewReader(input), &output, OutcomeQualityExecutorFunc(func(context.Context, OutcomeQualityExecutionRequest) (OutcomeQualityExecution, error) {
				t.Fatal("executor ran for malformed protocol input")
				return OutcomeQualityExecution{}, nil
			}))
			if err == nil {
				t.Fatal("malformed protocol input unexpectedly passed")
			}
			if output.Len() != 0 {
				t.Fatalf("malformed protocol output=%q, want empty", output.Bytes())
			}
		})
	}
}

func TestOutcomeQualityProcessExecutorRoundTrip(t *testing.T) {
	workspace, head := newOutcomeQualityGitRepo(t, "worker\n")
	request := outcomeQualityWorkerTestRequest(t)
	request.Target = outcomeQualitySourceTarget(head)
	executor := &OutcomeQualityProcessExecutor{
		Command: os.Args[0],
		Args:    []string{"-test.run", "^TestOutcomeQualityWorkerChild$", "-test.count=1"},
		Binding: OutcomeQualitySourceBinding{Target: request.Target, Workspace: workspace},
	}
	t.Setenv(outcomeQualityWorkerChildEnv, "1")
	execution, err := executor.Execute(context.Background(), requestToOutcomeQualityExecutionRequest(request))
	if err != nil {
		t.Fatalf("process executor: %v", err)
	}
	if execution.Metrics.OutcomeSuccess != OutcomeAssessmentPass || execution.Metrics.Evidence != EvidenceCurrent {
		t.Fatalf("process execution = %#v", execution)
	}
}

func TestOutcomeQualityProcessExecutorRejectsBindingTargetMismatch(t *testing.T) {
	workspace, head := newOutcomeQualityGitRepo(t, "worker\n")
	request := outcomeQualityWorkerTestRequest(t)
	request.Target = outcomeQualitySourceTarget(strings.Repeat("b", 40))
	executor := &OutcomeQualityProcessExecutor{
		Command: os.Args[0],
		Args:    []string{"-test.run", "^TestOutcomeQualityWorkerChild$"},
		Binding: OutcomeQualitySourceBinding{Target: outcomeQualitySourceTarget(head), Workspace: workspace},
	}

	_, err := executor.Execute(context.Background(), requestToOutcomeQualityExecutionRequest(request))
	if err == nil || !strings.Contains(err.Error(), "binding and request target must match") {
		t.Fatalf("target mismatch error=%v, want binding/request rejection", err)
	}
}

func TestOutcomeQualityWorkerChild(t *testing.T) {
	if os.Getenv(outcomeQualityWorkerChildEnv) != "1" {
		return
	}
	err := RunOutcomeQualityWorker(context.Background(), os.Stdin, os.Stdout, OutcomeQualityExecutorFunc(func(_ context.Context, _ OutcomeQualityExecutionRequest) (OutcomeQualityExecution, error) {
		return OutcomeQualityExecution{Metrics: passingOutcomeQualityMetrics()}, nil
	}))
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func outcomeQualityWorkerTestRequest(t *testing.T) OutcomeQualityWorkerRequest {
	t.Helper()
	scenario := DefaultOutcomeQualityScenarios()[0]
	input := DefaultOutcomeQualityInput(scenario)
	normalized, err := normalizeOutcomeQualityInput(input)
	if err != nil {
		t.Fatal(err)
	}
	scenario.InputSHA256 = outcomeQualityInputDigest(normalized)
	return OutcomeQualityWorkerRequest{
		Protocol:    OutcomeQualityWorkerProtocol,
		Scenario:    scenario,
		Variant:     OutcomeVariantCandidate,
		Repetition:  1,
		InputSHA256: scenario.InputSHA256,
		Input:       normalized,
		Target:      testOutcomeQualityRunnerConfig(2).Candidate,
		Policy:      testOutcomeQualityRunnerConfig(2).Policy,
	}
}

func requestToOutcomeQualityExecutionRequest(request OutcomeQualityWorkerRequest) OutcomeQualityExecutionRequest {
	return OutcomeQualityExecutionRequest{
		Scenario:    request.Scenario,
		Variant:     request.Variant,
		Repetition:  request.Repetition,
		InputSHA256: request.InputSHA256,
		Input:       request.Input,
		Target:      request.Target,
		Policy:      request.Policy,
	}
}

func mustMarshalOutcomeQualityWorkerRequest(t *testing.T, request OutcomeQualityWorkerRequest) []byte {
	t.Helper()
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
