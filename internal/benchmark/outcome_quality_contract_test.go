package benchmark

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestOutcomeQualityCatalogMatchesBriefCategories(t *testing.T) {
	catalog := DefaultOutcomeQualityScenarios()
	if len(catalog) != 20 {
		t.Fatalf("catalog length=%d, want 20", len(catalog))
	}

	wantCounts := map[OutcomeQualityScenarioCategory]int{
		OutcomeCategoryBeginner:            3,
		OutcomeCategoryStandardDevelopment: 4,
		OutcomeCategoryAdvanced:            4,
		OutcomeCategoryProduct:             3,
		OutcomeCategoryRobustness:          5,
		OutcomeCategoryLongHorizon:         1,
	}
	gotCounts := make(map[OutcomeQualityScenarioCategory]int, len(wantCounts))
	seenIDs := make(map[string]struct{}, len(catalog))
	for _, scenario := range catalog {
		if !scenario.Category.valid() || !scenario.Kind.valid() {
			t.Fatalf("catalog has unsupported scenario: %#v", scenario)
		}
		if _, ok := seenIDs[scenario.ID]; ok {
			t.Fatalf("catalog repeats scenario ID %q", scenario.ID)
		}
		seenIDs[scenario.ID] = struct{}{}
		gotCounts[scenario.Category]++
	}
	if fmt.Sprint(gotCounts) != fmt.Sprint(wantCounts) {
		t.Fatalf("category counts=%v, want %v", gotCounts, wantCounts)
	}

	catalog[0].ID = "mutated"
	if DefaultOutcomeQualityScenarios()[0].ID == "mutated" {
		t.Fatal("catalog accessor returned mutable process-wide state")
	}
}

func TestOutcomeQualityReportValidatesCompleteDeterministicMatrix(t *testing.T) {
	report := validOutcomeQualityReport()
	if err := report.Validate(); err != nil {
		t.Fatalf("valid report: %v", err)
	}

	first, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("report JSON is not deterministic across repeated marshals")
	}
	for _, field := range []string{`"baseline"`, `"candidate"`, `"input_sha256"`, `"outcome_success"`, `"verification_quality"`, `"context_growth_bytes"`} {
		if !strings.Contains(string(first), field) {
			t.Fatalf("report JSON missing required field %s", field)
		}
	}
}

func TestOutcomeQualityReportRequiresTwoRunsAndCompleteCoverage(t *testing.T) {
	report := validOutcomeQualityReport()
	report.Policy.Repetitions = 1
	if err := report.Validate(); err == nil || !strings.Contains(err.Error(), "repetitions") {
		t.Fatalf("one-run policy error=%v", err)
	}

	report = validOutcomeQualityReport()
	report.Observations = report.Observations[:len(report.Observations)-1]
	if err := report.Validate(); err == nil || !strings.Contains(err.Error(), "complete report") {
		t.Fatalf("incomplete complete-report error=%v", err)
	}

	report.Status = OutcomeReportUnverified
	report.Unverified = []string{"candidate process did not start"}
	if err := report.Validate(); err != nil {
		t.Fatalf("explicitly unverified partial report: %v", err)
	}

	report.Status = OutcomeReportInconclusive
	report.Unverified = nil
	if err := report.Validate(); err == nil || !strings.Contains(err.Error(), "inconclusive report") {
		t.Fatalf("unexplained inconclusive report error=%v", err)
	}

	report.Status = OutcomeReportUnverified
	if err := report.Validate(); err == nil || !strings.Contains(err.Error(), "unverified report") {
		t.Fatalf("unexplained unverified report error=%v", err)
	}
}

func TestOutcomeQualityReportRejectsMixedHeadsAndNondeterministicOrder(t *testing.T) {
	report := validOutcomeQualityReport()
	report.Observations[0].SourceHead = report.Candidate.SourceHead
	if err := report.Validate(); err == nil || !strings.Contains(err.Error(), "does not match baseline") {
		t.Fatalf("mixed baseline observation error=%v", err)
	}

	report = validOutcomeQualityReport()
	report.Observations[0], report.Observations[1] = report.Observations[1], report.Observations[0]
	if err := report.Validate(); err == nil || !strings.Contains(err.Error(), "deterministic") {
		t.Fatalf("reordered observations error=%v", err)
	}

	report = validOutcomeQualityReport()
	report.Scenarios[0].InputSHA256 = strings.Repeat("a", 63) + "F"
	if err := report.Validate(); err == nil || !strings.Contains(err.Error(), "input_sha256") {
		t.Fatalf("uppercase input digest error=%v", err)
	}

	report = validOutcomeQualityReport()
	report.Scenarios[0].Kind = OutcomeKindBug
	if err := report.Validate(); err == nil || !strings.Contains(err.Error(), "stable catalog") {
		t.Fatalf("catalog drift error=%v", err)
	}
}

func TestOutcomeQualityReportRequiresCurrentVerificationForSuccess(t *testing.T) {
	cases := []struct {
		name string
		edit func(*OutcomeQualityMetrics)
		want string
	}{
		{
			name: "passing verification with stale evidence",
			edit: func(metrics *OutcomeQualityMetrics) { metrics.Evidence = EvidenceStale },
			want: "passing verification",
		},
		{
			name: "passing correctness without verification",
			edit: func(metrics *OutcomeQualityMetrics) { metrics.VerificationQuality = OutcomeVerificationInconclusive },
			want: "passing correctness",
		},
		{
			name: "passing outcome without correctness",
			edit: func(metrics *OutcomeQualityMetrics) { metrics.Correctness = OutcomeAssessmentInconclusive },
			want: "passing outcome",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := validOutcomeQualityReport()
			tc.edit(&report.Observations[0].Metrics)
			if err := report.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("fail-open success error=%v", err)
			}
		})
	}

	report := validOutcomeQualityReport()
	report.InvariantFailures = []string{"candidate verification did not finish"}
	if err := report.Validate(); err == nil || !strings.Contains(err.Error(), "invariant failures") {
		t.Fatalf("success with invariant failure error=%v", err)
	}
}

func TestOutcomeQualityReportBoundsMetricsAndMetadata(t *testing.T) {
	cases := []struct {
		name string
		edit func(*OutcomeQualityReport)
		want string
	}{
		{
			name: "oversized command",
			edit: func(report *OutcomeQualityReport) { report.Command = strings.Repeat("x", MaxOutcomeQualityTextBytes+1) },
			want: "command exceeds",
		},
		{
			name: "negative metric",
			edit: func(report *OutcomeQualityReport) { report.Observations[0].Metrics.ToolCalls = -1 },
			want: "tool_calls",
		},
		{
			name: "budget overflow",
			edit: func(report *OutcomeQualityReport) {
				report.Observations[0].Metrics.Tokens = report.Policy.MaxTokens + 1
			},
			want: "shared benchmark budget",
		},
		{
			name: "latency exceeds shared timeout",
			edit: func(report *OutcomeQualityReport) {
				report.Observations[0].Metrics.LatencyMillis = report.Policy.TimeoutMillis + 1
			},
			want: "shared timeout",
		},
		{
			name: "too many observation notes",
			edit: func(report *OutcomeQualityReport) { report.Observations[0].Unverified = make([]string, 9) },
			want: "unverified=",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := validOutcomeQualityReport()
			tc.edit(&report)
			if err := report.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("bounds error=%v", err)
			}
		})
	}
}

func validOutcomeQualityReport() OutcomeQualityReport {
	const repetitions = 2
	report := OutcomeQualityReport{
		Schema:      OutcomeQualitySchema,
		ScenarioSet: OutcomeQualityScenarioSet,
		Status:      OutcomeReportComplete,
		Baseline: OutcomeQualityTarget{
			SourceHead:  strings.Repeat("a", 40),
			Host:        "darwin/arm64",
			GoVersion:   "go1.26.6",
			ToolVersion: "picogent-benchmark-v1",
		},
		Candidate: OutcomeQualityTarget{
			SourceHead:  strings.Repeat("b", 40),
			Host:        "darwin/arm64",
			GoVersion:   "go1.26.6",
			ToolVersion: "picogent-benchmark-v1",
		},
		Policy: OutcomeQualityPolicy{
			Repetitions:   repetitions,
			TimeoutMillis: 30_000,
			MaxTokens:     16_000,
			MaxModelCalls: 64,
			MaxToolCalls:  256,
			MaxTurns:      32,
		},
		Command: "go test ./internal/benchmark -run TestOutcomeQualityReport -count=2",
	}
	report.Scenarios = DefaultOutcomeQualityScenarios()
	for index := range report.Scenarios {
		report.Scenarios[index].InputSHA256 = fmt.Sprintf("%064x", index+1)
	}

	metrics := OutcomeQualityMetrics{
		OutcomeSuccess:      OutcomeAssessmentPass,
		Correctness:         OutcomeAssessmentPass,
		UserQuestions:       0,
		Tokens:              120,
		ModelCalls:          2,
		ToolCalls:           5,
		LatencyMillis:       40,
		ChangedLines:        8,
		UnnecessaryChanges:  0,
		VerificationQuality: OutcomeVerificationPass,
		RepairCount:         0,
		ContextGrowthBytes:  1024,
		Evidence:            EvidenceCurrent,
	}
	for _, scenario := range report.Scenarios {
		for _, variant := range []OutcomeQualityVariant{OutcomeVariantBaseline, OutcomeVariantCandidate} {
			for repetition := 1; repetition <= repetitions; repetition++ {
				sourceHead := report.Baseline.SourceHead
				if variant == OutcomeVariantCandidate {
					sourceHead = report.Candidate.SourceHead
				}
				report.Observations = append(report.Observations, OutcomeQualityObservation{
					ScenarioID: scenario.ID,
					Variant:    variant,
					Repetition: repetition,
					SourceHead: sourceHead,
					Metrics:    metrics,
				})
			}
		}
	}
	return report
}
