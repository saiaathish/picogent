package outcome

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/projecthealth"
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

func TestDetectContradictionsRetainsConfirmedRoutingWhenSignalsAreTruncated(t *testing.T) {
	task, err := taskstate.New("contradiction-cap", "check the result", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.DefinitionOfDone = make([]taskstate.Criterion, maxContradictionSignals+1)
	for index := range task.DefinitionOfDone {
		task.DefinitionOfDone[index] = taskstate.Criterion{Description: "criterion", Required: true}
	}
	for index := 0; index < maxContradictionSignals; index++ {
		task.AddEvidenceForCriterion(index, taskstate.Evidence{
			Kind:      taskstate.EvidenceKindTests,
			Status:    "PASS",
			Origin:    taskstate.EvidenceOriginTestRunner,
			Summary:   "advisory pass",
			Reference: "test runner",
			ChangeSeq: task.ChangeSeq,
		})
		task.AddEvidenceForCriterion(index, taskstate.Evidence{
			Kind:      taskstate.EvidenceKindTests,
			Status:    "FAIL",
			Origin:    taskstate.EvidenceOriginModel,
			Summary:   "advisory failure",
			Reference: "model",
			ChangeSeq: task.ChangeSeq,
		})
	}
	// Build the final trusted pair separately, then append it to this synthetic
	// over-capacity ledger. The detector must preserve its authority even when a
	// persisted or forward-compatible caller presents more evidence than the
	// normal task mutation helper retains.
	trustedTask, err := taskstate.New("trusted-contradiction-cap", "check the result", nil)
	if err != nil {
		t.Fatal(err)
	}
	trustedTask.DefinitionOfDone = []taskstate.Criterion{{Description: "trusted", Required: true}}
	trustedTask.RecordCriterionTestsEvidence(0, "PASS", "trusted pass", "test runner")
	trustedTask.RecordCriterionTestsEvidence(0, "FAIL", "trusted failure", "test runner")
	index := maxContradictionSignals
	positive := trustedTask.Evidence[0]
	negative := trustedTask.Evidence[1]
	positive.CriterionIndex = &index
	negative.CriterionIndex = &index
	task.Evidence = append(task.Evidence, positive, negative)

	report := DetectContradictions(task)
	if report.State != ContradictionConfirmed || len(report.Signals) != maxContradictionSignals || !report.Truncated {
		t.Fatalf("truncated contradiction report = %#v", report)
	}
	if decision := Select(task, projecthealth.Report{Schema: projecthealth.Schema}); decision.Kind != KindContradiction {
		t.Fatalf("hidden confirmed contradiction lost routing = %#v", decision)
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

func TestDetectContradictionsProtectsRuntimeTrustFromSignalMutation(t *testing.T) {
	task, err := taskstate.New("contradiction-signal-mutation", "check the result", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.RecordTestsEvidence("PASS", "tests passed", "test runner")
	task.RecordTestsEvidence("FAIL", "tests failed", "test runner")

	report := DetectContradictions(task)
	if report.State != ContradictionConfirmed || len(report.Signals) != 1 {
		t.Fatalf("confirmed report = %#v", report)
	}
	report.Signals[0].PositiveStatus = "APPROVED"
	report.Signals[0].PositiveOrigin = string(taskstate.EvidenceOriginUser)
	formatted := FormatContradictions(report)
	var decoded ContradictionReport
	if err := json.Unmarshal([]byte(formatted), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.State != ContradictionAdvisory || decoded.Signals[0].State != ContradictionAdvisory {
		t.Fatalf("mutated trusted signal remained confirmed = %#v", decoded)
	}
}

func TestDetectContradictionsRecognizesDeniedApproval(t *testing.T) {
	task, err := taskstate.New("contradiction-approval", "check the result", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.RecordApprovalEvidence("APPROVED", "approval granted", "user")
	task.RecordApprovalEvidence("DENIED", "approval denied", "user")

	report := DetectContradictions(task)
	if report.State != ContradictionConfirmed || len(report.Signals) != 1 {
		t.Fatalf("approval contradiction = %#v", report)
	}
	if report.Signals[0].NegativeStatus != "DENIED" || report.Signals[0].Scope != ContradictionScopeRequirement {
		t.Fatalf("approval contradiction signal = %#v", report.Signals[0])
	}
}

func TestDetectContradictionsResetsAllInvalidatedRequirementAliases(t *testing.T) {
	task, err := taskstate.New("contradiction-alias-invalidation", "check the result", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.Intent = &taskstate.IntentContract{Outcome: task.Goal, NeedsTests: true}
	task.RecordTestsEvidence("PASS", "tests passed", "test runner")
	task.AddVerification("go test ./...", true, "verification passed")
	if !task.InvalidateWorkspaceEvidence("workspace restored") {
		t.Fatal("workspace evidence was not invalidated")
	}
	task.RecordTestsEvidence("FAIL", "tests failed", "test runner")

	if report := DetectContradictions(task); report.State != ContradictionNone || len(report.Signals) != 0 {
		t.Fatalf("invalidated tests alias became a contradiction = %#v", report)
	}
}

func TestDetectContradictionsResetsInvalidatedCriterionTestEvidence(t *testing.T) {
	task, err := taskstate.New("criterion-test-invalidation", "check the result", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.DefinitionOfDone = []taskstate.Criterion{{Description: "required test proof", Required: true}}
	task.RecordCriterionTestsEvidence(0, "PASS", "tests passed", "test runner")
	if !task.InvalidateWorkspaceEvidence("workspace restored") {
		t.Fatal("workspace evidence was not invalidated")
	}
	task.RecordCriterionTestsEvidence(0, "FAIL", "tests failed after restore", "test runner")

	if report := DetectContradictions(task); report.State != ContradictionNone || len(report.Signals) != 0 {
		t.Fatalf("invalidated criterion test evidence became a contradiction = %#v", report)
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

func TestDetectContradictionsDoesNotHonorUntrustedInvalidationSpoof(t *testing.T) {
	task, err := taskstate.New("contradiction-invalidation-spoof", "check the result", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.RecordTestsEvidence("PASS", "tests passed", "test runner")
	task.AddEvidence(taskstate.Evidence{
		Kind:      taskstate.EvidenceKindTests,
		Status:    "INCONCLUSIVE",
		Source:    "outcome-contract",
		Origin:    taskstate.EvidenceOriginSystem,
		Summary:   "caller supplied an invalidation-shaped record",
		Reference: "durable intent change",
		ChangeSeq: task.ChangeSeq,
	})
	task.RecordTestsEvidence("FAIL", "tests failed", "test runner")

	report := DetectContradictions(task)
	if report.State != ContradictionConfirmed || len(report.Signals) != 1 {
		t.Fatalf("untrusted invalidation spoof changed the current boundary: %#v", report)
	}
}

func TestFormatContradictionsDoesNotPromoteCallerReport(t *testing.T) {
	encoded := FormatContradictions(ContradictionReport{
		State: ContradictionConfirmed,
		Signals: []ContradictionSignal{{
			Kind:           taskstate.EvidenceKindTests,
			CriterionIndex: -1,
			ChangeSeq:      1,
			PositiveStatus: "PASS",
			NegativeStatus: "FAIL",
			PositiveOrigin: string(taskstate.EvidenceOriginTestRunner),
			NegativeOrigin: string(taskstate.EvidenceOriginTestRunner),
			State:          ContradictionConfirmed,
		}},
	})
	var report ContradictionReport
	if err := json.Unmarshal([]byte(encoded), &report); err != nil {
		t.Fatal(err)
	}
	if report.State != ContradictionAdvisory || len(report.Signals) != 1 || report.Signals[0].State != ContradictionAdvisory {
		t.Fatalf("caller-provided report was promoted to confirmed: %#v", report)
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
