package benchmark

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// This is the explicitly declared candidate for the current exact-head
// evidence run. It includes OutcomeQualityTranscriptMetricContract, so the
// opt-in test below requires complete controlled-transcript evidence rather
// than silently retaining the historical adapter gap.
const outcomeQualityExactCandidateHead = "c11608747bb82b3e820009ee3f28498931758f12"

const outcomeQualityExactReportFileMode = 0o600

// TestRunOutcomeQualityExactSourcePairMatrix is opt-in because it builds two
// source trees and launches 80 isolated observations. Hosted CI exercises the
// fast contract and process-boundary tests by default; the exact evidence run
// is invoked with PICOGENT_RUN_EXACT_OUTCOME_QUALITY_MATRIX=1 and can persist
// its bounded report with PICOGENT_OUTCOME_QUALITY_REPORT.
func TestRunOutcomeQualityExactSourcePairMatrix(t *testing.T) {
	if os.Getenv("PICOGENT_RUN_EXACT_OUTCOME_QUALITY_MATRIX") != "1" {
		t.Skip("set PICOGENT_RUN_EXACT_OUTCOME_QUALITY_MATRIX=1 to run the exact 20-scenario source pair")
	}

	baselineSource := cleanOutcomeQualitySourceAtHead(t, OutcomeQualityLegacySourceHead)
	candidateSource := cleanOutcomeQualitySourceAtHead(t, outcomeQualityExactCandidateHead)
	baselineTarget := outcomeQualityExactSourceTarget(OutcomeQualityLegacySourceHead)
	candidateTarget := outcomeQualityExactSourceTarget(outcomeQualityExactCandidateHead)
	provider := newOutcomeQualityLegacyProvider(t)
	build, err := BuildOutcomeQualitySourcePair(context.Background(), OutcomeQualitySourcePairBuildConfig{
		Baseline:          OutcomeQualitySourceBinding{Target: baselineTarget, Workspace: baselineSource},
		Candidate:         OutcomeQualitySourceBinding{Target: candidateTarget, Workspace: candidateSource},
		TempParent:        t.TempDir(),
		LegacyProviderURL: provider.server.URL,
		LegacyModel:       "exact-source-pair-fixture-model",
	})
	if err != nil {
		t.Fatalf("build exact source pair: %v", err)
	}
	defer func() {
		if closeErr := build.Close(); closeErr != nil {
			t.Errorf("close exact source pair: %v", closeErr)
		}
	}()

	policy := testOutcomeQualityRunnerConfig(2).Policy
	report, err := build.RunMatrix(context.Background(), OutcomeQualitySourcePairConfig{
		Baseline:  OutcomeQualitySourceBinding{Target: baselineTarget, Workspace: baselineSource},
		Candidate: OutcomeQualitySourceBinding{Target: candidateTarget, Workspace: candidateSource},
		Policy:    policy,
		Command:   "exact-source-pair: v3 cmd/picogent + v4 outcome-quality-worker",
		ScenarioInput: func(scenario OutcomeQualityScenario) (OutcomeQualityInput, error) {
			return outcomeQualityLegacyInput(scenario), nil
		},
	})
	if err != nil {
		t.Fatalf("run exact source-pair matrix: %v", err)
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("validate exact source-pair report: %v", err)
	}
	if err := finalizeOutcomeQualityExactReport(context.Background(), report, outcomeQualityExactReportFinalization{
		Policy:               policy,
		Baseline:             OutcomeQualitySourceBinding{Target: baselineTarget, Workspace: baselineSource},
		Candidate:            OutcomeQualitySourceBinding{Target: candidateTarget, Workspace: candidateSource},
		ProviderRequestCount: provider.requestCount(),
		ValidateSourcePair:   ValidateOutcomeQualitySourcePair,
		Persist:              persistOutcomeQualityExactReport,
	}); err != nil {
		t.Fatal(err)
	}

}

type outcomeQualityExactReportFinalization struct {
	Policy               OutcomeQualityPolicy
	Baseline             OutcomeQualitySourceBinding
	Candidate            OutcomeQualitySourceBinding
	ProviderRequestCount int
	ValidateSourcePair   func(context.Context, OutcomeQualitySourceBinding, OutcomeQualitySourceBinding) error
	Persist              func(OutcomeQualityReport) error
}

func finalizeOutcomeQualityExactReport(ctx context.Context, report OutcomeQualityReport, cfg outcomeQualityExactReportFinalization) error {
	wantObservations := len(DefaultOutcomeQualityScenarios()) * 2 * cfg.Policy.Repetitions
	if len(report.Observations) != wantObservations {
		return fmt.Errorf("observations=%d, want %d", len(report.Observations), wantObservations)
	}
	if report.Status != OutcomeReportComplete || len(report.Unverified) != 0 {
		return fmt.Errorf("report status=%q unverified=%v, want complete controlled transcript evidence", report.Status, report.Unverified)
	}

	legacyObservations := 0
	candidateObservations := 0
	for _, observation := range report.Observations {
		switch observation.Variant {
		case OutcomeVariantBaseline:
			legacyObservations++
			if observation.SourceHead != OutcomeQualityLegacySourceHead ||
				observation.Metrics.OutcomeSuccess != OutcomeAssessmentPass ||
				observation.Metrics.Correctness != OutcomeAssessmentPass ||
				observation.Metrics.VerificationQuality != OutcomeVerificationPass ||
				observation.Metrics.Evidence != EvidenceCurrent ||
				len(observation.Unverified) != 0 {
				return fmt.Errorf("legacy observation=%#v, want exact head and complete controlled transcript metrics", observation)
			}
		case OutcomeVariantCandidate:
			candidateObservations++
			if observation.SourceHead != outcomeQualityExactCandidateHead ||
				observation.Metrics.OutcomeSuccess != OutcomeAssessmentPass ||
				observation.Metrics.Correctness != OutcomeAssessmentPass ||
				observation.Metrics.VerificationQuality != OutcomeVerificationPass ||
				observation.Metrics.Evidence != EvidenceCurrent ||
				len(observation.Unverified) != 0 {
				return fmt.Errorf("candidate observation=%#v, want exact head and complete controlled transcript metrics", observation)
			}
		default:
			return fmt.Errorf("unexpected observation variant %q", observation.Variant)
		}
	}
	if legacyObservations != len(DefaultOutcomeQualityScenarios())*cfg.Policy.Repetitions || candidateObservations != legacyObservations {
		return fmt.Errorf("legacy observations=%d candidate observations=%d, want %d each", legacyObservations, candidateObservations, len(DefaultOutcomeQualityScenarios())*cfg.Policy.Repetitions)
	}
	if cfg.ProviderRequestCount != legacyObservations*4 {
		return fmt.Errorf("legacy provider requests=%d, want four calls per v3 observation (%d)", cfg.ProviderRequestCount, legacyObservations*4)
	}
	if cfg.ValidateSourcePair == nil {
		return fmt.Errorf("exact source-pair validator is required")
	}
	if err := cfg.ValidateSourcePair(ctx, cfg.Baseline, cfg.Candidate); err != nil {
		return fmt.Errorf("source pair changed during exact matrix: %w", err)
	}
	if cfg.Persist == nil {
		return fmt.Errorf("exact source-pair report writer is required")
	}
	return cfg.Persist(report)
}

func persistOutcomeQualityExactReport(report OutcomeQualityReport) error {
	reportPath := strings.TrimSpace(os.Getenv("PICOGENT_OUTCOME_QUALITY_REPORT"))
	if reportPath == "" {
		return nil
	}
	return writeOutcomeQualityExactReport(report, reportPath)
}

func writeOutcomeQualityExactReport(report OutcomeQualityReport, reportPath string) error {
	if !filepath.IsAbs(reportPath) {
		return fmt.Errorf("PICOGENT_OUTCOME_QUALITY_REPORT must be absolute, got %q", reportPath)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode exact source-pair report: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(reportPath, data, outcomeQualityExactReportFileMode); err != nil {
		return fmt.Errorf("write exact source-pair report: %w", err)
	}
	return nil
}

func TestFinalizeOutcomeQualityExactReportPersistsOnlyAfterFinalValidation(t *testing.T) {
	report, policy := completeOutcomeQualityExactReport(t)

	t.Run("does not write after an observation-count failure", func(t *testing.T) {
		reportPath := filepath.Join(t.TempDir(), "outcome-quality-report.json")
		incomplete := report
		incomplete.Observations = incomplete.Observations[:len(incomplete.Observations)-1]
		persisted := false

		err := finalizeOutcomeQualityExactReport(context.Background(), incomplete, outcomeQualityExactReportFinalization{
			Policy:               policy,
			ProviderRequestCount: 0,
			ValidateSourcePair: func(context.Context, OutcomeQualitySourceBinding, OutcomeQualitySourceBinding) error {
				t.Fatal("source-pair validation must not run after an observation-count failure")
				return nil
			},
			Persist: func(report OutcomeQualityReport) error {
				persisted = true
				return writeOutcomeQualityExactReport(report, reportPath)
			},
		})
		if err == nil || !strings.Contains(err.Error(), "observations=") {
			t.Fatalf("finalization error=%v, want observation-count failure", err)
		}
		if persisted {
			t.Fatal("report writer ran after an observation-count failure")
		}
		if _, statErr := os.Stat(reportPath); !os.IsNotExist(statErr) {
			t.Fatalf("report path exists after an observation-count failure: %v", statErr)
		}
	})

	t.Run("does not write after a post-run source-pair failure", func(t *testing.T) {
		reportPath := filepath.Join(t.TempDir(), "outcome-quality-report.json")
		persisted := false

		err := finalizeOutcomeQualityExactReport(context.Background(), report, outcomeQualityExactReportFinalization{
			Policy:               policy,
			ProviderRequestCount: len(DefaultOutcomeQualityScenarios()) * policy.Repetitions * 4,
			ValidateSourcePair: func(context.Context, OutcomeQualitySourceBinding, OutcomeQualitySourceBinding) error {
				return fmt.Errorf("source worktree is no longer clean")
			},
			Persist: func(report OutcomeQualityReport) error {
				persisted = true
				return writeOutcomeQualityExactReport(report, reportPath)
			},
		})
		if err == nil || !strings.Contains(err.Error(), "source pair changed during exact matrix") {
			t.Fatalf("finalization error=%v, want source-pair failure", err)
		}
		if persisted {
			t.Fatal("report writer ran after a source-pair failure")
		}
		if _, statErr := os.Stat(reportPath); !os.IsNotExist(statErr) {
			t.Fatalf("report path exists after a source-pair failure: %v", statErr)
		}
	})

	t.Run("writes one valid report after every final check passes", func(t *testing.T) {
		reportPath := filepath.Join(t.TempDir(), "outcome-quality-report.json")
		writes := 0

		err := finalizeOutcomeQualityExactReport(context.Background(), report, outcomeQualityExactReportFinalization{
			Policy:               policy,
			Baseline:             OutcomeQualitySourceBinding{Target: outcomeQualityExactSourceTarget(OutcomeQualityLegacySourceHead), Workspace: "baseline"},
			Candidate:            OutcomeQualitySourceBinding{Target: outcomeQualityExactSourceTarget(outcomeQualityExactCandidateHead), Workspace: "candidate"},
			ProviderRequestCount: len(DefaultOutcomeQualityScenarios()) * policy.Repetitions * 4,
			ValidateSourcePair: func(_ context.Context, baseline, candidate OutcomeQualitySourceBinding) error {
				if baseline.Target.SourceHead != OutcomeQualityLegacySourceHead || candidate.Target.SourceHead != outcomeQualityExactCandidateHead {
					return fmt.Errorf("unexpected source heads baseline=%q candidate=%q", baseline.Target.SourceHead, candidate.Target.SourceHead)
				}
				return nil
			},
			Persist: func(report OutcomeQualityReport) error {
				writes++
				return writeOutcomeQualityExactReport(report, reportPath)
			},
		})
		if err != nil {
			t.Fatalf("finalize exact source-pair report: %v", err)
		}
		if writes != 1 {
			t.Fatalf("writes=%d, want 1", writes)
		}
		info, err := os.Stat(reportPath)
		if err != nil {
			t.Fatalf("stat persisted report: %v", err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != outcomeQualityExactReportFileMode {
			t.Fatalf("persisted report permissions=%#o, want %#o", info.Mode().Perm(), outcomeQualityExactReportFileMode)
		}
		data, err := os.ReadFile(reportPath)
		if err != nil {
			t.Fatalf("read persisted report: %v", err)
		}
		var got OutcomeQualityReport
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("decode persisted report: %v", err)
		}
		if err := got.Validate(); err != nil {
			t.Fatalf("validate persisted report: %v", err)
		}
		if got.Baseline.SourceHead != OutcomeQualityLegacySourceHead || got.Candidate.SourceHead != outcomeQualityExactCandidateHead {
			t.Fatalf("persisted report heads baseline=%q candidate=%q", got.Baseline.SourceHead, got.Candidate.SourceHead)
		}
	})
}

func completeOutcomeQualityExactReport(t *testing.T) (OutcomeQualityReport, OutcomeQualityPolicy) {
	t.Helper()
	cfg := testOutcomeQualityRunnerConfig(2)
	report, err := RunOutcomeQualityMatrix(context.Background(), cfg, NewOutcomeQualityAgentExecutor())
	if err != nil {
		t.Fatalf("run complete exact-report fixture: %v", err)
	}
	report.Baseline = outcomeQualityExactSourceTarget(OutcomeQualityLegacySourceHead)
	report.Candidate = outcomeQualityExactSourceTarget(outcomeQualityExactCandidateHead)
	for index := range report.Observations {
		switch report.Observations[index].Variant {
		case OutcomeVariantBaseline:
			report.Observations[index].SourceHead = OutcomeQualityLegacySourceHead
		case OutcomeVariantCandidate:
			report.Observations[index].SourceHead = outcomeQualityExactCandidateHead
		default:
			t.Fatalf("fixture observation variant=%q", report.Observations[index].Variant)
		}
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("validate complete exact-report fixture: %v", err)
	}
	return report, cfg.Policy
}

func outcomeQualityExactSourceTarget(head string) OutcomeQualityTarget {
	return OutcomeQualityTarget{
		SourceHead:  head,
		Host:        runtime.GOOS + "/" + runtime.GOARCH,
		GoVersion:   runtime.Version(),
		ToolVersion: OutcomeQualityRunnerToolVersion,
	}
}
