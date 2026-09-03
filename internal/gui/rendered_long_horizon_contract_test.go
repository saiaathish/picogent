package gui

import (
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/benchmark"
	"github.com/saiaathish/picogent/internal/taskstate"
)

func TestRenderedLongHorizonReportValidatesAuthoritativeProjection(t *testing.T) {
	report := validRenderedLongHorizonReport()
	if err := report.Validate(); err != nil {
		t.Fatalf("valid rendered report: %v", err)
	}
}

func TestRenderedLongHorizonReportRejectsFalseCompletionAfterSteering(t *testing.T) {
	report := validRenderedLongHorizonReport()
	report.Observations[1].Rendered.CompletionReady = true
	report.Observations[1].Rendered.CompletionMarker = true
	if err := report.Validate(); err == nil || !strings.Contains(err.Error(), "disagrees with authoritative eligibility") {
		t.Fatalf("false rendered completion error = %v", err)
	}

	report = validRenderedLongHorizonReport()
	report.Observations[0].Rendered.CompletionMarker = false
	if err := report.Validate(); err == nil || !strings.Contains(err.Error(), "completion marker") {
		t.Fatalf("missing completion marker error = %v", err)
	}
}

func TestRenderedLongHorizonReportRequiresExplicitUnverifiedBoundary(t *testing.T) {
	report := validRenderedLongHorizonReport()
	report.SourceVerified = false
	if err := report.Validate(); err == nil || !strings.Contains(err.Error(), "explicit unverified boundary") {
		t.Fatalf("unverified source error = %v", err)
	}

	report.Unverified = []string{"compiled source revision was not observed"}
	if err := report.Validate(); err != nil {
		t.Fatalf("explicit unverified source boundary: %v", err)
	}

	report.SourceVerified = true
	report.SourceTreeDirty = true
	if err := report.Validate(); err == nil || !strings.Contains(err.Error(), "modified source tree") {
		t.Fatalf("dirty verified source error = %v", err)
	}
}

func TestRenderedLongHorizonReportRejectsUnprojectableTaskState(t *testing.T) {
	report := validRenderedLongHorizonReport()
	report.Observations[1].Rendered.TaskStatus = taskstate.Status("unknown")
	if err := report.Validate(); err == nil || !strings.Contains(err.Error(), "unknown rendered task status") {
		t.Fatalf("unknown task status error = %v", err)
	}

	report = validRenderedLongHorizonReport()
	report.Observations[1].Rendered.ProgressVisible = false
	if err := report.Validate(); err == nil || !strings.Contains(err.Error(), "without visible progress") {
		t.Fatalf("missing progress error = %v", err)
	}
}

func validRenderedLongHorizonReport() RenderedLongHorizonReport {
	return RenderedLongHorizonReport{
		Schema:         RenderedLongHorizonSchema,
		Scenario:       "rendered-multi-turn-outcome",
		SourceHead:     strings.Repeat("a", 40),
		SourceVerified: true,
		Host:           "darwin/arm64",
		Runtime:        "go1.26.6",
		BrowserSession: "codex/rendered-long-horizon",
		BrowserTab:     "task-owned-tab-1",
		ObservedAtUTC:  "2026-09-03T02:20:00Z",
		Command:        "go run -tags rendered_fixture ./cmd/picogent-rendered-fixture",
		Observations: []RenderedLongHorizonObservation{
			{
				Outcome: benchmark.TurnObservation{
					Turn:                1,
					TurnRevision:        1,
					Events:              []benchmark.ScenarioEvent{benchmark.EventPlan, benchmark.EventMutation, benchmark.EventVerification, benchmark.EventStop},
					CriteriaComplete:    true,
					MutationSeq:         1,
					VerifiedMutationSeq: 1,
					Evidence:            benchmark.EvidenceCurrent,
					Recovery:            benchmark.RecoveryNotRequired,
					Stop:                benchmark.StopRecheck,
					CompletionEligible:  true,
				},
				Rendered: RenderedLongHorizonProjection{
					TaskPresent:      true,
					TaskStatus:       taskstate.StatusDone,
					ProgressVisible:  true,
					CompletionReady:  true,
					CompletionMarker: true,
					ChangedFiles:     []string{"outcome.txt"},
				},
			},
			{
				Outcome: benchmark.TurnObservation{
					Turn:                2,
					TurnRevision:        2,
					Events:              []benchmark.ScenarioEvent{benchmark.EventSteering, benchmark.EventRestart, benchmark.EventRecovery},
					CriteriaComplete:    false,
					MutationSeq:         2,
					VerifiedMutationSeq: 1,
					Evidence:            benchmark.EvidenceStale,
					Recovery:            benchmark.RecoveryComplete,
					Stop:                benchmark.StopContinue,
					CompletionEligible:  false,
				},
				Rendered: RenderedLongHorizonProjection{
					TaskPresent:     true,
					TaskStatus:      taskstate.StatusWorking,
					ProgressVisible: true,
					ChangedFiles:    []string{"outcome.txt"},
				},
			},
		},
	}
}
