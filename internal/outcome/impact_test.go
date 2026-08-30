package outcome

import (
	"testing"

	"github.com/saiaathish/picogent/internal/taskstate"
)

func TestPredictImpactClassifiesSecurityAndConcurrencySignals(t *testing.T) {
	task := &taskstate.Task{
		ChangedFiles: []string{"internal/auth/session.go", "internal/auth/session_test.go"},
		ChangeSeq:    2,
		Intent:       &taskstate.IntentContract{Risk: "high"},
	}

	got := PredictImpact(task)
	if got.Scope != ImpactFocused || got.Risk != ImpactRiskHigh || got.Confidence != "high" {
		t.Fatalf("impact identity = %#v", got)
	}
	for _, area := range []ImpactArea{ImpactAreaSecurity, ImpactAreaConcurrency, ImpactAreaTests, ImpactAreaSource} {
		if !containsImpactArea(got.Areas, area) {
			t.Fatalf("impact areas=%v missing %q", got.Areas, area)
		}
	}
	for _, check := range []ImpactCheck{ImpactCheckTargetedTests, ImpactCheckSecurity, ImpactCheckConcurrency} {
		if !containsImpactCheck(got.Verification, check) {
			t.Fatalf("verification=%v missing %q", got.Verification, check)
		}
	}
	if !containsImpactCheck(got.Review, ImpactCheckTargetedReview) || !containsImpactCheck(got.Review, ImpactCheckSecurity) || got.Checkpoint != ImpactCheckpointHighRisk {
		t.Fatalf("review/checkpoint = %#v", got)
	}
}

func TestPredictImpactEscalatesCrossAreaAndBroadChanges(t *testing.T) {
	crossArea := &taskstate.Task{
		ChangedFiles: []string{"cmd/app.go", "internal/app.go", "web/app.tsx", "config.yml", "docs/launch.md"},
		ChangeSeq:    5,
	}
	got := PredictImpact(crossArea)
	if got.Scope != ImpactCrossArea || got.Risk != ImpactRiskMedium || got.Checkpoint != ImpactCheckpointCrossArea {
		t.Fatalf("cross-area impact = %#v", got)
	}
	if !containsImpactCheck(got.Verification, ImpactCheckBroader) || !containsImpactCheck(got.Review, ImpactCheckBroaderReview) {
		t.Fatalf("cross-area checks = %#v", got)
	}

	broad := &taskstate.Task{ChangedFilesCapped: true, ChangeSeq: 129}
	got = PredictImpact(broad)
	if got.Scope != ImpactUnknown || got.Risk != ImpactRiskMedium || got.Confidence != "low" || got.Checkpoint != ImpactCheckpointRecheck {
		t.Fatalf("unknown capped impact = %#v", got)
	}
	if !containsImpactCheck(got.Verification, ImpactCheckBroader) || !containsImpactCheck(got.Review, ImpactCheckBroaderReview) {
		t.Fatalf("unknown capped checks = %#v", got)
	}

	broad.ChangedFiles = []string{"a.go", "b.go", "c.go", "d.go", "e.go", "f.go", "g.go", "h.go", "i.go"}
	got = PredictImpact(broad)
	if got.Scope != ImpactBroad || got.Checkpoint != ImpactCheckpointBroad || got.Confidence != "medium" {
		t.Fatalf("broad impact = %#v", got)
	}
}

func TestPredictImpactKeepsUnknownAndEmptyStateConservative(t *testing.T) {
	if got := PredictImpact(nil); got.Scope != ImpactNone || got.Risk != ImpactRiskLow || got.Checkpoint != ImpactCheckpointNone {
		t.Fatalf("nil impact = %#v", got)
	}
	task := &taskstate.Task{ChangeSeq: 1, Intent: &taskstate.IntentContract{Risk: "high"}}
	got := PredictImpact(task)
	if got.Scope != ImpactUnknown || got.Risk != ImpactRiskHigh || got.Checkpoint != ImpactCheckpointRecheck {
		t.Fatalf("unknown impact = %#v", got)
	}
	if got.ChangedFiles != 0 || got.Confidence != "low" {
		t.Fatalf("unknown impact metadata = %#v", got)
	}
}

func TestPredictImpactTreatsPartialPathMetadataAsUncertain(t *testing.T) {
	task := &taskstate.Task{
		ChangedFiles: []string{"internal/app.go", "  "},
		ChangeSeq:    2,
	}

	got := PredictImpact(task)
	if got.Scope != ImpactFocused || got.Confidence != "medium" {
		t.Fatalf("partial path impact = %#v", got)
	}
	if !containsImpactCheck(got.Verification, ImpactCheckBroader) || !containsImpactCheck(got.Review, ImpactCheckBroaderReview) {
		t.Fatalf("partial path checks = %#v", got)
	}
}

func containsImpactArea(values []ImpactArea, wanted ImpactArea) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func containsImpactCheck(values []ImpactCheck, wanted ImpactCheck) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
