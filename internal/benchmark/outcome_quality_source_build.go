package benchmark

import (
	"context"
	"errors"
	"fmt"
)

// OutcomeQualitySourcePairBuildConfig describes the two exact source trees
// that will be built for one comparison. The legacy provider is supplied as a
// loopback URL because the v3 executable has no worker protocol; it must not
// contain credentials or point at a non-loopback service.
type OutcomeQualitySourcePairBuildConfig struct {
	Baseline          OutcomeQualitySourceBinding
	Candidate         OutcomeQualitySourceBinding
	TempParent        string
	LegacyProviderURL string
	LegacyModel       string
}

// OutcomeQualitySourcePairBuild owns the two independently built executors
// used by one exact-head matrix. Both build directories are external to their
// source worktrees. Call Close when the matrix and its report have been
// captured.
type OutcomeQualitySourcePairBuild struct {
	legacy *OutcomeQualityLegacyBuild
	worker *OutcomeQualityWorkerBuild
}

// BuildOutcomeQualitySourcePair validates both clean exact-head worktrees
// before building either target. The baseline is built through the
// legacy-compatible cmd/picogent adapter and the candidate through the
// versioned outcome-quality worker. A candidate build failure closes the
// already-built legacy target before returning.
func BuildOutcomeQualitySourcePair(ctx context.Context, cfg OutcomeQualitySourcePairBuildConfig) (*OutcomeQualitySourcePairBuild, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ValidateOutcomeQualitySourcePair(ctx, cfg.Baseline, cfg.Candidate); err != nil {
		return nil, fmt.Errorf("outcome-quality source-pair build preflight: %w", err)
	}

	legacy, err := BuildOutcomeQualityLegacy(ctx, cfg.Baseline, OutcomeQualityLegacyBuildConfig{
		TempParent:  cfg.TempParent,
		ProviderURL: cfg.LegacyProviderURL,
		Model:       cfg.LegacyModel,
	})
	if err != nil {
		return nil, fmt.Errorf("build outcome-quality baseline: %w", err)
	}

	worker, err := BuildOutcomeQualityWorker(ctx, cfg.Candidate, cfg.TempParent)
	if err != nil {
		if closeErr := legacy.Close(); closeErr != nil {
			return nil, errors.Join(fmt.Errorf("build outcome-quality candidate: %w", err), fmt.Errorf("close baseline after candidate build failure: %w", closeErr))
		}
		return nil, fmt.Errorf("build outcome-quality candidate: %w", err)
	}

	return &OutcomeQualitySourcePairBuild{legacy: legacy, worker: worker}, nil
}

// BaselineExecutor returns the source-bound v3 executor, or nil after the
// build has been closed.
func (b *OutcomeQualitySourcePairBuild) BaselineExecutor() *OutcomeQualityLegacyProcessExecutor {
	if b == nil || b.legacy == nil {
		return nil
	}
	return b.legacy.ProcessExecutor()
}

// CandidateExecutor returns the source-bound v4 worker executor, or nil after
// the build has been closed.
func (b *OutcomeQualitySourcePairBuild) CandidateExecutor() *OutcomeQualityProcessExecutor {
	if b == nil || b.worker == nil {
		return nil
	}
	return b.worker.ProcessExecutor()
}

// RunMatrix runs the fixed source-pair matrix using the two executors owned by
// the build. The runner still performs its own source preflight and report
// validation so a caller cannot reuse a build with a different binding.
func (b *OutcomeQualitySourcePairBuild) RunMatrix(ctx context.Context, cfg OutcomeQualitySourcePairConfig) (OutcomeQualityReport, error) {
	if b == nil {
		return OutcomeQualityReport{}, errors.New("outcome-quality source-pair build is nil")
	}
	if b.legacy == nil || b.worker == nil {
		return OutcomeQualityReport{}, errors.New("outcome-quality source-pair build is closed")
	}
	return RunOutcomeQualitySourcePairMatrix(ctx, cfg, b.BaselineExecutor(), b.CandidateExecutor())
}

// Close removes both external build directories. It is safe to call more
// than once; a failed removal leaves that target available for a retry.
func (b *OutcomeQualitySourcePairBuild) Close() error {
	if b == nil {
		return nil
	}
	var errs []error
	if b.legacy != nil {
		if err := b.legacy.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close outcome-quality baseline build: %w", err))
		} else {
			b.legacy = nil
		}
	}
	if b.worker != nil {
		if err := b.worker.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close outcome-quality candidate build: %w", err))
		} else {
			b.worker = nil
		}
	}
	return errors.Join(errs...)
}
