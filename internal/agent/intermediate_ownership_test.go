package agent

import (
	"reflect"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/taskstate"
)

type intermediateOwnershipEvents struct {
	NopHandler
	updates []*taskstate.Task
	errors  []error
}

func (h *intermediateOwnershipEvents) OnTaskState(task *taskstate.Task) {
	h.updates = append(h.updates, task)
}

func (h *intermediateOwnershipEvents) OnError(err error) { h.errors = append(h.errors, err) }

func intermediateOwnershipFixture(t *testing.T) *Agent {
	t.Helper()
	task, err := taskstate.New("owned-intermediate", "fix the backend bug", []string{"work", "verify"})
	if err != nil {
		t.Fatal(err)
	}
	task.SetIntent(taskstate.Infer("fix the backend bug").Intent)
	if err := task.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	if _, ok := task.BeginTurn(taskstate.TurnRouteImplement); !ok {
		t.Fatal("start owned turn")
	}
	store := taskstate.NewStore(t.TempDir())
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}
	return &Agent{TaskStore: store, TaskSession: task.SessionID, task: task}
}

func TestIntermediateTaskReplayRejectsChangedOwnership(t *testing.T) {
	changes := []struct {
		name   string
		change func(*taskstate.Task)
	}{
		{"task identity", func(task *taskstate.Task) { task.ID = "owned-new-task" }},
		{"intent revision", func(task *taskstate.Task) {
			if err := task.ReplaceOutcome("document the API"); err != nil {
				t.Fatal(err)
			}
		}},
		{"new turn", func(task *taskstate.Task) {
			if _, ok := task.BeginTurn(taskstate.TurnRouteInspect); !ok {
				t.Fatal("start newer turn")
			}
		}},
		{"closed turn", func(task *taskstate.Task) {
			if !task.FinishTurn(task.TurnRevision, taskstate.TurnRouteImplement, "newer owner closed turn", "", taskstate.StopNone, 0, 0) {
				t.Fatal("close turn")
			}
		}},
		{"same text intent ABA", func(task *taskstate.Task) {
			for _, prompt := range []string{"document the API", "fix the backend bug"} {
				if err := task.ReplaceOutcome(prompt); err != nil {
					t.Fatal(err)
				}
			}
		}},
	}
	for _, change := range changes {
		t.Run(change.name, func(t *testing.T) {
			a := intermediateOwnershipFixture(t)
			admitted := a.TaskSnapshot()
			current, err := a.TaskStore.Load(admitted.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			change.change(current)
			if err := a.TaskStore.Save(current); err != nil {
				t.Fatal(err)
			}
			before, err := a.TaskStore.Load(current.SessionID)
			if err != nil {
				t.Fatal(err)
			}
			h := &intermediateOwnershipEvents{}
			mutations := 0
			// A refused retry must not adopt the replacement into memory and let
			// the next old-turn callback mutate it under its new identity.
			for attempt := 0; attempt < 2; attempt++ {
				if a.mutateTask(h, func(task *taskstate.Task) error {
					mutations++
					task.RecordChanged("stale-turn.txt")
					task.RecordApprovalEvidence("APPROVED", "stale permission", "permission prompt")
					task.NoteAttempt()
					return nil
				}) {
					t.Fatal("old turn replayed into a replacement owner")
				}
			}
			after, err := a.TaskStore.Load(current.SessionID)
			if err != nil || !reflect.DeepEqual(before, after) || !reflect.DeepEqual(admitted, a.TaskSnapshot()) || len(h.updates) != 0 || mutations != 2 || len(h.errors) != 2 {
				t.Fatalf("stale replay changed/adopted state: updates=%d mutations=%d errors=%v err=%v", len(h.updates), mutations, h.errors, err)
			}
			for _, err := range h.errors {
				if !strings.Contains(err.Error(), "ownership changed") {
					t.Fatalf("ownership refusal was not explained: %v", err)
				}
			}
		})
	}
}

func TestIntermediateTaskReplayMergesSameOwnerProgress(t *testing.T) {
	a := intermediateOwnershipFixture(t)
	current, err := a.TaskStore.Load(a.task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	current.NoteAttempt()
	current.AddConstraint("other process boundary")
	if err := a.TaskStore.Save(current); err != nil {
		t.Fatal(err)
	}
	h := &intermediateOwnershipEvents{}
	if !a.mutateTask(h, func(task *taskstate.Task) error { task.RecordChanged("owned-turn.txt"); return nil }) {
		t.Fatalf("same-owner progress conflict was not merged: %v", h.errors)
	}
	got, err := a.TaskStore.Load(current.SessionID)
	if err != nil || got.Attempts != current.Attempts || !reflect.DeepEqual(got.Constraints, current.Constraints) || len(got.ChangedFiles) != 1 || got.ChangedFiles[0] != "owned-turn.txt" || len(h.updates) != 1 || len(h.errors) != 0 {
		t.Fatalf("same-owner replay lost progress: %+v errors=%v err=%v", got, h.errors, err)
	}
}
