package agent

import (
	"reflect"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/taskstate"
)

func completedAdmissionFixture(t *testing.T) (*Agent, string) {
	t.Helper()
	a, root, sequence := completionCandidate(t)
	completed, _, _, closed, _, err := finishCompletionCandidate(a, root, sequence)
	if err != nil || !closed || completed == nil || completed.Status != taskstate.StatusDone || completed.NeedsVerification() {
		t.Fatalf("fixture is not genuinely complete: task=%+v closed=%v err=%v", completed, closed, err)
	}
	a.TaskSession = completed.SessionID
	return a, root
}

func TestSuccessorAdmissionUsesNewOutcomeAndPreviousRevision(t *testing.T) {
	a, root := completedAdmissionFixture(t)
	previous := a.TaskSnapshot()
	if failed, err := a.beginDurableTask("document the API", NopHandler{}); failed || err != nil {
		t.Fatalf("successor admission failed: %v", err)
	}
	current := a.TaskSnapshot()
	if current.ID == previous.ID || current.Goal != "Document the API" || current.Revision != previous.Revision+1 || current.IntentRevision <= previous.IntentRevision || current.TurnRevision != previous.TurnRevision || current.Attempts != 1 || current.Status != taskstate.StatusWorking {
		t.Fatalf("successor reused the old outcome: previous=%+v current=%+v", previous, current)
	}
	if len(current.ChangedFiles) != 0 || len(current.Verification) != 0 || len(current.Evidence) != 0 || len(current.Turns) != 0 || reflect.DeepEqual(current.DefinitionOfDone, previous.DefinitionOfDone) {
		t.Fatalf("successor inherited old progress or proof: %+v", current)
	}
	fresh := newUndoHookAgent(t, root)
	defer fresh.Close()
	fresh.SetTaskStore(a.TaskStore)
	if err := fresh.SetTaskSession(current.SessionID); err != nil {
		t.Fatal(err)
	}
	if restored := fresh.TaskSnapshot(); restored.ID != current.ID || restored.Goal != current.Goal || restored.Revision != current.Revision {
		t.Fatalf("restart lost the successor: %+v", restored)
	}
	sequence, started := a.beginDurableTurn(taskstate.TurnRouteImplement, NopHandler{})
	if !started || sequence <= previous.TurnRevision {
		t.Fatal("successor reused an old turn sequence")
	}
	beforeClose := a.TaskSnapshot()
	closed, err := a.closeDurableTurn(previous.TurnRevision, false, taskstate.TurnRouteComplete, "old completion", "", taskstate.StopNone, 0, 0, NopHandler{})
	if err != nil || closed || !reflect.DeepEqual(beforeClose, a.TaskSnapshot()) {
		t.Fatal("old completion closed the successor turn")
	}
}

func TestSuccessorAdmissionRefusesExhaustedOwnershipCounters(t *testing.T) {
	for _, counter := range []string{"intent", "turn"} {
		t.Run(counter, func(t *testing.T) {
			a, _ := completedAdmissionFixture(t)
			if counter == "intent" {
				a.task.IntentRevision = ^uint64(0)
			} else {
				a.task.TurnRevision = ^uint64(0)
			}
			before := a.TaskSnapshot()
			if failed, err := a.beginDurableTask("document the API", NopHandler{}); !failed || err == nil || !strings.Contains(err.Error(), "exhausted") || !reflect.DeepEqual(before, a.TaskSnapshot()) {
				t.Fatalf("exhausted ownership was reused: failed=%v err=%v", failed, err)
			}
		})
	}
}

func TestFreshAdmissionDoesNotAdoptConcurrentTask(t *testing.T) {
	store := taskstate.NewStore(t.TempDir())
	current, ok, err := taskstate.NewFromPrompt("fresh-admission", "fix the backend bug")
	if err != nil || !ok {
		t.Fatal("fixture must be task-like")
	}
	if err := current.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(current); err != nil {
		t.Fatal(err)
	}
	a := &Agent{TaskStore: store, TaskSession: current.SessionID}
	before, err := store.Load(current.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	h := &intermediateOwnershipEvents{}
	for attempt := 0; attempt < 2; attempt++ {
		if failed, err := a.beginDurableTask("document the API", h); !failed || err == nil {
			t.Fatalf("fresh admission adopted the concurrent task: task=%+v err=%v", a.TaskSnapshot(), err)
		}
		if a.TaskSnapshot() != nil {
			t.Fatal("failed fresh admission published someone else's task")
		}
	}
	after, err := store.Load(current.SessionID)
	if err != nil || !reflect.DeepEqual(before, after) || len(h.updates) != 0 || len(h.errors) != 2 {
		t.Fatalf("refused admission changed state: errors=%v err=%v", h.errors, err)
	}
}

func TestSuccessorAdmissionRefusesConcurrentRevisionWithoutAdoption(t *testing.T) {
	for _, replace := range []bool{false, true} {
		name := "same-owner update"
		if replace {
			name = "replacement outcome"
		}
		t.Run(name, func(t *testing.T) {
			a, _ := completedAdmissionFixture(t)
			admitted := a.TaskSnapshot()
			current, err := a.TaskStore.Load(admitted.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			if replace {
				if err := current.ReplaceOutcome("fix the cache bug"); err != nil {
					t.Fatal(err)
				}
			} else {
				current.NoteAttempt()
			}
			if err := a.TaskStore.Save(current); err != nil {
				t.Fatal(err)
			}
			before, err := a.TaskStore.Load(current.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			h := &intermediateOwnershipEvents{}
			for attempt := 0; attempt < 2; attempt++ {
				if failed, err := a.beginDurableTask("document the API", h); !failed || err == nil {
					t.Fatalf("successor rebased into a concurrent task: %+v %v", a.TaskSnapshot(), err)
				}
			}
			after, err := a.TaskStore.Load(current.SessionID)
			if err != nil || !reflect.DeepEqual(before, after) || !reflect.DeepEqual(admitted, a.TaskSnapshot()) || len(h.updates) != 0 || len(h.errors) != 2 {
				t.Fatalf("successor refusal changed/adopted state: errors=%v err=%v", h.errors, err)
			}
		})
	}
}

func TestActiveAdmissionReplayKeepsOriginalOwnership(t *testing.T) {
	for _, replace := range []bool{false, true} {
		name := "same-owner progress"
		if replace {
			name = "changed intent"
		}
		t.Run(name, func(t *testing.T) {
			a := intermediateOwnershipFixture(t)
			admitted := a.TaskSnapshot()
			current, err := a.TaskStore.Load(admitted.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			current.NoteAttempt()
			if replace {
				if err := current.ReplaceOutcome("fix the cache bug"); err != nil {
					t.Fatal(err)
				}
			}
			if err := a.TaskStore.Save(current); err != nil {
				t.Fatal(err)
			}
			before, err := a.TaskStore.Load(current.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			h := &intermediateOwnershipEvents{}
			failed, err := a.beginDurableTask("also audit the frontend", h)
			if replace {
				if !failed || err == nil || !strings.Contains(err.Error(), "ownership changed") || !reflect.DeepEqual(admitted, a.TaskSnapshot()) {
					t.Fatalf("active admission acquired a changed owner: failed=%v err=%v", failed, err)
				}
				after, loadErr := a.TaskStore.Load(current.SessionID)
				if loadErr != nil || !reflect.DeepEqual(before, after) || len(h.updates) != 0 {
					t.Fatal("active refusal changed durable state")
				}
			} else if failed || err != nil || a.TaskSnapshot().Attempts != before.Attempts+1 || a.TaskSnapshot().Goal != admitted.Goal {
				t.Fatalf("benign admission rebase lost progress: failed=%v err=%v task=%+v", failed, err, a.TaskSnapshot())
			}
		})
	}
}

func TestAdmissionReplayMergesOnlyClosedEarlierTurns(t *testing.T) {
	for _, closed := range []bool{false, true} {
		name := "active newer turn"
		if closed {
			name = "completed earlier turn"
		}
		t.Run(name, func(t *testing.T) {
			a := intermediateOwnershipFixture(t)
			admitted := a.TaskSnapshot()
			current, err := a.TaskStore.Load(admitted.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			sequence, started := current.BeginTurn(taskstate.TurnRouteInspect)
			if !started {
				t.Fatal("start newer turn")
			}
			if closed && !current.FinishTurn(sequence, taskstate.TurnRouteInspect, "earlier process finished", "", taskstate.StopNone, 0, 0) {
				t.Fatal("close earlier turn")
			}
			if err := a.TaskStore.Save(current); err != nil {
				t.Fatal(err)
			}
			h := &intermediateOwnershipEvents{}
			failed, err := a.beginDurableTask(admitted.Goal, h)
			if closed {
				if failed || err != nil || a.TaskSnapshot().TurnRevision != sequence || a.TaskSnapshot().Attempts != current.Attempts+1 {
					t.Fatalf("closed earlier progress not merged: %v", err)
				}
			} else if !failed || err == nil || !reflect.DeepEqual(admitted, a.TaskSnapshot()) || len(h.updates) != 0 {
				t.Fatalf("admission acquired a newer active turn: %v", err)
			}
		})
	}
}
