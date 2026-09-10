package benchmark

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/outcome"
)

func TestMeasureAdaptiveDepthQualityCatalogIsPairedAndBounded(t *testing.T) {
	report, err := MeasureAdaptiveDepthQualityCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("quality report: %v", err)
	}
	if report.Status != OutcomeReportComplete || report.QualityImpact != OutcomeAssessmentPass {
		t.Fatalf("quality result = status=%q impact=%q, want complete/pass", report.Status, report.QualityImpact)
	}
	wantObservations := len(DefaultOutcomeQualityScenarios()) * MaxAdaptiveDepthQualityRepetitions
	if len(report.Observations) != wantObservations {
		t.Fatalf("observations=%d, want %d repeated paired observations", len(report.Observations), wantObservations)
	}
	if len(report.Unverified) != 1 || report.Unverified[0] != adaptiveDepthQualityUnverifiedReason {
		t.Fatalf("generalization boundary = %v", report.Unverified)
	}

	classCounts := make(map[outcome.TaskDepthClass]int)
	fixedExhausted := 0
	adaptiveExhausted := 0
	qualityImprovements := 0
	for _, observation := range report.Observations {
		classCounts[observation.Profile.Class]++
		if observation.FixedBudgetExhausted {
			fixedExhausted++
		}
		if observation.AdaptiveBudgetExhausted {
			adaptiveExhausted++
		}
		if observation.QualityDelta == OutcomeAssessmentPass {
			qualityImprovements++
		}
		if len(observation.FixedUnverified) != 0 || len(observation.AdaptiveUnverified) != 0 {
			t.Fatalf("unexpected execution uncertainty for %q: fixed=%v adaptive=%v", observation.ScenarioID, observation.FixedUnverified, observation.AdaptiveUnverified)
		}
		if observation.Fixed.OutcomeSuccess != OutcomeAssessmentPass && !observation.FixedBudgetExhausted {
			t.Fatalf("fixed failure for %q was not an expected budget exhaustion: %#v", observation.ScenarioID, observation.Fixed)
		}
		if observation.Adaptive.OutcomeSuccess != OutcomeAssessmentPass || observation.Adaptive.Correctness != OutcomeAssessmentPass {
			t.Fatalf("adaptive execution did not pass for %q: %#v", observation.ScenarioID, observation.Adaptive)
		}
		if observation.AdaptiveMaxTurns < observation.FixedMaxTurns || observation.AdaptiveMaxTurns > report.Policy.MaxTurns {
			t.Fatalf("route limits for %q = fixed=%d adaptive=%d policy=%d", observation.ScenarioID, observation.FixedMaxTurns, observation.AdaptiveMaxTurns, report.Policy.MaxTurns)
		}
	}
	for _, class := range []outcome.TaskDepthClass{outcome.TaskDepthFocused, outcome.TaskDepthDeep, outcome.TaskDepthBroad} {
		if classCounts[class] == 0 {
			t.Fatalf("catalog did not exercise %q: %v", class, classCounts)
		}
	}
	if fixedExhausted != 13*MaxAdaptiveDepthQualityRepetitions || adaptiveExhausted != 0 || qualityImprovements != 13*MaxAdaptiveDepthQualityRepetitions {
		t.Fatalf("paired quality result did not distinguish routes: fixed_exhausted=%d adaptive_exhausted=%d improvements=%d", fixedExhausted, adaptiveExhausted, qualityImprovements)
	}
	t.Logf("adaptive-depth quality: catalog=%d observations=%d focused=%d deep=%d broad=%d fixed_exhausted=%d adaptive_exhausted=%d improvements=%d", len(DefaultOutcomeQualityScenarios()), len(report.Observations), classCounts[outcome.TaskDepthFocused], classCounts[outcome.TaskDepthDeep], classCounts[outcome.TaskDepthBroad], fixedExhausted, adaptiveExhausted, qualityImprovements)
}

func TestAdaptiveDepthQualityReportRoundTripAndBoundary(t *testing.T) {
	report, err := MeasureAdaptiveDepthQualityCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "adaptive-depth-measurement:") || strings.Contains(string(data), "Finish the deterministic benchmark") {
		t.Fatalf("fixture-derived text leaked into report: %s", data)
	}
	var decoded AdaptiveDepthQualityReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, report) {
		t.Fatalf("quality report changed across JSON round trip: unverified=%#v/%#v observations=%d/%d first signals=%#v/%#v first fixed_unverified=%#v/%#v first adaptive_unverified=%#v/%#v", report.Unverified, decoded.Unverified, len(report.Observations), len(decoded.Observations), report.Observations[0].Profile.Signals, decoded.Observations[0].Profile.Signals, report.Observations[0].FixedUnverified, decoded.Observations[0].FixedUnverified, report.Observations[0].AdaptiveUnverified, decoded.Observations[0].AdaptiveUnverified)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("round-tripped report: %v", err)
	}

	hostile := report
	hostile.Target.Host = "untrusted-host"
	if err := hostile.Validate(); err == nil || !strings.Contains(err.Error(), "target provenance") {
		t.Fatalf("mismatched target provenance was accepted: %v", err)
	}

	hostile = report
	hostile.Observations = append([]AdaptiveDepthQualityObservation(nil), report.Observations...)
	hostile.Observations[0].InputSHA256 = strings.Repeat("0", 64)
	if err := hostile.Validate(); err == nil || !strings.Contains(err.Error(), "stable fixture digest") {
		t.Fatalf("mismatched fixture digest was accepted: %v", err)
	}

	hostile = report
	hostile.Observations = append([]AdaptiveDepthQualityObservation(nil), report.Observations...)
	hostile.Observations[0].AdaptiveMaxTurns = report.Policy.MaxTurns + 1
	if err := hostile.Validate(); err == nil || !strings.Contains(err.Error(), "route limits") {
		t.Fatalf("over-budget route was accepted: %v", err)
	}
}

func TestMeasureAdaptiveDepthQualityCatalogHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := MeasureAdaptiveDepthQualityCatalog(ctx); err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("canceled measurement error=%v", err)
	}
}
