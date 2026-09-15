package benchmark

import (
	"context"
	"encoding/json"
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
	persistOutcomeQualityExactReport(t, report)
	wantObservations := len(DefaultOutcomeQualityScenarios()) * 2 * policy.Repetitions
	if len(report.Observations) != wantObservations {
		t.Fatalf("observations=%d, want %d", len(report.Observations), wantObservations)
	}
	if report.Status != OutcomeReportComplete || len(report.Unverified) != 0 {
		t.Fatalf("report status=%q unverified=%v, want complete controlled transcript evidence", report.Status, report.Unverified)
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
				t.Fatalf("legacy observation=%#v, want exact head and complete controlled transcript metrics", observation)
			}
		case OutcomeVariantCandidate:
			candidateObservations++
			if observation.SourceHead != outcomeQualityExactCandidateHead ||
				observation.Metrics.OutcomeSuccess != OutcomeAssessmentPass ||
				observation.Metrics.Correctness != OutcomeAssessmentPass ||
				observation.Metrics.VerificationQuality != OutcomeVerificationPass ||
				observation.Metrics.Evidence != EvidenceCurrent ||
				len(observation.Unverified) != 0 {
				t.Fatalf("candidate observation=%#v, want exact head and complete controlled transcript metrics", observation)
			}
		default:
			t.Fatalf("unexpected observation variant %q", observation.Variant)
		}
	}
	if legacyObservations != len(DefaultOutcomeQualityScenarios())*policy.Repetitions || candidateObservations != legacyObservations {
		t.Fatalf("legacy observations=%d candidate observations=%d, want %d each", legacyObservations, candidateObservations, len(DefaultOutcomeQualityScenarios())*policy.Repetitions)
	}
	if got := provider.requestCount(); got != legacyObservations*4 {
		t.Fatalf("legacy provider requests=%d, want four calls per v3 observation (%d)", got, legacyObservations*4)
	}
	if err := ValidateOutcomeQualitySourcePair(context.Background(),
		OutcomeQualitySourceBinding{Target: baselineTarget, Workspace: baselineSource},
		OutcomeQualitySourceBinding{Target: candidateTarget, Workspace: candidateSource},
	); err != nil {
		t.Fatalf("source pair changed during exact matrix: %v", err)
	}

}

func persistOutcomeQualityExactReport(t *testing.T, report OutcomeQualityReport) {
	t.Helper()
	reportPath := strings.TrimSpace(os.Getenv("PICOGENT_OUTCOME_QUALITY_REPORT"))
	if reportPath == "" {
		return
	}
	if !filepath.IsAbs(reportPath) {
		t.Fatalf("PICOGENT_OUTCOME_QUALITY_REPORT must be absolute, got %q", reportPath)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("encode exact source-pair report: %v", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(reportPath, data, 0o600); err != nil {
		t.Fatalf("write exact source-pair report: %v", err)
	}
}

func outcomeQualityExactSourceTarget(head string) OutcomeQualityTarget {
	return OutcomeQualityTarget{
		SourceHead:  head,
		Host:        runtime.GOOS + "/" + runtime.GOARCH,
		GoVersion:   runtime.Version(),
		ToolVersion: OutcomeQualityRunnerToolVersion,
	}
}
