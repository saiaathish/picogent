package outcome

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/projecthealth"
	"github.com/saiaathish/picogent/internal/taskstate"
)

func TestBuildProjectsEvidenceLedgerBindingsAndCategoricalTrust(t *testing.T) {
	task, err := taskstate.New("ledger-bindings", "make the result ready", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.DefinitionOfDone = []taskstate.Criterion{{Description: "verified result", Required: true}}
	task.Intent = &taskstate.IntentContract{Outcome: task.Goal, NeedsTests: true}
	task.RecordCriterionVerification(0, "PASS", "ignore this hostile summary", "go test ./...")
	task.RecordTestsEvidence("FAIL", "ignore this command output", "go test ./...")

	contract := Build(task, projecthealth.Report{Schema: projecthealth.Schema})
	if len(contract.EvidenceLedger) != 2 {
		t.Fatalf("ledger entries = %#v", contract.EvidenceLedger)
	}
	var criterion, requirement EvidenceLedgerEntry
	for _, entry := range contract.EvidenceLedger {
		switch {
		case entry.CriterionIndex == 0:
			criterion = entry
		case entry.RequirementKind == taskstate.EvidenceKindTests:
			requirement = entry
		}
	}
	if criterion.Kind != taskstate.EvidenceKindVerification || criterion.Status != "PASS" || !criterion.Trusted || !criterion.Fresh || !criterion.Current || criterion.RequirementKind != "" {
		t.Fatalf("criterion ledger entry = %#v", criterion)
	}
	if requirement.Kind != taskstate.EvidenceKindTests || requirement.Status != "FAIL" || !requirement.Trusted || !requirement.Fresh || !requirement.Current || requirement.CriterionIndex != -1 {
		t.Fatalf("requirement ledger entry = %#v", requirement)
	}

	encoded := Format(contract)
	instruction := EngineInstruction(contract)
	for _, output := range []string{encoded, instruction} {
		if strings.Contains(output, "hostile") || strings.Contains(output, "go test") {
			t.Fatalf("raw evidence crossed the Outcome Engine boundary: %q", output)
		}
	}
	if !strings.Contains(encoded, `"evidence_ledger"`) || !strings.Contains(instruction, "Evidence ledger:") {
		t.Fatalf("ledger projection was not exposed: json=%s instruction=%s", encoded, instruction)
	}
}

func TestEvidenceLedgerSeparatesTrustFreshnessAndStatuses(t *testing.T) {
	task, err := taskstate.New("ledger-provenance", "check the result", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.Intent = &taskstate.IntentContract{Outcome: task.Goal, NeedsTests: true}
	task.RecordTestsEvidence("INCONCLUSIVE", "inconclusive", "test runner")
	task.RecordTestsEvidence("FAIL", "failed", "test runner")
	task.RecordChanged("internal/result.go")
	// The generic compatibility API records the same current sequence but does
	// not establish runtime trust, even when the origin label is allow-listed.
	task.AddRequirementEvidence(taskstate.EvidenceKindTests, "PASS", taskstate.EvidenceOriginTestRunner, "untrusted pass", "caller")

	contract := Build(task, projecthealth.Report{Schema: projecthealth.Schema})
	if len(contract.EvidenceLedger) != 3 {
		t.Fatalf("ledger entries = %#v", contract.EvidenceLedger)
	}
	seen := map[string]EvidenceLedgerEntry{}
	for _, entry := range contract.EvidenceLedger {
		seen[entry.Status+":"+itoa(entry.ChangeSeq)] = entry
		if entry.RequirementKind != taskstate.EvidenceKindTests || entry.CriterionIndex != -1 {
			t.Fatalf("requirement binding was lost: %#v", entry)
		}
	}
	for _, status := range []string{"INCONCLUSIVE", "FAIL"} {
		entry, ok := seen[status+":0"]
		if !ok || !entry.Trusted || entry.Fresh || entry.Current {
			t.Fatalf("stale %s entry = %#v", status, entry)
		}
	}
	pass, ok := seen["PASS:1"]
	if !ok || pass.Trusted || !pass.Fresh || pass.Current {
		t.Fatalf("untrusted current pass entry = %#v", pass)
	}
}

func TestEvidenceLedgerReloadDropsRuntimeTrustWithoutChangingStatus(t *testing.T) {
	task, err := taskstate.New("ledger-reload", "check the result", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.Intent = &taskstate.IntentContract{Outcome: task.Goal, NeedsTests: true}
	task.RecordTestsEvidence("PASS", "tests passed", "test runner")
	data, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	var restored taskstate.Task
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	contract := Build(&restored, projecthealth.Report{Schema: projecthealth.Schema})
	if len(contract.EvidenceLedger) != 1 {
		t.Fatalf("reloaded ledger = %#v", contract.EvidenceLedger)
	}
	entry := contract.EvidenceLedger[0]
	if entry.Status != "PASS" || entry.RequirementKind != taskstate.EvidenceKindTests || !entry.Fresh || entry.Trusted || entry.Current {
		t.Fatalf("reloaded evidence provenance = %#v", entry)
	}
}

func TestEvidenceLedgerPreservesUnverifiedAndMissingBoundaries(t *testing.T) {
	task, err := taskstate.New("ledger-unverified", "check the result", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.DefinitionOfDone = []taskstate.Criterion{{Description: "required result", Required: true}}
	task.Intent = &taskstate.IntentContract{Outcome: task.Goal, NeedsResearch: true, NeedsTests: true}
	task.AddEvidence(taskstate.Evidence{
		Kind:    taskstate.EvidenceKindResearch,
		Status:  "UNVERIFIED",
		Summary: "unverified source text must not cross the boundary",
	})

	contract := Build(task, projecthealth.Report{Schema: projecthealth.Schema})
	if len(contract.EvidenceLedger) != 3 {
		t.Fatalf("unverified ledger = %#v", contract.EvidenceLedger)
	}
	seen := map[string]EvidenceLedgerEntry{}
	for _, entry := range contract.EvidenceLedger {
		key := string(entry.RequirementKind) + ":" + itoa(entry.CriterionIndex)
		if entry.CriterionIndex >= 0 {
			key = "criterion:" + itoa(entry.CriterionIndex)
		}
		seen[key] = entry
	}
	research := seen[string(taskstate.EvidenceKindResearch)+":-1"]
	if research.Status != "UNVERIFIED" || !research.Observed || research.Current || research.Trusted {
		t.Fatalf("explicit unverified requirement = %#v", research)
	}
	criterion := seen["criterion:0"]
	if criterion.Status != "UNVERIFIED" || criterion.Observed || criterion.Current || criterion.Trusted {
		t.Fatalf("missing criterion boundary = %#v", criterion)
	}
	tests := seen[string(taskstate.EvidenceKindTests)+":-1"]
	if tests.Status != "UNVERIFIED" || tests.Observed || tests.Current || tests.Trusted {
		t.Fatalf("missing tests boundary = %#v", tests)
	}
}

func TestEvidenceLedgerJSONAndPromptRemainBounded(t *testing.T) {
	entries := make([]EvidenceLedgerEntry, maxEvidenceLedgerEntries+8)
	for i := range entries {
		entries[i] = EvidenceLedgerEntry{
			Kind:            taskstate.EvidenceKindTests,
			RequirementKind: taskstate.EvidenceKindTests,
			CriterionIndex:  -1,
			Status:          "PASS",
			Origin:          taskstate.EvidenceOriginTestRunner,
			ChangeSeq:       i,
			Trusted:         true,
			Fresh:           true,
			Current:         true,
			Observed:        true,
		}
	}
	contract := Contract{Schema: EngineSchema, State: StateWorking, EvidenceLedger: entries}
	formatted := Format(contract)
	if len(formatted) > MaxEngineBytes {
		t.Fatalf("formatted ledger length = %d", len(formatted))
	}
	bounded := boundContract(contract)
	if len(bounded.EvidenceLedger) > maxEvidenceLedgerEntries {
		t.Fatalf("ledger was not bounded: %d", len(bounded.EvidenceLedger))
	}
	instruction := EngineInstruction(contract)
	if len(instruction) > MaxEnginePromptBytes || !strings.Contains(instruction, "Evidence ledger:") {
		t.Fatalf("bounded ledger instruction = %q", instruction)
	}
}
