package gui

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/saiaathish/picogent/internal/outcome"
	"github.com/saiaathish/picogent/internal/projecthealth"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/testsupport"
)

func TestGUIProjectsLongHorizonOutcomeStates(t *testing.T) {
	cases, err := testsupport.NewCompletionProjectionCases()
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			want := outcome.EvaluateCompletion(tc.Task)
			contract := outcome.Build(tc.Task, projecthealth.Report{Schema: projecthealth.Schema})
			if got := taskCompletionProof(tc.Task); got == nil || !reflect.DeepEqual(*got, want) {
				t.Fatalf("direct GUI proof = %#v, want durable proof %#v", got, want)
			}

			events := make(chan event, 1)
			s := &server{subs: []chan event{events}, sessionID: tc.Task.SessionID, turnGen: 1}
			h := &guiHandler{s: s, sessionID: tc.Task.SessionID, turnGen: 1}
			h.OnTaskState(tc.Task)

			got := <-events
			if got.Type != "task_progress" || got.Task == nil || got.Completion == nil {
				t.Fatalf("GUI task-progress event = %#v, want task and proof", got)
			}
			if !reflect.DeepEqual(*got.Completion, want) {
				t.Fatalf("GUI event proof = %#v, want durable proof %#v", *got.Completion, want)
			}
			if got.Task.SessionID != tc.Task.SessionID {
				t.Fatalf("GUI event session = %q, want %q", got.Task.SessionID, tc.Task.SessionID)
			}
			if last := tc.Task.LastTurn(); last != nil {
				gotLast := got.Task.LastTurn()
				if gotLast == nil || gotLast.State != last.State || gotLast.Route != last.Route {
					t.Fatalf("GUI event turn = %#v, want state=%q route=%q", gotLast, last.State, last.Route)
				}
			}
			if last := tc.Task.LastTurn(); last == nil || contract.Turn.TurnSequence != last.Sequence || contract.Turn.LastTurnState != string(last.State) || contract.Turn.LastRoute != last.Route {
				t.Fatalf("GUI turn projection = %#v, want durable turn %#v", contract.Turn, last)
			}
			wantRecovery := tc.State == testsupport.StateRecoveryPending
			if contract.Turn.NeedsRecovery() != wantRecovery {
				t.Fatalf("GUI recovery projection = %v, want %v: %#v", contract.Turn.NeedsRecovery(), wantRecovery, contract.Turn)
			}
			if tc.State == testsupport.StateCurrentProof && contract.Stop.Policy != outcome.StopRecheck {
				t.Fatalf("GUI current-proof stop policy = %q, want %q", contract.Stop.Policy, outcome.StopRecheck)
			}

			wire, err := jsonTaskProgress(got)
			if err != nil {
				t.Fatal(err)
			}
			if wire.Completion == nil || !reflect.DeepEqual(*wire.Completion, want) {
				t.Fatalf("GUI wire proof = %#v, want durable proof %#v", wire.Completion, want)
			}
		})
	}
}

func TestGUIReloadProjectionRequiresFreshWorkspaceEvidence(t *testing.T) {
	workspaceRoot := t.TempDir()
	fixture, err := testsupport.NewWorkspaceBoundCompletionFixture(workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	store := taskstate.NewStore(t.TempDir())
	if err := store.Save(fixture.Task); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.Load(fixture.Task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	wantPending := outcome.EvaluateCompletion(reloaded)
	if wantPending.Ready {
		t.Fatalf("reload retained runtime proof: %#v", wantPending)
	}

	events := make(chan event, 2)
	s := &server{subs: []chan event{events}, sessionID: reloaded.SessionID, turnGen: 1}
	h := &guiHandler{s: s, sessionID: reloaded.SessionID, turnGen: 1}
	h.OnTaskState(reloaded)
	pending := <-events
	if pending.Completion == nil || !reflect.DeepEqual(*pending.Completion, wantPending) || pending.Completion.Ready {
		t.Fatalf("GUI reloaded proof = %#v, want fail-closed %#v", pending.Completion, wantPending)
	}

	if !reloaded.ReestablishWorkspaceVerification(&fixture.Observation) {
		t.Fatal("fresh workspace observation did not rebind GUI proof")
	}
	wantReady := outcome.EvaluateCompletion(reloaded)
	h.OnTaskState(reloaded)
	ready := <-events
	if ready.Completion == nil || !reflect.DeepEqual(*ready.Completion, wantReady) || !ready.Completion.Ready {
		t.Fatalf("GUI rebound proof = %#v, want ready %#v", ready.Completion, wantReady)
	}
}

type guiTaskProgressWire struct {
	Task       *taskstate.Task            `json:"task"`
	Completion *taskstate.CompletionCheck `json:"completion"`
}

func jsonTaskProgress(e event) (guiTaskProgressWire, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return guiTaskProgressWire{}, err
	}
	var wire guiTaskProgressWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return guiTaskProgressWire{}, err
	}
	return wire, nil
}
