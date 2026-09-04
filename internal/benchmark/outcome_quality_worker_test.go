package benchmark

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
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

func TestRunOutcomeQualityWorkerRejectsSourceHeadMismatch(t *testing.T) {
	request := outcomeQualityWorkerTestRequest(t)
	request.Target.SourceHead = strings.Repeat("b", 40)
	var output bytes.Buffer
	err := RunOutcomeQualityWorker(context.Background(), bytes.NewReader(mustMarshalOutcomeQualityWorkerRequest(t, request)), &output, OutcomeQualityExecutorFunc(func(context.Context, OutcomeQualityExecutionRequest) (OutcomeQualityExecution, error) {
		t.Fatal("executor ran for a source-head mismatch")
		return OutcomeQualityExecution{}, nil
	}))
	if err == nil || !strings.Contains(err.Error(), "source head does not match target") {
		t.Fatalf("source-head mismatch error=%v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("source-head mismatch output=%q, want empty", output.Bytes())
	}
}

func TestRunOutcomeQualityWorkerRequestReadHonorsCancellation(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- RunOutcomeQualityWorker(ctx, reader, io.Discard, OutcomeQualityExecutorFunc(func(context.Context, OutcomeQualityExecutionRequest) (OutcomeQualityExecution, error) {
			return OutcomeQualityExecution{}, fmt.Errorf("executor ran for a canceled request")
		}))
	}()
	cancel()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "context canceled") {
			t.Fatalf("canceled request error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled request did not stop reading")
	}
}

func TestRunOutcomeQualityWorkerDowngradesUnverifiedPassingMetrics(t *testing.T) {
	request := outcomeQualityWorkerTestRequest(t)
	var output bytes.Buffer
	err := RunOutcomeQualityWorker(context.Background(), bytes.NewReader(mustMarshalOutcomeQualityWorkerRequest(t, request)), &output, OutcomeQualityExecutorFunc(func(context.Context, OutcomeQualityExecutionRequest) (OutcomeQualityExecution, error) {
		return OutcomeQualityExecution{
			Metrics:    passingOutcomeQualityMetrics(),
			Unverified: []string{"verification was not recorded"},
		}, nil
	}))
	if err != nil {
		t.Fatalf("unverified worker result: %v", err)
	}
	response, err := decodeOutcomeQualityWorkerResponse(bytes.NewReader(output.Bytes()))
	if err != nil {
		t.Fatalf("decode unverified worker result: %v", err)
	}
	if response.Metrics.OutcomeSuccess != OutcomeAssessmentInconclusive || response.Metrics.Correctness != OutcomeAssessmentInconclusive || response.Metrics.Evidence != EvidenceUnverified {
		t.Fatalf("unverified worker metrics=%#v, want inconclusive", response.Metrics)
	}
	if len(response.Unverified) != 1 || response.Unverified[0] != "verification was not recorded" {
		t.Fatalf("unverified reasons=%v", response.Unverified)
	}
}

func TestDecodeOutcomeQualityWorkerResponseRejectsTrailingAndOversizedData(t *testing.T) {
	request := outcomeQualityWorkerTestRequest(t)
	response := OutcomeQualityWorkerResponse{
		Protocol:   OutcomeQualityWorkerProtocol,
		SourceHead: request.Target.SourceHead,
		Metrics:    passingOutcomeQualityMetrics(),
	}
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string][]byte{
		"trailing value": append(append([]byte(nil), payload...), []byte(`{}`)...),
		"oversized":      bytes.Repeat([]byte("x"), maxOutcomeQualityWorkerResponseBytes+1),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeOutcomeQualityWorkerResponse(bytes.NewReader(input)); err == nil {
				t.Fatal("malformed response unexpectedly decoded")
			}
		})
	}
}

func TestEncodeOutcomeQualityWorkerResponseRejectsShortWriter(t *testing.T) {
	request := outcomeQualityWorkerTestRequest(t)
	err := encodeOutcomeQualityWorkerResponse(shortOutcomeQualityWorkerWriter{}, OutcomeQualityWorkerResponse{
		Protocol:   OutcomeQualityWorkerProtocol,
		SourceHead: request.Target.SourceHead,
		Metrics:    passingOutcomeQualityMetrics(),
	})
	if err == nil || !strings.Contains(err.Error(), "short write") {
		t.Fatalf("short writer error=%v", err)
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

func TestOutcomeQualityProcessExecutorRejectsRelativeCommand(t *testing.T) {
	request := outcomeQualityWorkerTestRequest(t)
	executor := &OutcomeQualityProcessExecutor{Command: "outcome-quality-worker"}
	_, err := executor.Execute(context.Background(), requestToOutcomeQualityExecutionRequest(request))
	if err == nil || !strings.Contains(err.Error(), "absolute path") {
		t.Fatalf("relative command error=%v, want absolute-path rejection", err)
	}
}

func TestOutcomeQualityProcessExecutorUsesMinimalEnvironment(t *testing.T) {
	workspace, head := newOutcomeQualityGitRepo(t, "worker\n")
	request := outcomeQualityWorkerTestRequest(t)
	request.Target = outcomeQualitySourceTarget(head)
	executor := &OutcomeQualityProcessExecutor{
		Command: os.Args[0],
		Args:    []string{"-test.run", "^TestOutcomeQualityWorkerChild$", "-test.count=1"},
		Binding: OutcomeQualitySourceBinding{Target: request.Target, Workspace: workspace},
	}
	for _, key := range []string{"PICOGENT_HOME", "PICOGENT_CODEX_HOME", "CODEX_HOME", "OLLAMA_HOST"} {
		t.Setenv(key, "/untrusted/worker-setting")
	}
	t.Setenv(outcomeQualityWorkerChildEnv, "env")
	if _, err := executor.Execute(context.Background(), requestToOutcomeQualityExecutionRequest(request)); err != nil {
		t.Fatalf("minimal worker environment: %v", err)
	}
}

func TestOutcomeQualityProcessExecutorKillsDescendantOnPolicyTimeout(t *testing.T) {
	workspace, head := newOutcomeQualityGitRepo(t, "worker\n")
	request := outcomeQualityWorkerTestRequest(t)
	request.Target = outcomeQualitySourceTarget(head)
	request.Policy.TimeoutMillis = 2_000
	executor := &OutcomeQualityProcessExecutor{
		Command: os.Args[0],
		Args:    []string{"-test.run", "^TestOutcomeQualityWorkerChild$", "-test.count=1"},
		Binding: OutcomeQualitySourceBinding{Target: request.Target, Workspace: workspace},
	}
	t.Setenv(outcomeQualityWorkerChildEnv, "spawn-descendant")
	started := time.Now()
	_, err := executor.Execute(context.Background(), requestToOutcomeQualityExecutionRequest(request))
	if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("timeout error=%v, want deadline", err)
	}
	if elapsed := time.Since(started); elapsed > 4*time.Second {
		t.Fatalf("descendant cleanup took too long: %s", elapsed)
	}
}

func TestOutcomeQualityWorkerChild(t *testing.T) {
	mode := os.Getenv(outcomeQualityWorkerChildEnv)
	if mode == "" {
		return
	}
	switch mode {
	case "sleep":
		time.Sleep(5 * time.Second)
		return
	case "spawn-descendant":
		command := exec.Command(os.Args[0], "-test.run", "^TestOutcomeQualityWorkerDescendant$", "-test.count=1")
		command.Env = outcomeQualityWorkerTestEnvironment("descendant")
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		if err := command.Start(); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	case "env":
		for _, key := range []string{"PICOGENT_HOME", "PICOGENT_CODEX_HOME", "CODEX_HOME", "OLLAMA_HOST"} {
			if os.Getenv(key) != "" {
				_, _ = fmt.Fprintln(os.Stderr, "untrusted worker environment: "+key)
				os.Exit(1)
			}
		}
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

func TestOutcomeQualityWorkerDescendant(t *testing.T) {
	if os.Getenv(outcomeQualityWorkerChildEnv) != "descendant" {
		return
	}
	time.Sleep(5 * time.Second)
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
	target := testOutcomeQualityRunnerConfig(2).Candidate
	target.SourceHead = outcomeQualityWorkerCurrentHead(t)
	return OutcomeQualityWorkerRequest{
		Protocol:    OutcomeQualityWorkerProtocol,
		Scenario:    scenario,
		Variant:     OutcomeVariantCandidate,
		Repetition:  1,
		InputSHA256: scenario.InputSHA256,
		Input:       normalized,
		Target:      target,
		Policy:      testOutcomeQualityRunnerConfig(2).Policy,
	}
}

func outcomeQualityWorkerCurrentHead(t *testing.T) string {
	t.Helper()
	output, err := exec.Command("git", "rev-parse", "--verify", "HEAD").Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD: %v", err)
	}
	return strings.TrimSpace(string(output))
}

func outcomeQualityWorkerTestEnvironment(mode string) []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		key, _, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(key, outcomeQualityWorkerChildEnv) {
			continue
		}
		env = append(env, entry)
	}
	return append(env, outcomeQualityWorkerChildEnv+"="+mode)
}

type shortOutcomeQualityWorkerWriter struct{}

func (shortOutcomeQualityWorkerWriter) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	return len(data) - 1, nil
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
