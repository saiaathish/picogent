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

// RunOutcomeQualitySourcePairMatrix validates both exact source worktrees
// before delegating the fixed matrix to the existing runner. Each variant is
// routed to its own executor; no parallel runner, store, or report authority
// is introduced here.
func RunOutcomeQualitySourcePairMatrix(ctx context.Context, cfg OutcomeQualitySourcePairConfig, baseline, candidate OutcomeQualityExecutor) (OutcomeQualityReport, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ValidateOutcomeQualitySourcePair(ctx, cfg.Baseline, cfg.Candidate); err != nil {
		return OutcomeQualityReport{}, fmt.Errorf("outcome-quality source preflight: %w", err)
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
	baseline        OutcomeQualityExecutor
	candidate       OutcomeQualityExecutor
}

func newOutcomeQualitySourcePairExecutor(cfg OutcomeQualitySourcePairConfig, baseline, candidate OutcomeQualityExecutor) (*outcomeQualitySourcePairExecutor, error) {
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

func (e *outcomeQualitySourcePairExecutor) Execute(ctx context.Context, request OutcomeQualityExecutionRequest) (OutcomeQualityExecution, error) {
	if e == nil {
		return OutcomeQualityExecution{}, fmt.Errorf("outcome-quality source-pair executor is nil")
	}
	var target OutcomeQualityTarget
	var delegate OutcomeQualityExecutor
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
	if request.Target != target {
		return OutcomeQualityExecution{}, fmt.Errorf("outcome-quality %s request target does not match source binding", request.Variant)
	}
	return delegate.Execute(ctx, request)
}
