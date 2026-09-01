package gui

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/saiaathish/picogent/internal/outcome"
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
