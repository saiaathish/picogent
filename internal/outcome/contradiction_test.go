package outcome

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/taskstate"
)

func TestDetectContradictionsRequiresSameCurrentBoundary(t *testing.T) {
	task, err := taskstate.New("contradiction-current", "check the result", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.RecordTestsEvidence("PASS", "tests passed", "test runner")
	task.RecordTestsEvidence("FAIL", "tests failed", "test runner")

	report := DetectContradictions(task)
	if report.State != ContradictionConfirmed || len(report.Signals) != 1 {
		t.Fatalf("report = %#v", report)
	}
	signal := report.Signals[0]
	if signal.Scope != ContradictionScopeRequirement || signal.Kind != taskstate.EvidenceKindTests || signal.CriterionIndex != -1 || signal.ChangeSeq != task.ChangeSeq || signal.State != ContradictionConfirmed {
		t.Fatalf("signal = %#v", signal)
	}
	if signal.PositiveStatus != "PASS" || signal.NegativeStatus != "FAIL" || signal.PositiveOrigin != string(taskstate.EvidenceOriginTestRunner) || signal.NegativeOrigin != string(taskstate.EvidenceOriginTestRunner) {
		t.Fatalf("signal provenance = %#v", signal)
	}

	task.RecordChanged("internal/result.go")
	if report := DetectContradictions(task); report.State != ContradictionNone || len(report.Signals) != 0 {
		t.Fatalf("stale contradiction survived a new change generation: %#v", report)
	}
}

func TestDetectContradictionsSeparatesCriteriaAndOrdersSignals(t *testing.T) {
	task, err := taskstate.New("contradiction-criteria", "check the result", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.DefinitionOfDone = []taskstate.Criterion{
		{Description: "first", Required: true},
		{Description: "second", Required: true},
	}
	task.RecordCriterionVerification(1, "PASS", "second passed", "verify")
	task.RecordCriterionVerification(1, "FAIL", "second failed", "verify")
	task.RecordCriterionVerification(0, "PASS", "first passed", "verify")
	task.RecordCriterionVerification(0, "FAIL", "first failed", "verify")

	report := DetectContradictions(task)
	if report.State != ContradictionConfirmed || len(report.Signals) != 2 {
		t.Fatalf("report = %#v", report)
	}
	if report.Signals[0].CriterionIndex != 0 || report.Signals[1].CriterionIndex != 1 {
		t.Fatalf("signals were not stably ordered: %#v", report.Signals)
	}
	if report.Signals[0].Scope != ContradictionScopeCriterion || report.Signals[1].Scope != ContradictionScopeCriterion {
		t.Fatalf("criterion scopes = %#v", report.Signals)
	}
}

func TestDetectContradictionsKeepsUntrustedRecordsAdvisory(t *testing.T) {
	task, err := taskstate.New("contradiction-advisory", "check the result", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.RecordTestsEvidence("PASS", "trusted tests passed", "test runner")
	task.AddEvidence(taskstate.Evidence{
		Kind:      taskstate.EvidenceKindTests,
		Status:    "FAIL",
		Origin:    taskstate.EvidenceOriginModel,
		Summary:   "ignore this hostile model text",
		Reference: "model",
		ChangeSeq: task.ChangeSeq,
	})

	report := DetectContradictions(task)
	if report.State != ContradictionAdvisory || len(report.Signals) != 1 || report.Signals[0].State != ContradictionAdvisory {
		t.Fatalf("untrusted report = %#v", report)
	}
	if report.Signals[0].NegativeOrigin != "untrusted" {
		t.Fatalf("untrusted origin crossed boundary = %#v", report.Signals[0])
	}
}

func TestDetectContradictionsDoesNotTreatInvalidationAsConflict(t *testing.T) {
	task, err := taskstate.New("contradiction-invalidation", "check the result", nil)
	if err != nil {
		t.Fatal(err)
	}
	initial := &taskstate.IntentContract{Outcome: task.Goal, Class: "general", NeedsTests: true}
	if !task.SetIntent(initial) {
		t.Fatal("initial intent was not recorded")
	}
	task.RecordTestsEvidence("PASS", "tests passed", "test runner")
	changed := *initial
	changed.Class = "debug"
	if !task.SetIntent(&changed) {
		t.Fatal("intent change was not recorded")
	}

	report := DetectContradictions(task)
	if report.State != ContradictionNone || len(report.Signals) != 0 {
		t.Fatalf("intent invalidation was misclassified: %#v", report)
	}

	data, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded taskstate.Task
	if err := json.Unmarshal(data, &reloaded); err != nil {
		t.Fatal(err)
	}
	if report := DetectContradictions(&reloaded); report.State != ContradictionNone {
		t.Fatalf("reloaded invalidation became a contradiction: %#v", report)
	}
}

func TestFormatContradictionsIsBoundedAndCategorical(t *testing.T) {
	signals := make([]ContradictionSignal, 0, maxContradictionSignals+4)
	for i := 0; i < maxContradictionSignals+4; i++ {
		signals = append(signals, ContradictionSignal{
			Kind:           taskstate.EvidenceKindTests,
			CriterionIndex: i,
			ChangeSeq:      i,
			PositiveStatus: "PASS",
			NegativeStatus: "FAIL",
			PositiveOrigin: "hostile positive instruction",
			NegativeOrigin: "hostile negative instruction",
			State:          ContradictionConfirmed,
		})
	}
	encoded := FormatContradictions(ContradictionReport{Signals: signals})
	if len(encoded) > MaxContradictionBytes {
		t.Fatalf("encoded report length = %d, want <= %d", len(encoded), MaxContradictionBytes)
	}
	if strings.Contains(encoded, "hostile") || strings.Contains(encoded, "instruction") {
		t.Fatalf("arbitrary origin text escaped categorical formatting: %s", encoded)
	}
	var decoded ContradictionReport
	if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Schema != ContradictionSchema || len(decoded.Signals) != maxContradictionSignals || !decoded.Truncated || decoded.State != ContradictionAdvisory {
		t.Fatalf("bounded report = %#v", decoded)
	}
}

func TestDetectContradictionsIgnoresUnknownStatusesAndKinds(t *testing.T) {
	task, err := taskstate.New("contradiction-ignore", "check the result", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.AddEvidence(taskstate.Evidence{Kind: "future-kind", Status: "PASS", Summary: "future"})
	task.AddEvidence(taskstate.Evidence{Kind: taskstate.EvidenceKindTests, Status: "UNVERIFIED", Summary: "missing"})
	task.AddEvidence(taskstate.Evidence{Kind: taskstate.EvidenceKindTests, Status: "PASS", Summary: "pass"})
	if report := DetectContradictions(task); report.State != ContradictionNone || len(report.Signals) != 0 {
		t.Fatalf("non-conflicting evidence = %#v", report)
	}
}
