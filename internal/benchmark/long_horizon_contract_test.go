package benchmark

import (
	"strings"
	"testing"
)

func TestLongHorizonReportValidatesExactHeadAndMonotonicTurnIdentity(t *testing.T) {
	report := validLongHorizonReport()
	if err := report.Validate(); err != nil {
		t.Fatalf("valid report: %v", err)
	}
	report.SourceHead = "abc123"
	if err := report.Validate(); err == nil || !strings.Contains(err.Error(), "full 40-character commit SHA") {
		t.Fatalf("abbreviated source head error=%v", err)
	}

	report = validLongHorizonReport()
	report.Observations[1].Turn = 3
	if err := report.Validate(); err == nil || !strings.Contains(err.Error(), "turn=3, want 2") {
		t.Fatalf("non-monotonic turn identity error=%v", err)
	}

	report = validLongHorizonReport()
	report.Observations[1].TurnRevision = report.Observations[0].TurnRevision
	if err := report.Validate(); err == nil || !strings.Contains(err.Error(), "not strictly increasing") {
		t.Fatalf("non-monotonic revision error=%v", err)
	}
}

func TestLongHorizonCompletionEligibilityFailsClosedForStaleProof(t *testing.T) {
	cases := []struct {
		name string
		edit func(*TurnObservation)
	}{
		{name: "criteria incomplete", edit: func(observation *TurnObservation) { observation.CriteriaComplete = false }},
		{name: "stale evidence", edit: func(observation *TurnObservation) {
			observation.Evidence = EvidenceStale
			observation.VerifiedMutationSeq = 1
			observation.MutationSeq = 2
		}},
		{name: "missing evidence", edit: func(observation *TurnObservation) { observation.Evidence = EvidenceMissing }},
		{name: "unverified evidence", edit: func(observation *TurnObservation) { observation.Evidence = EvidenceUnverified }},
		{name: "pending recovery", edit: func(observation *TurnObservation) { observation.Recovery = RecoveryPending }},
		{name: "unknown stop", edit: func(observation *TurnObservation) { observation.Stop = StopUnknown }},
		{name: "pause decision", edit: func(observation *TurnObservation) { observation.Stop = StopPause }},
		{name: "continue decision", edit: func(observation *TurnObservation) { observation.Stop = StopContinue }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			observation := validLongHorizonReport().Observations[0]
			tc.edit(&observation)
			if observation.CanStop() {
				t.Fatalf("observation incorrectly eligible: %#v", observation)
			}
		})
	}

	current := validLongHorizonReport().Observations[0]
	if !current.CanStop() || !current.CompletionEligible {
		t.Fatalf("fresh complete observation should be eligible: %#v", current)
	}
}

func TestLongHorizonReportRejectsCurrentEvidenceWithStaleMutationSequence(t *testing.T) {
	report := validLongHorizonReport()
	report.Observations[0].MutationSeq = 4
	report.Observations[0].VerifiedMutationSeq = 3
	report.Observations[0].CompletionEligible = false
	if err := report.Validate(); err == nil || !strings.Contains(err.Error(), "marks evidence current") {
		t.Fatalf("stale current-evidence error=%v", err)
	}
}

func TestLongHorizonReportKeepsRecordedInvariantFailuresFailClosed(t *testing.T) {
	report := validLongHorizonReport()
	report.InvariantFailures = []string{"turn 2 lost recovery evidence"}
	report.Observations[0].CompletionEligible = false
	report.Observations[0].Stop = StopContinue
	if err := report.Validate(); err != nil {
		t.Fatalf("failure report should remain structurally valid: %v", err)
	}

	report = validLongHorizonReport()
	report.InvariantFailures = []string{"turn 2 lost recovery evidence"}
	if err := report.Validate(); err == nil || !strings.Contains(err.Error(), "while invariant failures are recorded") {
		t.Fatalf("false completion with invariant failure error=%v", err)
	}
}

func TestLongHorizonReportBoundsRawMetadataBeforeTrimming(t *testing.T) {
	report := validLongHorizonReport()
	report.Command = strings.Repeat(" ", MaxLongHorizonTextBytes) + "x"
	if err := report.Validate(); err == nil || !strings.Contains(err.Error(), "command exceeds") {
		t.Fatalf("oversized raw command error=%v", err)
	}

	report = validLongHorizonReport()
	report.Unverified = []string{strings.Repeat(" ", MaxLongHorizonTextBytes) + "x"}
	if err := report.Validate(); err == nil || !strings.Contains(err.Error(), "unverified[0] exceeds") {
		t.Fatalf("oversized raw unverified entry error=%v", err)
	}
}

func validLongHorizonReport() Report {
	return Report{
		Schema:       LongHorizonSchema,
		Scenario:     "deterministic-outcome-lifecycle",
		SourceHead:   strings.Repeat("a", 40),
		BaselineHead: strings.Repeat("b", 40),
		Host:         "darwin/arm64",
		GoVersion:    "go1.26.6",
		Command:      "go test ./internal/benchmark -run TestLongHorizonOutcome",
		Observations: []TurnObservation{{
			Turn:                1,
			TurnRevision:        1,
			Events:              []ScenarioEvent{EventPlan, EventVerification, EventStop},
			CriteriaComplete:    true,
			MutationSeq:         0,
			VerifiedMutationSeq: 0,
			Evidence:            EvidenceCurrent,
			Recovery:            RecoveryNotRequired,
			Stop:                StopRecheck,
			CompletionEligible:  true,
		}, {
			Turn:                2,
			TurnRevision:        2,
			Events:              []ScenarioEvent{EventRestart, EventSteering},
			CriteriaComplete:    false,
			MutationSeq:         0,
			VerifiedMutationSeq: 0,
			Evidence:            EvidenceMissing,
			Recovery:            RecoveryNotRequired,
			Stop:                StopContinue,
			CompletionEligible:  false,
		}},
	}
}
