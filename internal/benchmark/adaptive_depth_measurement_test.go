package benchmark

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/outcome"
)

func TestMeasureAdaptiveDepthCatalogIsBoundedAndExplicitlyInconclusive(t *testing.T) {
	report, err := MeasureAdaptiveDepthCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("measurement report: %v", err)
	}
	if report.Status != OutcomeReportInconclusive || report.QualityImpact != OutcomeAssessmentInconclusive {
		t.Fatalf("quality boundary = status=%q impact=%q", report.Status, report.QualityImpact)
	}

	classCounts := make(map[outcome.TaskDepthClass]int)
	fixedFailures := 0
	adaptivePasses := 0
	adaptiveWorkAboveFixed := false
	maxAdaptiveWork := 0
	for _, observation := range report.Observations {
		classCounts[observation.Profile.Class]++
		if observation.FixedBudgetAdequacy == OutcomeAssessmentFail {
			fixedFailures++
		}
		if observation.AdaptiveBudgetAdequacy == OutcomeAssessmentPass {
			adaptivePasses++
		}
		if observation.AdaptiveWorkUnits > observation.FixedWorkUnits {
			adaptiveWorkAboveFixed = true
		}
		if observation.AdaptiveWorkUnits > maxAdaptiveWork {
			maxAdaptiveWork = observation.AdaptiveWorkUnits
		}
		if observation.AdaptiveWorkUnits > report.MaxWorkUnits {
			t.Fatalf("unbounded adaptive work for %q: %d > %d", observation.ScenarioID, observation.AdaptiveWorkUnits, report.MaxWorkUnits)
		}
	}
	for _, class := range []outcome.TaskDepthClass{outcome.TaskDepthFocused, outcome.TaskDepthDeep, outcome.TaskDepthBroad} {
		if classCounts[class] == 0 {
			t.Fatalf("catalog did not exercise %q: counts=%v", class, classCounts)
		}
	}
	if fixedFailures == 0 || adaptivePasses != len(report.Observations) || !adaptiveWorkAboveFixed {
		t.Fatalf("adaptive routing proxy did not distinguish bounded work: fixed_failures=%d adaptive_passes=%d observations=%d work_delta=%v", fixedFailures, adaptivePasses, len(report.Observations), adaptiveWorkAboveFixed)
	}
	t.Logf("adaptive-depth proxy: scenarios=%d focused=%d deep=%d broad=%d fixed_underadequate=%d adaptive_adequate=%d fixed_work=%d max_adaptive_work=%d quality_impact=%s", len(report.Observations), classCounts[outcome.TaskDepthFocused], classCounts[outcome.TaskDepthDeep], classCounts[outcome.TaskDepthBroad], fixedFailures, adaptivePasses, report.Observations[0].FixedWorkUnits, maxAdaptiveWork, report.QualityImpact)
}

func TestMeasureAdaptiveDepthCatalogHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := MeasureAdaptiveDepthCatalog(ctx); err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("canceled measurement error=%v", err)
	}
}

func TestAdaptiveDepthMeasurementRoundTripsAndRejectsHostileProfiles(t *testing.T) {
	report, err := MeasureAdaptiveDepthCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "adaptive-depth-measurement:") {
		t.Fatalf("task-derived text leaked into measurement report: %s", data)
	}
	var decoded AdaptiveDepthMeasurementReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, report) {
		t.Fatalf("measurement report changed across JSON round trip: before=%#v after=%#v", report, decoded)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("round-tripped measurement report: %v", err)
	}

	hostile := report
	hostile.Observations = append([]AdaptiveDepthMeasurementObservation(nil), report.Observations...)
	hostile.Observations[0].Profile.Class = "execute-untrusted-plan"
	if err := hostile.Validate(); err == nil || !strings.Contains(err.Error(), "unknown class") {
		t.Fatalf("hostile class was accepted: %v", err)
	}

	hostile = report
	hostile.Observations = append([]AdaptiveDepthMeasurementObservation(nil), report.Observations...)
	hostile.Observations[0].Profile.Budget.QualityLoops = 99
	if err := hostile.Validate(); err == nil || !strings.Contains(err.Error(), "quality_loops") {
		t.Fatalf("hostile quality budget was accepted: %v", err)
	}
}
