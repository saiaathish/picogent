package agent

import (
	"reflect"
	"testing"

	"github.com/saiaathish/picogent/internal/outcome"
	"github.com/saiaathish/picogent/internal/taskstate"
)

func TestCompletionProjectionUsesDurableProofDecision(t *testing.T) {
	task, err := taskstate.New("completion-projection", "finish the outcome", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := task.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	task.DefinitionOfDone = []taskstate.Criterion{{Description: "required proof", Required: true}}

	missing := completionProjection(task, task.Goal, true, false, 0, "")
	wantMissing := outcome.EvaluateCompletion(task)
	if !reflect.DeepEqual(missing.Proof, wantMissing) {
		t.Fatalf("projection proof = %#v, evaluator = %#v", missing.Proof, wantMissing)
	}
	if missing.Ready || missing.Proof.Ready || missing.Reason != wantMissing.Reason {
		t.Fatalf("missing proof projection = %#v", missing)
	}

	task.RecordCriterionVerification(0, "PASS", "required proof passed", "verify")
	ready := completionProjection(task, task.Goal, true, false, 0, "")
	if !ready.Ready || !ready.Proof.Ready || ready.Reason != ready.Proof.Reason {
		t.Fatalf("ready proof projection = %#v", ready)
	}
}
