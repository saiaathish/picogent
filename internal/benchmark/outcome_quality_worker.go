package benchmark

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/procenv"
)

// OutcomeQualityWorkerProtocol identifies the one-request/one-response
// protocol used to isolate a benchmark target process.
const OutcomeQualityWorkerProtocol = "picogent.v4.outcome-quality-worker.v1"

const (
	maxOutcomeQualityWorkerRequestBytes  = 2 << 20
	maxOutcomeQualityWorkerResponseBytes = 64 << 10
	maxOutcomeQualityWorkerArgs          = 32
	maxOutcomeQualityWorkerReadTimeout   = 10 * time.Second
)

// OutcomeQualityWorkerRequest is the complete bounded input for one target
// execution. The controller sends the same request data to both source-head
// variants; the worker does not interpret the prompt as an instruction to the
// controller.
type OutcomeQualityWorkerRequest struct {
	Protocol    string                 `json:"protocol"`
	Scenario    OutcomeQualityScenario `json:"scenario"`
	Variant     OutcomeQualityVariant  `json:"variant"`
	Repetition  int                    `json:"repetition"`
	InputSHA256 string                 `json:"input_sha256"`
	Input       OutcomeQualityInput    `json:"input"`
	Target      OutcomeQualityTarget   `json:"target"`
	Policy      OutcomeQualityPolicy   `json:"policy"`
}

// OutcomeQualityWorkerResponse is the bounded result returned by one worker.
// SourceHead is checked against the controller's target before the result is
// admitted to the comparison report.
type OutcomeQualityWorkerResponse struct {
	Protocol   string                `json:"protocol"`
	SourceHead string                `json:"source_head"`
	Metrics    OutcomeQualityMetrics `json:"metrics"`
	Unverified []string              `json:"unverified,omitempty"`
}

// OutcomeQualityProcessExecutor runs a worker command with one JSON request
// on stdin and one JSON response on stdout. Command and Args are typed
// process inputs; no shell is involved and the inherited environment is
// sanitized before launch.
type OutcomeQualityProcessExecutor struct {
	Command string
	Args    []string
	Binding OutcomeQualitySourceBinding
}

func (e *OutcomeQualityProcessExecutor) outcomeQualitySourceBinding() OutcomeQualitySourceBinding {
	if e == nil {
		return OutcomeQualitySourceBinding{}
	}
	return e.Binding
}

func (e *OutcomeQualityProcessExecutor) validateOutcomeQualitySource(ctx context.Context) error {
	if e == nil {
		return fmt.Errorf("outcome-quality process executor is nil")
	}
	workspace, err := canonicalOutcomeQualityWorkspace(e.Binding.Workspace)
	if err != nil {
		return fmt.Errorf("outcome-quality worker workspace: %w", err)
	}
	return validateOutcomeQualitySourceBinding(ctx, "worker", e.Binding.Target, workspace)
}

// RunOutcomeQualityWorker serves exactly one request. It writes no response
// when decoding, validation, or execution fails, allowing the controller to
// record that observation as inconclusive instead of accepting partial data.
func RunOutcomeQualityWorker(ctx context.Context, input io.Reader, output io.Writer, executor OutcomeQualityExecutor) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if input == nil {
		return fmt.Errorf("outcome-quality worker input is required")
	}
	if output == nil {
		return fmt.Errorf("outcome-quality worker output is required")
	}
	if executor == nil {
		return fmt.Errorf("outcome-quality worker executor is required")
	}

	readCtx, cancelRead := context.WithTimeout(ctx, maxOutcomeQualityWorkerReadTimeout)
	defer cancelRead()
	request, err := decodeOutcomeQualityWorkerRequest(readCtx, input)
	if err != nil {
		return err
	}
	executionRequest, err := normalizeOutcomeQualityWorkerRequest(request)
	if err != nil {
		return err
	}
	sourceHead, err := outcomeQualityWorkerSourceHead(ctx)
	if err != nil {
		return err
	}
	if !strings.EqualFold(sourceHead, request.Target.SourceHead) {
		return fmt.Errorf("outcome-quality worker source head does not match target")
	}
	execution, err := executor.Execute(ctx, executionRequest)
	if err != nil {
		return fmt.Errorf("outcome-quality worker execution failed: %w", err)
	}
	reasons := boundedOutcomeQualityReasons(execution.Unverified, 8)
	metrics := execution.Metrics
	if len(reasons) > 0 {
		metrics = inconclusiveOutcomeQualityMetrics(metrics.LatencyMillis)
	}
	if err := validateOutcomeQualityMetrics(metrics, request.Policy, 0); err != nil {
		return fmt.Errorf("outcome-quality worker metrics: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("outcome-quality worker canceled: %w", err)
	}
	response := OutcomeQualityWorkerResponse{
		Protocol:   OutcomeQualityWorkerProtocol,
		SourceHead: sourceHead,
		Metrics:    metrics,
		Unverified: reasons,
	}
	return encodeOutcomeQualityWorkerResponse(output, response)
}

// Execute implements OutcomeQualityExecutor by invoking the configured
// worker process. The source binding is checked immediately before launch so
// a stale or dirty target cannot produce a passing-looking observation.
func (e *OutcomeQualityProcessExecutor) Execute(ctx context.Context, request OutcomeQualityExecutionRequest) (OutcomeQualityExecution, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if e == nil {
		return OutcomeQualityExecution{}, fmt.Errorf("outcome-quality process executor is nil")
	}
	if strings.TrimSpace(e.Command) == "" {
		return OutcomeQualityExecution{}, fmt.Errorf("outcome-quality worker command is required")
	}
	if len(e.Command) > MaxOutcomeQualityTextBytes {
		return OutcomeQualityExecution{}, fmt.Errorf("outcome-quality worker command is too large")
	}
	if !filepath.IsAbs(e.Command) {
		return OutcomeQualityExecution{}, fmt.Errorf("outcome-quality worker command must be an absolute path")
	}
	if len(e.Args) > maxOutcomeQualityWorkerArgs {
		return OutcomeQualityExecution{}, fmt.Errorf("outcome-quality worker args exceed %d", maxOutcomeQualityWorkerArgs)
	}
	for index, arg := range e.Args {
		if len(arg) > MaxOutcomeQualityTextBytes {
			return OutcomeQualityExecution{}, fmt.Errorf("outcome-quality worker arg %d is too large", index)
		}
	}
	if err := validateOutcomeQualityWorkerTarget(e.Binding.Target, request.Target); err != nil {
		return OutcomeQualityExecution{}, fmt.Errorf("outcome-quality worker target: %w", err)
	}
	if err := validateOutcomeQualityPolicy(request.Policy); err != nil {
		return OutcomeQualityExecution{}, fmt.Errorf("outcome-quality worker policy: %w", err)
	}
	workerCtx, cancel := context.WithTimeout(ctx, time.Duration(request.Policy.TimeoutMillis)*time.Millisecond)
	defer cancel()
	workspace, err := canonicalOutcomeQualityWorkspace(e.Binding.Workspace)
	if err != nil {
		return OutcomeQualityExecution{}, fmt.Errorf("outcome-quality worker workspace: %w", err)
	}
	if err := validateOutcomeQualitySourceBinding(workerCtx, "worker", e.Binding.Target, workspace); err != nil {
		return OutcomeQualityExecution{}, err
	}

	workerRequest := OutcomeQualityWorkerRequest{
		Protocol:    OutcomeQualityWorkerProtocol,
		Scenario:    request.Scenario,
		Variant:     request.Variant,
		Repetition:  request.Repetition,
		InputSHA256: request.InputSHA256,
		Input:       cloneOutcomeQualityInput(request.Input),
		Target:      request.Target,
		Policy:      request.Policy,
	}
	payload, err := json.Marshal(workerRequest)
	if err != nil {
		return OutcomeQualityExecution{}, fmt.Errorf("encode outcome-quality worker request: %w", err)
	}
	if len(payload) > maxOutcomeQualityWorkerRequestBytes {
		return OutcomeQualityExecution{}, fmt.Errorf("outcome-quality worker request exceeds %d bytes", maxOutcomeQualityWorkerRequestBytes)
	}

	command := exec.Command(e.Command, e.Args...)
	command.Dir = workspace
	command.Env = outcomeQualityWorkerEnvironment()
	command.Stdin = bytes.NewReader(payload)
	var stdout outcomeQualityWorkerBuffer
	command.Stdout = &stdout
	command.Stderr = io.Discard
	configureOutcomeQualityWorkerCommand(command)
	if err := runOutcomeQualityWorkerCommand(workerCtx, command); err != nil {
		return OutcomeQualityExecution{}, fmt.Errorf("outcome-quality worker command failed: %w", err)
	}
	if err := workerCtx.Err(); err != nil {
		return OutcomeQualityExecution{}, fmt.Errorf("outcome-quality worker command failed: %w", err)
	}
	if stdout.truncated {
		return OutcomeQualityExecution{}, fmt.Errorf("outcome-quality worker response exceeds %d bytes", maxOutcomeQualityWorkerResponseBytes)
	}
	if err := validateOutcomeQualitySourceBinding(workerCtx, "worker", e.Binding.Target, workspace); err != nil {
		return OutcomeQualityExecution{}, err
	}
	response, err := decodeOutcomeQualityWorkerResponse(bytes.NewReader(stdout.data.Bytes()))
	if err != nil {
		return OutcomeQualityExecution{}, err
	}
	if !strings.EqualFold(response.SourceHead, request.Target.SourceHead) {
		return OutcomeQualityExecution{}, fmt.Errorf("outcome-quality worker source head does not match target")
	}
	reasons := boundedOutcomeQualityReasons(response.Unverified, 8)
	metrics := response.Metrics
	if len(reasons) > 0 {
		metrics = inconclusiveOutcomeQualityMetrics(metrics.LatencyMillis)
	}
	if err := validateOutcomeQualityMetrics(metrics, request.Policy, 0); err != nil {
		return OutcomeQualityExecution{}, fmt.Errorf("outcome-quality worker metrics: %w", err)
	}
	return OutcomeQualityExecution{Metrics: metrics, Unverified: reasons}, nil
}

func validateOutcomeQualityWorkerTarget(binding, request OutcomeQualityTarget) error {
	if err := validateOutcomeQualityTarget("worker binding", binding); err != nil {
		return err
	}
	if err := validateOutcomeQualityTarget("worker request", request); err != nil {
		return err
	}
	if !outcomeQualityTargetsEqual(binding, request) {
		return fmt.Errorf("worker binding and request target must match")
	}
	return nil
}

func decodeOutcomeQualityWorkerRequest(ctx context.Context, input io.Reader) (OutcomeQualityWorkerRequest, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if input == nil {
		return OutcomeQualityWorkerRequest{}, fmt.Errorf("read outcome-quality worker request: input is required")
	}
	data, err := readOutcomeQualityWorkerBytes(ctx, input, maxOutcomeQualityWorkerRequestBytes)
	if err != nil {
		return OutcomeQualityWorkerRequest{}, fmt.Errorf("read outcome-quality worker request: %w", err)
	}
	if len(data) > maxOutcomeQualityWorkerRequestBytes {
		return OutcomeQualityWorkerRequest{}, fmt.Errorf("outcome-quality worker request exceeds %d bytes", maxOutcomeQualityWorkerRequestBytes)
	}
	var request OutcomeQualityWorkerRequest
	if err := decodeOneOutcomeQualityJSON(data, &request); err != nil {
		return OutcomeQualityWorkerRequest{}, fmt.Errorf("decode outcome-quality worker request: %w", err)
	}
	return request, nil
}

func readOutcomeQualityWorkerBytes(ctx context.Context, input io.Reader, maxBytes int) ([]byte, error) {
	readDone := make(chan struct{})
	var data []byte
	var readErr error
	go func() {
		data, readErr = io.ReadAll(io.LimitReader(input, int64(maxBytes)+1))
		close(readDone)
	}()
	select {
	case <-readDone:
		return data, readErr
	case <-ctx.Done():
		if closer, ok := input.(io.Closer); ok {
			_ = closer.Close()
		}
		return nil, ctx.Err()
	}
}

func outcomeQualityWorkerSourceHead(ctx context.Context) (string, error) {
	result, err := procenv.Output(ctx, procenv.DefaultCommandTimeout, "git", "rev-parse", "--verify", "HEAD")
	if err != nil {
		return "", fmt.Errorf("outcome-quality worker source head: %w", err)
	}
	if result.Truncated {
		return "", fmt.Errorf("outcome-quality worker source head output is too large")
	}
	head := strings.TrimSpace(string(result.Output))
	if !validSHA(head) {
		return "", fmt.Errorf("outcome-quality worker source head is invalid")
	}
	return head, nil
}

func normalizeOutcomeQualityWorkerRequest(request OutcomeQualityWorkerRequest) (OutcomeQualityExecutionRequest, error) {
	if request.Protocol != OutcomeQualityWorkerProtocol {
		return OutcomeQualityExecutionRequest{}, fmt.Errorf("outcome-quality worker protocol=%q is unsupported", request.Protocol)
	}
	if !request.Variant.valid() {
		return OutcomeQualityExecutionRequest{}, fmt.Errorf("outcome-quality worker variant=%q is unsupported", request.Variant)
	}
	if !outcomeQualityScenarioMatchesCatalog(request.Scenario) {
		return OutcomeQualityExecutionRequest{}, fmt.Errorf("outcome-quality worker scenario is not in the stable catalog")
	}
	if err := validateOutcomeQualityTarget("worker target", request.Target); err != nil {
		return OutcomeQualityExecutionRequest{}, err
	}
	if err := validateOutcomeQualityPolicy(request.Policy); err != nil {
		return OutcomeQualityExecutionRequest{}, err
	}
	if request.Repetition < 1 || request.Repetition > request.Policy.Repetitions {
		return OutcomeQualityExecutionRequest{}, fmt.Errorf("outcome-quality worker repetition=%d is outside 1..%d", request.Repetition, request.Policy.Repetitions)
	}
	input, err := normalizeOutcomeQualityInput(request.Input)
	if err != nil {
		return OutcomeQualityExecutionRequest{}, fmt.Errorf("outcome-quality worker input: %w", err)
	}
	digest := outcomeQualityInputDigest(input)
	if digest != request.InputSHA256 || digest != request.Scenario.InputSHA256 {
		return OutcomeQualityExecutionRequest{}, fmt.Errorf("outcome-quality worker input digest does not match scenario")
	}
	return OutcomeQualityExecutionRequest{
		Scenario:    request.Scenario,
		Variant:     request.Variant,
		Repetition:  request.Repetition,
		InputSHA256: request.InputSHA256,
		Input:       input,
		Target:      request.Target,
		Policy:      request.Policy,
	}, nil
}

func outcomeQualityScenarioMatchesCatalog(scenario OutcomeQualityScenario) bool {
	for _, want := range DefaultOutcomeQualityScenarios() {
		if scenario.ID == want.ID && scenario.Category == want.Category && scenario.Kind == want.Kind && scenario.Seed == want.Seed {
			return validSHA256(scenario.InputSHA256)
		}
	}
	return false
}

func encodeOutcomeQualityWorkerResponse(output io.Writer, response OutcomeQualityWorkerResponse) error {
	data, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("encode outcome-quality worker response: %w", err)
	}
	if len(data) > maxOutcomeQualityWorkerResponseBytes {
		return fmt.Errorf("outcome-quality worker response exceeds %d bytes", maxOutcomeQualityWorkerResponseBytes)
	}
	written, err := output.Write(data)
	if err != nil {
		return fmt.Errorf("write outcome-quality worker response: %w", err)
	}
	if written != len(data) {
		return fmt.Errorf("write outcome-quality worker response: %w", io.ErrShortWrite)
	}
	return nil
}

func decodeOutcomeQualityWorkerResponse(input io.Reader) (OutcomeQualityWorkerResponse, error) {
	data, err := io.ReadAll(io.LimitReader(input, maxOutcomeQualityWorkerResponseBytes+1))
	if err != nil {
		return OutcomeQualityWorkerResponse{}, fmt.Errorf("read outcome-quality worker response: %w", err)
	}
	if len(data) > maxOutcomeQualityWorkerResponseBytes {
		return OutcomeQualityWorkerResponse{}, fmt.Errorf("outcome-quality worker response exceeds %d bytes", maxOutcomeQualityWorkerResponseBytes)
	}
	var response OutcomeQualityWorkerResponse
	if err := decodeOneOutcomeQualityJSON(data, &response); err != nil {
		return OutcomeQualityWorkerResponse{}, fmt.Errorf("decode outcome-quality worker response: %w", err)
	}
	if response.Protocol != OutcomeQualityWorkerProtocol {
		return OutcomeQualityWorkerResponse{}, fmt.Errorf("outcome-quality worker response protocol is unsupported")
	}
	if !validSHA(response.SourceHead) {
		return OutcomeQualityWorkerResponse{}, fmt.Errorf("outcome-quality worker response source head is invalid")
	}
	if err := validateTextList("outcome-quality worker response unverified", response.Unverified, 8); err != nil {
		return OutcomeQualityWorkerResponse{}, err
	}
	return response, nil
}

func decodeOneOutcomeQualityJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return err
	}
	return nil
}

type outcomeQualityWorkerBuffer struct {
	data      bytes.Buffer
	truncated bool
}

func (b *outcomeQualityWorkerBuffer) Write(data []byte) (int, error) {
	remaining := maxOutcomeQualityWorkerResponseBytes - b.data.Len()
	if remaining <= 0 {
		b.truncated = true
		return len(data), nil
	}
	if len(data) > remaining {
		_, _ = b.data.Write(data[:remaining])
		b.truncated = true
		return len(data), nil
	}
	_, _ = b.data.Write(data)
	return len(data), nil
}
