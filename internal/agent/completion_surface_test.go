package agent

import (
	"reflect"
	"testing"

	"github.com/saiaathish/picogent/internal/outcome"
	"github.com/saiaathish/picogent/internal/testsupport"
)

func TestHeadlessCompletionGateProjectsLongHorizonStates(t *testing.T) {
	cases, err := testsupport.NewCompletionProjectionCases()
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			wantProof := outcome.EvaluateCompletion(tc.Task)
			got := (Result{Task: tc.Task, GoalDone: tc.Marker}).CompletionGate(tc.Goal)

			if !reflect.DeepEqual(got.Proof, wantProof) {
				t.Fatalf("headless proof = %#v, want durable proof %#v", got.Proof, wantProof)
			}
			if got.Required != true || got.Marker != tc.Marker || got.Ready != tc.WantReady {
				t.Fatalf("headless projection = %#v, want required=true marker=%v ready=%v", got, tc.Marker, tc.WantReady)
			}
			if got.Explanation() == "" {
				t.Fatal("headless projection has no bounded explanation")
			}
		})
	}
}
