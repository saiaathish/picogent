package taskstate

import (
	"reflect"
	"testing"
)

// TestCompletionProofContinuityMatrix is the small-lane contract for the
// boundaries that a queued or resumed turn can cross. A new turn by itself is
// a valid no-op continuation; a new intent, mutation, partial proof, or stale
// criterion must not reuse the older completion claim.
func TestCompletionProofContinuityMatrix(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*Task)
		wantReady bool
		wantMiss  []int
	}{
		{
			name: "queued no-op keeps same-generation proof",
			mutate: func(task *Task) {
				finishNoopTurn(t, task, TurnRouteInspect)
			},
			wantReady: true,
		},
		{
			name: "later write invalidates the older proof",
			mutate: func(task *Task) {
				sequence := beginTurn(t, task, TurnRouteImplement)
				task.RecordChanged("feature.go")
				if !task.FinishTurn(sequence, TurnRouteImplement, "later write", "UNVERIFIED", StopNone, 1, 1) {
					t.Fatal("later write turn did not finish")
				}
			},
			wantMiss: []int{0, 1},
		},
		{
			name: "intent revision invalidates the older proof",
			mutate: func(task *Task) {
				intent := *task.Intent
				intent.Action = "review"
				if !task.SetIntent(&intent) {
					t.Fatal("changed intent was not recorded")
				}
			},
			wantMiss: []int{0, 1},
		},
		{
			name: "partial fresh proof leaves the missing criterion blocked",
			mutate: func(task *Task) {
				sequence := beginTurn(t, task, TurnRouteImplement)
				task.RecordChanged("feature.go")
				if !task.FinishTurn(sequence, TurnRouteImplement, "later write", "UNVERIFIED", StopNone, 1, 1) {
					t.Fatal("later write turn did not finish")
				}
				task.RecordCriterionVerification(0, "PASS", "first criterion refreshed", "verify")
				task.RecordTestsEvidence("PASS", "tests refreshed", "go test ./...")
			},
			wantMiss: []int{1},
		},
		{
			name: "fresh proof rebinds after the later write",
			mutate: func(task *Task) {
				sequence := beginTurn(t, task, TurnRouteImplement)
				task.RecordChanged("feature.go")
				if !task.FinishTurn(sequence, TurnRouteImplement, "later write", "UNVERIFIED", StopNone, 1, 1) {
					t.Fatal("later write turn did not finish")
				}
				recordCurrentProof(task)
			},
			wantReady: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := newProofContinuityTask(t)
			tt.mutate(task)

			check := task.CompletionCheck()
			if check.Ready != tt.wantReady {
				t.Fatalf("completion check = %#v, want ready=%v", check, tt.wantReady)
			}
			if !reflect.DeepEqual(check.MissingCriteria, tt.wantMiss) {
				t.Fatalf("missing criteria = %v, want %v", check.MissingCriteria, tt.wantMiss)
			}
			if err := task.Validate(); err != nil {
				t.Fatalf("task state is invalid: %v", err)
			}
		})
	}
}

// TestCompletionProofContinuityReloadIsFailClosedUntilFreshEvidence proves
// that a process reload retains the turn/change chronology but does not
// re-authorize serialized runtime trust. A fresh verifier record is required
// before the same workspace generation can become completion-ready again.
func TestCompletionProofContinuityReloadIsFailClosedUntilFreshEvidence(t *testing.T) {
	task := newProofContinuityTask(t)
	store := NewStore(t.TempDir())
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}

	reloaded, err := store.Load(task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ChangeSeq != task.ChangeSeq || reloaded.TurnRevision != task.TurnRevision || len(reloaded.Turns) != len(task.Turns) {
		t.Fatalf("reload lost chronology: change=%d/%d turn=%d/%d turns=%d/%d", reloaded.ChangeSeq, task.ChangeSeq, reloaded.TurnRevision, task.TurnRevision, len(reloaded.Turns), len(task.Turns))
	}
	if reloaded.CompletionReady() {
		t.Fatal("reload re-authorized serialized completion proof without a fresh verifier")
	}
	if check := reloaded.CompletionCheck(); check.Ready || !reflect.DeepEqual(check.MissingCriteria, []int{0, 1}) {
		t.Fatalf("reload completion check = %#v, want both criteria unproven", check)
	}

	recordCurrentProof(reloaded)
	if !reloaded.CompletionReady() {
		t.Fatalf("fresh verifier evidence did not restore completion: %#v", reloaded.CompletionCheck())
	}
	if err := reloaded.Validate(); err != nil {
		t.Fatalf("freshly rebound task is invalid: %v", err)
	}
}

func newProofContinuityTask(t *testing.T) *Task {
	t.Helper()
	task, err := New("proof-continuity", "ship a verified change", []string{"implement", "verify"})
	if err != nil {
		t.Fatal(err)
	}
	task.DefinitionOfDone = []Criterion{
		{Description: "implementation is complete", Required: true},
		{Description: "verification is complete", Required: true},
	}
	if !task.SetIntent(&IntentContract{
		Outcome:    task.Goal,
		Class:      "implementation",
		Action:     "deliver",
		NeedsTests: true,
	}) {
		t.Fatal("initial intent was not recorded")
	}
	if err := task.SetStatus(StatusWorking); err != nil {
		t.Fatal(err)
	}
	sequence := beginTurn(t, task, TurnRouteImplement)
	task.RecordChanged("feature.go")
	if !task.FinishTurn(sequence, TurnRouteImplement, "initial change", "UNVERIFIED", StopNone, 1, 1) {
		t.Fatal("initial turn did not finish")
	}
	recordCurrentProof(task)
	if !task.CompletionReady() {
		t.Fatalf("fixture proof is not current: %#v", task.CompletionCheck())
	}
	return task
}

func recordCurrentProof(task *Task) {
	task.RecordCriterionVerification(0, "PASS", "implementation is complete", "verify")
	task.RecordCriterionVerification(1, "PASS", "verification is complete", "verify")
	task.RecordTestsEvidence("PASS", "tests passed", "go test ./...")
}

func beginTurn(t *testing.T, task *Task, route TurnRoute) uint64 {
	t.Helper()
	sequence, ok := task.BeginTurn(route)
	if !ok {
		t.Fatalf("begin %q turn failed", route)
	}
	return sequence
}

func finishNoopTurn(t *testing.T, task *Task, route TurnRoute) {
	t.Helper()
	sequence := beginTurn(t, task, route)
	if !task.FinishTurn(sequence, route, "no-op continuation", "PASS", StopNone, 0, 0) {
		t.Fatalf("no-op %q turn did not finish", route)
	}
}
