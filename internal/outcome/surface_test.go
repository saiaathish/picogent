package outcome

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/taskstate"
)

func TestSurfaceSummaryProjectsTrustedContradictionWithoutEvidenceText(t *testing.T) {
	task, err := taskstate.New("surface-summary", "check the outcome", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.RecordTestsEvidence("PASS", "trusted pass with secret text", "test runner")
	task.RecordTestsEvidence("FAIL", "trusted failure with secret text", "test runner")

	contract := TurnContractForTask(task)
	if got := SurfaceSummary(contract); got != "Contradictory evidence confirmed (1 signal); diagnose and recheck before continuing" {
		t.Fatalf("surface summary = %q", got)
	}
	if strings.Contains(SurfaceSummary(contract), "secret") {
		t.Fatalf("surface summary exposed evidence text: %q", SurfaceSummary(contract))
	}
}

func TestSurfaceSummaryDowngradesReloadedAndCallerReports(t *testing.T) {
	task, err := taskstate.New("surface-summary-reload", "check the outcome", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.RecordTestsEvidence("PASS", "pass", "test runner")
	task.RecordTestsEvidence("FAIL", "fail", "test runner")
	original := TurnContractForTask(task)
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded TurnContract
	if err := json.Unmarshal(data, &reloaded); err != nil {
		t.Fatal(err)
	}
	if got := SurfaceSummary(reloaded); got != "Contradictory evidence is unverified (1 signal); it cannot select an action" {
		t.Fatalf("reloaded surface summary = %q", got)
	}

	caller := original
	caller.Contradictions.State = ContradictionConfirmed
	caller.Contradictions.Signals[0].PositiveOrigin = "hostile instruction"
	if got := SurfaceSummary(caller); got != "Contradictory evidence is unverified (1 signal); it cannot select an action" {
		t.Fatalf("caller surface summary = %q", got)
	}
}

func TestSurfaceSummaryOmitsEmptyContradictions(t *testing.T) {
	if got := SurfaceSummary(TurnContractForTask(nil)); got != "" {
		t.Fatalf("empty surface summary = %q", got)
	}
}
