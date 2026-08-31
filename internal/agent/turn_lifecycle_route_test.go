package agent

import (
	"testing"

	"github.com/saiaathish/picogent/internal/taskstate"
)

func TestDurableTurnStartRouteFollowsIntentInvalidation(t *testing.T) {
	t.Run("initial intent remains admission", func(t *testing.T) {
		task, err := taskstate.New("route-initial-intent", "finish the requested change", nil)
		if err != nil {
			t.Fatal(err)
		}
		if !task.SetIntent(&taskstate.IntentContract{Outcome: task.Goal, Class: "general"}) {
			t.Fatal("initial intent was not recorded")
		}
		if task.NeedsVerification() {
			t.Fatal("initial intent unexpectedly requested verification")
		}
		if got := durableTurnStartRoute(task, TaskAgent); got != taskstate.TurnRouteAdmission {
			t.Fatalf("initial intent route = %q, want %q", got, taskstate.TurnRouteAdmission)
		}
	})

	t.Run("invalidated quality proof routes to verify", func(t *testing.T) {
		task, err := taskstate.New("route-quality-proof", "finish the requested change", nil)
		if err != nil {
			t.Fatal(err)
		}
		initial := &taskstate.IntentContract{Outcome: task.Goal, Class: "general", NeedsTests: true}
		if !task.SetIntent(initial) {
			t.Fatal("initial intent was not recorded")
		}
		task.RecordTestsEvidence("PASS", "current tests passed", "go test ./...")
		if !task.CompletionReady() {
			t.Fatal("current quality proof did not complete the initial contract")
		}

		changed := *initial
		changed.Class = "bug"
		if !task.SetIntent(&changed) {
			t.Fatal("changed intent was not recorded")
		}
		if got := durableTurnStartRoute(task, TaskAgent); got != taskstate.TurnRouteVerify {
			t.Fatalf("invalidated quality proof route = %q, want %q", got, taskstate.TurnRouteVerify)
		}
	})

	t.Run("invalidated criterion proof routes to verify", func(t *testing.T) {
		task, err := taskstate.New("route-criterion-proof", "finish the requested change", nil)
		if err != nil {
			t.Fatal(err)
		}
		task.DefinitionOfDone = []taskstate.Criterion{{Description: "required proof", Required: true}}
		initial := &taskstate.IntentContract{Outcome: task.Goal, Class: "general"}
		if !task.SetIntent(initial) {
			t.Fatal("initial intent was not recorded")
		}
		task.RecordCriterionVerification(0, "PASS", "criterion passed", "verify")
		if !task.CompletionReady() {
			t.Fatal("current criterion proof did not complete the initial contract")
		}

		changed := *initial
		changed.Class = "documentation"
		if !task.SetIntent(&changed) {
			t.Fatal("changed intent was not recorded")
		}
		if got := durableTurnStartRoute(task, TaskAgent); got != taskstate.TurnRouteVerify {
			t.Fatalf("invalidated criterion proof route = %q, want %q", got, taskstate.TurnRouteVerify)
		}
	})
}
