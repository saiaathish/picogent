package benchmark

import (
	"context"
	"fmt"
)

// OutcomeQualitySourcePairConfig is the controller input for one exact-head
// comparison. Workspace paths are runtime-only; the target metadata is copied
// into the report by the runner.
type OutcomeQualitySourcePairConfig struct {
	Baseline      OutcomeQualitySourceBinding
	Candidate     OutcomeQualitySourceBinding
	Policy        OutcomeQualityPolicy
	Command       string
	ScenarioInput func(OutcomeQualityScenario) (OutcomeQualityInput, error)
	Unverified    []string
}

// OutcomeQualitySourceExecutor is the source-aware extension point for an
// exact-head matrix. Its unexported hooks keep arbitrary metrics callbacks
// from being passed as source-backed executors by callers outside benchmark.
type OutcomeQualitySourceExecutor interface {
	OutcomeQualityExecutor
	outcomeQualitySourceBinding() OutcomeQualitySourceBinding
	validateOutcomeQualitySource(context.Context) error
}

// RunOutcomeQualitySourcePairMatrix validates both exact source worktrees
// before delegating the fixed matrix to the existing runner. Each variant is
// routed to its own executor; no parallel runner, store, or report authority
// is introduced here.
func RunOutcomeQualitySourcePairMatrix(ctx context.Context, cfg OutcomeQualitySourcePairConfig, baseline, candidate OutcomeQualitySourceExecutor) (OutcomeQualityReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ValidateOutcomeQualitySourcePair(ctx, cfg.Baseline, cfg.Candidate); err != nil {
		return OutcomeQualityReport{}, fmt.Errorf("outcome-quality source preflight: %w", err)
	}
	if err := validateOutcomeQualitySourceExecutor(ctx, "baseline", baseline, cfg.Baseline); err != nil {
		return OutcomeQualityReport{}, err
	}
	if err := validateOutcomeQualitySourceExecutor(ctx, "candidate", candidate, cfg.Candidate); err != nil {
		return OutcomeQualityReport{}, err
	}
	executor, err := newOutcomeQualitySourcePairExecutor(cfg, baseline, candidate)
	if err != nil {
		return OutcomeQualityReport{}, err
	}
	return RunOutcomeQualityMatrix(ctx, OutcomeQualityRunnerConfig{
		Baseline:      cfg.Baseline.Target,
		Candidate:     cfg.Candidate.Target,
		Policy:        cfg.Policy,
		Command:       cfg.Command,
		ScenarioInput: cfg.ScenarioInput,
		Unverified:    cfg.Unverified,
	}, executor)
}

type outcomeQualitySourcePairExecutor struct {
	baselineTarget  OutcomeQualityTarget
	candidateTarget OutcomeQualityTarget
	baseline        OutcomeQualitySourceExecutor
	candidate       OutcomeQualitySourceExecutor
}

func newOutcomeQualitySourcePairExecutor(cfg OutcomeQualitySourcePairConfig, baseline, candidate OutcomeQualitySourceExecutor) (*outcomeQualitySourcePairExecutor, error) {
	if baseline == nil {
		return nil, fmt.Errorf("outcome-quality baseline executor is required")
	}
	if candidate == nil {
		return nil, fmt.Errorf("outcome-quality candidate executor is required")
	}
	return &outcomeQualitySourcePairExecutor{
		baselineTarget:  cfg.Baseline.Target,
		candidateTarget: cfg.Candidate.Target,
		baseline:        baseline,
		candidate:       candidate,
	}, nil
}

func validateOutcomeQualitySourceExecutor(ctx context.Context, name string, executor OutcomeQualitySourceExecutor, binding OutcomeQualitySourceBinding) error {
	if executor == nil {
		return fmt.Errorf("outcome-quality %s executor is required", name)
	}
	declared := executor.outcomeQualitySourceBinding()
	if err := validateOutcomeQualityTarget(name+" executor", declared.Target); err != nil {
		return err
	}
	expectedWorkspace, err := canonicalOutcomeQualityWorkspace(binding.Workspace)
	if err != nil {
		return fmt.Errorf("%s source workspace: %w", name, err)
	}
	declaredWorkspace, err := canonicalOutcomeQualityWorkspace(declared.Workspace)
	if err != nil {
		return fmt.Errorf("%s executor workspace: %w", name, err)
	}
	if declaredWorkspace != expectedWorkspace || !outcomeQualityTargetsEqual(declared.Target, binding.Target) {
		return fmt.Errorf("outcome-quality %s executor binding does not match source binding", name)
	}
	if err := executor.validateOutcomeQualitySource(ctx); err != nil {
		return fmt.Errorf("outcome-quality %s executor source: %w", name, err)
	}
	return nil
}

func (e *outcomeQualitySourcePairExecutor) Execute(ctx context.Context, request OutcomeQualityExecutionRequest) (OutcomeQualityExecution, error) {
	if e == nil {
		return OutcomeQualityExecution{}, fmt.Errorf("outcome-quality source-pair executor is nil")
	}
	var target OutcomeQualityTarget
	var delegate OutcomeQualitySourceExecutor
	switch request.Variant {
	case OutcomeVariantBaseline:
		target = e.baselineTarget
		delegate = e.baseline
	case OutcomeVariantCandidate:
		target = e.candidateTarget
		delegate = e.candidate
	default:
		return OutcomeQualityExecution{}, fmt.Errorf("outcome-quality source-pair variant %q is unsupported", request.Variant)
	}
	if !outcomeQualityTargetsEqual(request.Target, target) {
		return OutcomeQualityExecution{}, fmt.Errorf("outcome-quality %s request target does not match source binding", request.Variant)
	}
	return delegate.Execute(ctx, request)
}
