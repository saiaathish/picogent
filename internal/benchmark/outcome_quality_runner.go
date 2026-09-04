package benchmark

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// OutcomeQualityRunnerToolVersion identifies the deterministic runner format
// separately from the report schema. A later runner may evolve its fixture
// execution while keeping old reports readable.
const OutcomeQualityRunnerToolVersion = "picogent-outcome-quality-runner-v1"

const (
	maxOutcomeQualityFixtureFiles = 32
	maxOutcomeQualityFixtureBytes = 1 << 20
)

// OutcomeQualityInput is the exact bounded prompt/fixture input shared by
// both source-head variants. It is deliberately data-only: the runner does
// not interpret it as a plan or permit it to add a new scenario category.
type OutcomeQualityInput struct {
	Prompt               string                    `json:"prompt"`
	Files                []OutcomeQualityInputFile `json:"files"`
	ExpectedChangedPaths []string                  `json:"expected_changed_paths"`
}

// OutcomeQualityInputFile is one deterministic fixture file. Contents are
// only used by a supplied executor; they are included in the input digest so
// the two variants cannot silently receive different bytes.
type OutcomeQualityInputFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// DefaultOutcomeQualityInput returns a small deterministic fixture for each
// catalog entry. It is intentionally modest: the M lane measures runner and
// contract behavior, not arbitrary repository quality.
func DefaultOutcomeQualityInput(scenario OutcomeQualityScenario) OutcomeQualityInput {
	return OutcomeQualityInput{
		Prompt: fmt.Sprintf("Finish the deterministic benchmark fixture for %s", scenario.ID),
		Files: []OutcomeQualityInputFile{{
			Path:    "fixture.txt",
			Content: fmt.Sprintf("before %s\n", scenario.ID),
		}},
		ExpectedChangedPaths: []string{"fixture.txt"},
	}
}

// OutcomeQualityRunnerConfig contains the shared comparison policy and
// provenance. The runner rejects unequal target environments before invoking
// an executor.
type OutcomeQualityRunnerConfig struct {
	Baseline      OutcomeQualityTarget
	Candidate     OutcomeQualityTarget
	Policy        OutcomeQualityPolicy
	Command       string
	ScenarioInput func(OutcomeQualityScenario) (OutcomeQualityInput, error)
	Unverified    []string
}

// OutcomeQualityExecutionRequest is the immutable-at-the-call-boundary input
// for one scenario/variant/repetition. Input slices are cloned before the
// executor receives them.
type OutcomeQualityExecutionRequest struct {
	Scenario    OutcomeQualityScenario
	Variant     OutcomeQualityVariant
	Repetition  int
	InputSHA256 string
	Input       OutcomeQualityInput
	Target      OutcomeQualityTarget
	Policy      OutcomeQualityPolicy
}

// OutcomeQualityExecution is the executor's bounded observation. The runner
// owns ordering, source-head binding, latency measurement, and final report
// validation.
type OutcomeQualityExecution struct {
	Metrics    OutcomeQualityMetrics
	Unverified []string
}

// OutcomeQualityExecutor is the only extension point in the M runner. The
// production agent, taskstate, verification, and provider seams remain owned
// by the supplied executor; the runner adds no second planner or store.
type OutcomeQualityExecutor interface {
	Execute(context.Context, OutcomeQualityExecutionRequest) (OutcomeQualityExecution, error)
}

// OutcomeQualityExecutorFunc adapts a function to OutcomeQualityExecutor.
type OutcomeQualityExecutorFunc func(context.Context, OutcomeQualityExecutionRequest) (OutcomeQualityExecution, error)

func (f OutcomeQualityExecutorFunc) Execute(ctx context.Context, request OutcomeQualityExecutionRequest) (OutcomeQualityExecution, error) {
	if f == nil {
		return OutcomeQualityExecution{}, fmt.Errorf("outcome-quality executor is nil")
	}
	return f(ctx, request)
}

// RunOutcomeQualityMatrix executes the fixed catalog sequentially in stable
// scenario, variant, repetition order. A failed individual execution is
// represented as an explicit inconclusive observation so the report cannot
// turn a partial run into a passing comparison.
func RunOutcomeQualityMatrix(ctx context.Context, cfg OutcomeQualityRunnerConfig, executor OutcomeQualityExecutor) (OutcomeQualityReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateOutcomeQualityRunnerConfig(cfg, executor); err != nil {
		return OutcomeQualityReport{}, err
	}

	inputFactory := cfg.ScenarioInput
	if inputFactory == nil {
		inputFactory = func(scenario OutcomeQualityScenario) (OutcomeQualityInput, error) {
			return DefaultOutcomeQualityInput(scenario), nil
		}
	}
	scenarios := DefaultOutcomeQualityScenarios()
	inputs := make([]OutcomeQualityInput, len(scenarios))
	for index, scenario := range scenarios {
		input, err := inputFactory(scenario)
		if err != nil {
			return OutcomeQualityReport{}, fmt.Errorf("scenario %q input: %w", scenario.ID, err)
		}
		input, err = normalizeOutcomeQualityInput(input)
		if err != nil {
			return OutcomeQualityReport{}, fmt.Errorf("scenario %q input: %w", scenario.ID, err)
		}
		scenarios[index].InputSHA256 = outcomeQualityInputDigest(input)
		inputs[index] = input
	}

	report := OutcomeQualityReport{
		Schema:       OutcomeQualitySchema,
		ScenarioSet:  OutcomeQualityScenarioSet,
		Status:       OutcomeReportComplete,
		Baseline:     cfg.Baseline,
		Candidate:    cfg.Candidate,
		Policy:       cfg.Policy,
		Command:      cfg.Command,
		Scenarios:    scenarios,
		Unverified:   boundedOutcomeQualityReasons(cfg.Unverified, MaxOutcomeQualityUnverified),
		Observations: make([]OutcomeQualityObservation, 0, len(scenarios)*2*cfg.Policy.Repetitions),
	}

	for scenarioIndex, scenario := range scenarios {
		for _, variant := range []OutcomeQualityVariant{OutcomeVariantBaseline, OutcomeVariantCandidate} {
			target := cfg.Baseline
			if variant == OutcomeVariantCandidate {
				target = cfg.Candidate
			}
			for repetition := 1; repetition <= cfg.Policy.Repetitions; repetition++ {
				if err := ctx.Err(); err != nil {
					report.Status = OutcomeReportInconclusive
					report.Unverified = appendOutcomeQualityReason(report.Unverified, fmt.Sprintf("matrix canceled before %s/%s repetition %d: %v", scenario.ID, variant, repetition, err), MaxOutcomeQualityUnverified)
					return finalizeOutcomeQualityReport(report, err)
				}

				request := OutcomeQualityExecutionRequest{
					Scenario:    scenario,
					Variant:     variant,
					Repetition:  repetition,
					InputSHA256: scenario.InputSHA256,
					Input:       cloneOutcomeQualityInput(inputs[scenarioIndex]),
					Target:      target,
					Policy:      cfg.Policy,
				}
				started := time.Now()
				runCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Policy.TimeoutMillis)*time.Millisecond)
				execution, executionErr := executor.Execute(runCtx, request)
				cancel()
				elapsed := time.Since(started).Milliseconds()
				observation, reasons := outcomeQualityObservationFromExecution(request, execution, executionErr, elapsed)
				report.Observations = append(report.Observations, observation)
				if len(reasons) > 0 {
					report.Status = OutcomeReportInconclusive
					for _, reason := range reasons {
						report.Unverified = appendOutcomeQualityReason(report.Unverified, reason, MaxOutcomeQualityUnverified)
					}
				}
			}
		}
	}
	return finalizeOutcomeQualityReport(report, nil)
}

func validateOutcomeQualityRunnerConfig(cfg OutcomeQualityRunnerConfig, executor OutcomeQualityExecutor) error {
	if executor == nil {
		return fmt.Errorf("outcome-quality executor is required")
	}
	if err := validateOutcomeQualityTarget("baseline", cfg.Baseline); err != nil {
		return err
	}
	if err := validateOutcomeQualityTarget("candidate", cfg.Candidate); err != nil {
		return err
	}
	if strings.EqualFold(cfg.Baseline.SourceHead, cfg.Candidate.SourceHead) {
		return fmt.Errorf("baseline and candidate must use different source heads")
	}
	if cfg.Baseline.Host != cfg.Candidate.Host || cfg.Baseline.GoVersion != cfg.Candidate.GoVersion || cfg.Baseline.ToolVersion != cfg.Candidate.ToolVersion {
		return fmt.Errorf("baseline and candidate must share host, Go version, and tool version")
	}
	if err := validateOutcomeQualityPolicy(cfg.Policy); err != nil {
		return err
	}
	if err := validateText("command", cfg.Command, true); err != nil {
		return err
	}
	return nil
}

func normalizeOutcomeQualityInput(input OutcomeQualityInput) (OutcomeQualityInput, error) {
	if err := validateText("prompt", input.Prompt, true); err != nil {
		return OutcomeQualityInput{}, err
	}
	if len(input.Files) == 0 || len(input.Files) > maxOutcomeQualityFixtureFiles {
		return OutcomeQualityInput{}, fmt.Errorf("files=%d outside 1..%d", len(input.Files), maxOutcomeQualityFixtureFiles)
	}
	out := OutcomeQualityInput{Prompt: strings.TrimSpace(input.Prompt)}
	out.Files = make([]OutcomeQualityInputFile, 0, len(input.Files))
	seen := make(map[string]struct{}, len(input.Files))
	bytes := 0
	for _, file := range input.Files {
		path, err := normalizeOutcomeQualityFixturePath(file.Path)
		if err != nil {
			return OutcomeQualityInput{}, err
		}
		if _, ok := seen[path]; ok {
			return OutcomeQualityInput{}, fmt.Errorf("fixture path %q is repeated", path)
		}
		seen[path] = struct{}{}
		if len(file.Content) > maxOutcomeQualityFixtureBytes {
			return OutcomeQualityInput{}, fmt.Errorf("fixture %q content is too large", path)
		}
		bytes += len(file.Content)
		if bytes > maxOutcomeQualityFixtureBytes {
			return OutcomeQualityInput{}, fmt.Errorf("fixture contents exceed %d bytes", maxOutcomeQualityFixtureBytes)
		}
		out.Files = append(out.Files, OutcomeQualityInputFile{Path: path, Content: file.Content})
	}
	sort.Slice(out.Files, func(i, j int) bool { return out.Files[i].Path < out.Files[j].Path })
	for _, path := range input.ExpectedChangedPaths {
		path, err := normalizeOutcomeQualityFixturePath(path)
		if err != nil {
			return OutcomeQualityInput{}, fmt.Errorf("expected changed path: %w", err)
		}
		if _, ok := seen[path]; !ok {
			return OutcomeQualityInput{}, fmt.Errorf("expected changed path %q is not in fixture files", path)
		}
		out.ExpectedChangedPaths = append(out.ExpectedChangedPaths, path)
	}
	sort.Strings(out.ExpectedChangedPaths)
	return out, nil
}

func normalizeOutcomeQualityFixturePath(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if raw == "" || strings.ContainsRune(raw, '\x00') || filepath.IsAbs(raw) {
		return "", fmt.Errorf("fixture path %q is invalid", raw)
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(raw)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || len(clean) > MaxOutcomeQualityTextBytes {
		return "", fmt.Errorf("fixture path %q is outside the bounded fixture", raw)
	}
	return clean, nil
}

func outcomeQualityInputDigest(input OutcomeQualityInput) string {
	data, _ := json.Marshal(input)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func cloneOutcomeQualityInput(input OutcomeQualityInput) OutcomeQualityInput {
	clone := input
	clone.Files = append([]OutcomeQualityInputFile(nil), input.Files...)
	clone.ExpectedChangedPaths = append([]string(nil), input.ExpectedChangedPaths...)
	return clone
}

func outcomeQualityObservationFromExecution(request OutcomeQualityExecutionRequest, execution OutcomeQualityExecution, executionErr error, elapsedMillis int64) (OutcomeQualityObservation, []string) {
	if elapsedMillis < 0 {
		elapsedMillis = 0
	}
	reasons := boundedOutcomeQualityReasons(execution.Unverified, 8)
	if executionErr != nil {
		reasons = appendOutcomeQualityReason(reasons, "executor failed: "+executionErr.Error(), 8)
	}
	if elapsedMillis > request.Policy.TimeoutMillis {
		reasons = appendOutcomeQualityReason(reasons, fmt.Sprintf("executor exceeded timeout_millis=%d", request.Policy.TimeoutMillis), 8)
		elapsedMillis = request.Policy.TimeoutMillis
	}

	metrics := execution.Metrics
	metrics.LatencyMillis = elapsedMillis
	if len(reasons) > 0 {
		metrics = inconclusiveOutcomeQualityMetrics(elapsedMillis)
	} else if err := validateOutcomeQualityMetrics(metrics, request.Policy, 0); err != nil {
		reasons = appendOutcomeQualityReason(reasons, "executor metrics rejected: "+err.Error(), 8)
		metrics = inconclusiveOutcomeQualityMetrics(elapsedMillis)
	}
	return OutcomeQualityObservation{
		ScenarioID: request.Scenario.ID,
		Variant:    request.Variant,
		Repetition: request.Repetition,
		SourceHead: request.Target.SourceHead,
		Metrics:    metrics,
		Unverified: reasons,
	}, reasons
}

func inconclusiveOutcomeQualityMetrics(latencyMillis int64) OutcomeQualityMetrics {
	return OutcomeQualityMetrics{
		OutcomeSuccess:      OutcomeAssessmentInconclusive,
		Correctness:         OutcomeAssessmentInconclusive,
		LatencyMillis:       latencyMillis,
		VerificationQuality: OutcomeVerificationInconclusive,
		Evidence:            EvidenceUnverified,
	}
}

func finalizeOutcomeQualityReport(report OutcomeQualityReport, runErr error) (OutcomeQualityReport, error) {
	if err := report.Validate(); err != nil {
		if runErr != nil {
			return report, fmt.Errorf("outcome-quality matrix canceled: %w (report invalid: %v)", runErr, err)
		}
		return report, fmt.Errorf("outcome-quality report: %w", err)
	}
	return report, runErr
}

func boundedOutcomeQualityReasons(values []string, max int) []string {
	out := make([]string, 0, minOutcomeQualityInt(len(values), max))
	for _, value := range values {
		out = appendOutcomeQualityReason(out, value, max)
	}
	return out
}

func appendOutcomeQualityReason(values []string, value string, max int) []string {
	value = strings.TrimSpace(value)
	if value == "" || max <= 0 {
		return values
	}
	if len(value) > MaxOutcomeQualityTextBytes {
		value = value[:MaxOutcomeQualityTextBytes]
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	if len(values) >= max {
		return values
	}
	return append(values, value)
}

func minOutcomeQualityInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
