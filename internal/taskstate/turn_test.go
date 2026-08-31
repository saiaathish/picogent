package taskstate

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestIntentRevisionChangesOnlyWhenInterpretationChanges(t *testing.T) {
	task, err := New("intent-revision", "finish the requested change", nil)
	if err != nil {
		t.Fatal(err)
	}
	intent := &IntentContract{Outcome: task.Goal, Class: "general", Action: "implementation"}
	changedInitial := task.SetIntent(intent)
	if !changedInitial || task.IntentRevision != 1 {
		t.Fatalf("initial intent update = changed:%v revision:%d", changedInitial, task.IntentRevision)
	}
	if task.NeedsVerification() {
		t.Fatal("recording the initial intent made an untouched task need verification")
	}
	if task.SetIntent(intent) {
		t.Fatal("identical intent unexpectedly advanced revision")
	}
	changed := *intent
	changed.Class = "bug"
	if !task.SetIntent(&changed) || task.IntentRevision != 2 {
		t.Fatalf("changed intent = %#v", task)
	}
	if err := task.Validate(); err != nil {
		t.Fatalf("intent revision state invalid: %v", err)
	}
}

func TestIntentChangeInvalidatesCurrentQualityProof(t *testing.T) {
	task, err := New("intent-proof", "finish the requested change", nil)
	if err != nil {
		t.Fatal(err)
	}
	initial := &IntentContract{Outcome: task.Goal, Class: "general", NeedsTests: true}
	if !task.SetIntent(initial) {
		t.Fatal("initial intent was not recorded")
	}
	task.RecordTestsEvidence("PASS", "current tests passed", "go test ./...")
	if !task.CompletionReady() {
		t.Fatal("current quality proof did not complete the initial contract")
	}
	historical := len(task.Evidence)

	changed := *initial
	changed.Class = "bug"
	if !task.SetIntent(&changed) {
		t.Fatal("changed intent was not recorded")
	}
	if task.CompletionReady() {
		t.Fatal("quality proof from the previous contract remained completion-ready")
	}
	status, current, origin := task.RequirementEvidenceState(EvidenceKindTests)
	if status != "INCONCLUSIVE" || current || origin != EvidenceOriginSystem {
		t.Fatalf("invalidated quality proof = status=%q current=%v origin=%q", status, current, origin)
	}
	if len(task.Evidence) <= historical {
		t.Fatalf("contract change did not retain an invalidation record: %#v", task.Evidence)
	}
}

func TestIntentChangeInvalidatesCurrentCriterionProof(t *testing.T) {
	task, err := New("intent-criterion-proof", "finish the requested change", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.DefinitionOfDone = []Criterion{{Description: "required proof", Required: true}}
	if !task.SetIntent(&IntentContract{Outcome: task.Goal, Class: "general"}) {
		t.Fatal("initial intent was not recorded")
	}
	task.RecordCriterionVerification(0, "PASS", "criterion passed", "verify")
	if !task.CompletionReady() {
		t.Fatal("current criterion proof did not complete the initial contract")
	}

	changed := &IntentContract{Outcome: task.Goal, Class: "documentation"}
	if !task.SetIntent(changed) {
		t.Fatal("changed intent was not recorded")
	}
	status, current := task.CriterionEvidenceState(0)
	if status != "INCONCLUSIVE" || !current {
		t.Fatalf("invalidated criterion proof = status=%q current=%v", status, current)
	}
	if task.CompletionReady() {
		t.Fatal("criterion proof from the previous contract remained completion-ready")
	}
}

func TestEquivalentIntentDoesNotInvalidateProof(t *testing.T) {
	task, err := New("equivalent-intent", "finish the requested change", nil)
	if err != nil {
		t.Fatal(err)
	}
	initial := &IntentContract{Outcome: task.Goal, Class: "general", NeedsTests: true}
	if !task.SetIntent(initial) {
		t.Fatal("initial intent was not recorded")
	}
	task.RecordTestsEvidence("PASS", "current tests passed", "go test ./...")
	historical := len(task.Evidence)

	equivalent := *initial
	equivalent.Outcome = "  " + task.Goal + "  "
	if task.SetIntent(&equivalent) {
		t.Fatal("equivalent normalized intent advanced the contract")
	}
	if len(task.Evidence) != historical || !task.CompletionReady() {
		t.Fatalf("equivalent intent invalidated proof: evidence=%#v ready=%v", task.Evidence, task.CompletionReady())
	}
}

func TestIntentProofInvalidationPersistsAcrossReload(t *testing.T) {
	task, err := New("intent-reload", "finish the requested change", nil)
	if err != nil {
		t.Fatal(err)
	}
	initial := &IntentContract{Outcome: task.Goal, Class: "general", NeedsTests: true}
	if !task.SetIntent(initial) {
		t.Fatal("initial intent was not recorded")
	}
	task.RecordTestsEvidence("PASS", "current tests passed", "go test ./...")
	store := NewStore(t.TempDir())
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}

	changed := *initial
	changed.Class = "bug"
	if !task.SetIntent(&changed) {
		t.Fatal("changed intent was not recorded")
	}
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}
	restored, err := store.Load(task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	status, current, origin := restored.RequirementEvidenceState(EvidenceKindTests)
	if status != "INCONCLUSIVE" || current || origin != EvidenceOriginSystem {
		t.Fatalf("reloaded invalidation = status=%q current=%v origin=%q", status, current, origin)
	}
	if restored.CompletionReady() {
		t.Fatal("reloaded task reused proof from the previous contract")
	}
}

func TestRecoverActiveTurnRecordsProcessRestart(t *testing.T) {
	task, err := New("restart-recovery", "resume the interrupted outcome", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.Attempts = 2
	sequence, ok := task.BeginTurn(TurnRouteImplement)
	if !ok {
		t.Fatal("active turn did not start")
	}
	if !task.RecoverActiveTurn() {
		t.Fatal("active turn was not recovered")
	}
	last := task.LastTurn()
	if last == nil || last.Sequence != sequence || last.State != TurnInterrupted || last.Route != string(TurnRouteRecover) || last.EvidenceState != "UNVERIFIED" || last.StopReason != StopProcessRestart || last.Hypothesis == "" || last.FinishedAt == nil {
		t.Fatalf("recovered turn = %#v, want explicit process-restart metadata", last)
	}
	if task.RecoverActiveTurn() {
		t.Fatal("already recovered turn was changed again")
	}
	if err := task.Validate(); err != nil {
		t.Fatalf("recovered task is invalid: %v", err)
	}
}

func TestTurnLedgerIsBoundedPersistentAndIdentityBound(t *testing.T) {
	task, err := New("turn-ledger", "finish the requested change", []string{"inspect"})
	if err != nil {
		t.Fatal(err)
	}
	task.Attempts = 1
	if !task.SetIntent(&IntentContract{Outcome: task.Goal, Class: "bug", Action: "implementation"}) {
		t.Fatal("intent was not recorded")
	}
	sequence, ok := task.BeginTurn(TurnRouteImplement)
	if !ok || sequence != 1 {
		t.Fatalf("begin sequence=%d ok=%v", sequence, ok)
	}
	if got := task.LastTurn(); got == nil || got.State != TurnActive || got.Route != string(TurnRouteImplement) || got.IntentRevision != 1 {
		t.Fatalf("active turn=%#v", got)
	}
	if task.FinishTurn(sequence+1, TurnRouteComplete, "stale", "PASS", StopGoalComplete, 1, 1) {
		t.Fatal("stale turn identity closed the active record")
	}
	if !task.FinishTurn(sequence, TurnRouteRecover, strings.Repeat("hypothesis ", 100), "FAIL", StopNone, maxTurnToolRounds+1, -1) {
		t.Fatal("active turn did not close")
	}
	last := task.LastTurn()
	if last == nil || last.State != TurnCompleted || last.Route != string(TurnRouteRecover) || last.EvidenceState != "FAIL" || last.ToolRounds != maxTurnToolRounds || last.MutationCount != 0 || last.FinishedAt == nil {
		t.Fatalf("completed turn=%#v", last)
	}
	if len(last.Hypothesis) != maxTurnHypothesis {
		t.Fatalf("hypothesis length=%d want %d", len(last.Hypothesis), maxTurnHypothesis)
	}
	active, ok := task.BeginTurn(TurnRouteInspect)
	if !ok {
		t.Fatal("second turn did not start")
	}
	if _, ok := task.BeginTurn(TurnRouteVerify); !ok {
		t.Fatal("third turn did not start")
	}
	if task.Turns[len(task.Turns)-2].State != TurnInterrupted {
		t.Fatalf("superseded active turn=%#v", task.Turns[len(task.Turns)-2])
	}
	if task.FinishTurn(active, TurnRouteComplete, "stale", "PASS", StopGoalComplete, 1, 0) {
		t.Fatal("superseded turn identity closed a newer turn")
	}
	if err := task.Validate(); err != nil {
		t.Fatalf("turn state invalid: %v", err)
	}

	for i := 0; i < maxTurnRecords+4; i++ {
		seq, ok := task.BeginTurn(TurnRouteOther)
		if !ok {
			t.Fatal("bounded turn did not start")
		}
		if !task.FinishTurn(seq, TurnRouteOther, "continue", "UNVERIFIED", StopNone, i, i) {
			t.Fatal("bounded turn did not finish")
		}
	}
	if len(task.Turns) != maxTurnRecords {
		t.Fatalf("turn history length=%d want %d", len(task.Turns), maxTurnRecords)
	}
	data, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	var restored Task
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if err := restored.Validate(); err != nil {
		t.Fatalf("round-tripped turn state invalid: %v", err)
	}
	if restored.TurnRevision != task.TurnRevision || len(restored.Turns) != maxTurnRecords || restored.LastTurn() == nil {
		t.Fatalf("round-tripped turn state=%#v", restored)
	}
}

func TestTurnValidationRejectsMalformedLifecycle(t *testing.T) {
	base, err := New("turn-validation", "finish the requested change", nil)
	if err != nil {
		t.Fatal(err)
	}
	sequence, ok := base.BeginTurn(TurnRouteInspect)
	if !ok {
		t.Fatal("turn did not start")
	}
	if !base.FinishTurn(sequence, TurnRouteInspect, "inspect", "UNVERIFIED", StopNone, 0, 0) {
		t.Fatal("turn did not finish")
	}
	tests := []struct {
		name string
		edit func(*Task)
	}{
		{"state", func(task *Task) { task.Turns[0].State = "unknown" }},
		{"evidence", func(task *Task) { task.Turns[0].EvidenceState = "model says pass" }},
		{"changed files", func(task *Task) {
			task.Turns[0].ChangedFiles = make([]string, maxTurnChangedFiles+1)
		}},
		{"missing finish", func(task *Task) { task.Turns[0].FinishedAt = nil }},
		{"revision", func(task *Task) { task.TurnRevision = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := *base
			candidate.Turns = append([]TurnRecord(nil), base.Turns...)
			test.edit(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("malformed turn state passed validation")
			}
		})
	}
}

func TestTurnLedgerAttributesBoundedChangedFiles(t *testing.T) {
	task, err := New("turn-side-effects", "finish the requested change", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := task.BeginTurn(TurnRouteImplement); !ok {
		t.Fatal("turn did not start")
	}
	for i := 0; i < maxTurnChangedFiles+2; i++ {
		task.RecordChanged(fmt.Sprintf("./src/file-%02d.go", i))
	}
	last := task.LastTurn()
	if last == nil || len(last.ChangedFiles) != maxTurnChangedFiles || !last.ChangedFilesCapped {
		t.Fatalf("active turn side effects = %#v", last)
	}
	if last.ChangedFiles[0] != "src/file-00.go" || last.ChangedFiles[len(last.ChangedFiles)-1] != "src/file-15.go" {
		t.Fatalf("active turn paths = %#v", last.ChangedFiles)
	}
	isolated := task.LastTurn()
	isolated.ChangedFiles[0] = "caller-owned.go"
	if task.Turns[0].ChangedFiles[0] != "src/file-00.go" {
		t.Fatalf("last turn changed files were aliased: %#v", task.Turns[0].ChangedFiles)
	}
	if !task.FinishTurn(last.Sequence, TurnRouteRecover, "recover changed files", "UNVERIFIED", StopNone, 2, maxTurnChangedFiles+2) {
		t.Fatal("turn did not finish")
	}
	encoded, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	var restored Task
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if err := restored.Validate(); err != nil {
		t.Fatalf("round-tripped turn state is invalid: %v", err)
	}
	last = restored.LastTurn()
	if last == nil || len(last.ChangedFiles) != maxTurnChangedFiles || !last.ChangedFilesCapped {
		t.Fatalf("restored turn side effects = %#v", last)
	}
}

func TestInterruptTurnClosesOnlyItsActiveSequence(t *testing.T) {
	task, err := New("turn-cancel", "finish the requested change", nil)
	if err != nil {
		t.Fatal(err)
	}
	sequence, ok := task.BeginTurn(TurnRouteImplement)
	if !ok {
		t.Fatal("turn did not start")
	}
	if task.InterruptTurn(sequence+1, TurnRouteRecover, "stale", "UNVERIFIED", StopCanceled, 1, 0) {
		t.Fatal("stale sequence interrupted the active turn")
	}
	if !task.InterruptTurn(sequence, TurnRouteRecover, "canceled before completion", "UNVERIFIED", StopCanceled, 1, 0) {
		t.Fatal("active turn did not become interrupted")
	}
	turn := task.LastTurn()
	if turn == nil || turn.State != TurnInterrupted || turn.StopReason != StopCanceled || turn.FinishedAt == nil {
		t.Fatalf("interrupted turn = %#v", turn)
	}
	if err := task.Validate(); err != nil {
		t.Fatalf("interrupted task state is invalid: %v", err)
	}
}
