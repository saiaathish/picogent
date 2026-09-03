package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/taskstate"
)

func TestScenarioTableIsCompleteAndFailClosed(t *testing.T) {
	scenarios := Scenarios()
	if err := ValidateScenarioTable(scenarios); err != nil {
		t.Fatal(err)
	}
	if len(scenarios) != 13 {
		t.Fatalf("scenario count = %d, want 13", len(scenarios))
	}

	copyOfTable := Scenarios()
	copyOfTable[0].ID = "mutated-copy"
	if Scenarios()[0].ID == "mutated-copy" {
		t.Fatal("Scenarios returned the mutable canonical table")
	}
}

func TestHeadlessProcessKillScenarioRequiresFreshProcessRecovery(t *testing.T) {
	scenario := scenarioByID(t, "headless-process-kill-active-turn")
	if scenario.Trigger != TriggerProcessKill || scenario.Surface != SurfaceHeadless {
		t.Fatalf("process-kill scenario = %#v, want headless process-kill row", scenario)
	}
	if !scenario.FreshProcessRequired || scenario.Evidence != EvidenceUnverified {
		t.Fatalf("headless process-kill evidence boundary = %#v, want fresh-process UNVERIFIED", scenario)
	}
	persisted := scenario.Persisted
	if persisted.TaskStatus != taskstate.StatusWorking || persisted.TurnState != taskstate.TurnInterrupted ||
		persisted.TurnRoute != string(taskstate.TurnRouteRecover) || persisted.StopReason != taskstate.StopProcessRestart ||
		!persisted.MustRetainTask || !persisted.MustBeRecoverable {
		t.Fatalf("headless process-kill persistence contract = %#v, want recoverable interrupted turn", persisted)
	}
	if !scenario.Completion.MustNotBeReady || !scenario.Completion.MustNotShowMarker {
		t.Fatalf("headless process-kill completion contract = %#v, want fail-closed", scenario.Completion)
	}
}

func TestGUIProcessKillScenarioRequiresFreshProcessRecovery(t *testing.T) {
	scenario := scenarioByID(t, "gui-process-kill-active-turn")
	if scenario.Trigger != TriggerProcessKill || scenario.Surface != SurfaceGUI {
		t.Fatalf("process-kill scenario = %#v, want GUI process-kill row", scenario)
	}
	if !scenario.FreshProcessRequired || scenario.Evidence != EvidenceUnverified {
		t.Fatalf("process-kill evidence boundary = %#v, want fresh-process UNVERIFIED", scenario)
	}
	persisted := scenario.Persisted
	if persisted.TaskStatus != taskstate.StatusWorking || persisted.TurnState != taskstate.TurnInterrupted ||
		persisted.TurnRoute != string(taskstate.TurnRouteRecover) || persisted.StopReason != taskstate.StopProcessRestart ||
		!persisted.MustRetainTask || !persisted.MustBeRecoverable {
		t.Fatalf("process-kill persistence contract = %#v, want recoverable interrupted turn", persisted)
	}
	if !scenario.Completion.MustNotBeReady || !scenario.Completion.MustNotShowMarker {
		t.Fatalf("process-kill completion contract = %#v, want fail-closed", scenario.Completion)
	}
}

func TestTUIProcessKillScenarioRequiresFreshProcessRecovery(t *testing.T) {
	scenario := scenarioByID(t, "tui-process-kill-active-turn")
	if scenario.Trigger != TriggerProcessKill || scenario.Surface != SurfaceTUI {
		t.Fatalf("process-kill scenario = %#v, want TUI process-kill row", scenario)
	}
	if !scenario.FreshProcessRequired || scenario.Evidence != EvidenceUnverified {
		t.Fatalf("TUI process-kill evidence boundary = %#v, want fresh-process UNVERIFIED", scenario)
	}
	persisted := scenario.Persisted
	if persisted.TaskStatus != taskstate.StatusWorking || persisted.TurnState != taskstate.TurnInterrupted ||
		persisted.TurnRoute != string(taskstate.TurnRouteRecover) || persisted.StopReason != taskstate.StopProcessRestart ||
		!persisted.MustRetainTask || !persisted.MustBeRecoverable {
		t.Fatalf("TUI process-kill persistence contract = %#v, want recoverable interrupted turn", persisted)
	}
	if !scenario.Completion.MustNotBeReady || !scenario.Completion.MustNotShowMarker {
		t.Fatalf("TUI process-kill completion contract = %#v, want fail-closed", scenario.Completion)
	}
}

func TestSummarizeErrorPreservesPrimaryAndDurabilityClasses(t *testing.T) {
	combined := errors.New("provider unavailable; durable task state was not saved")
	summary := SummarizeError(combined)
	if summary.Primary != ErrorProvider || summary.Durability != ErrorTaskPersistence {
		t.Fatalf("combined error summary = %#v, want provider plus task persistence", summary)
	}
	if summary.VisibleClass() != ErrorProvider {
		t.Fatalf("visible combined class = %q, want provider", summary.VisibleClass())
	}

	canceled := SummarizeError(context.Canceled)
	if canceled.Primary != ErrorCanceled || canceled.Durability != ErrorNone {
		t.Fatalf("canceled error summary = %#v, want canceled only", canceled)
	}

	session := SummarizeError(errors.New("couldn't save session: disk full"))
	if session.VisibleClass() != ErrorSessionPersistence || session.Durability != ErrorSessionPersistence {
		t.Fatalf("session error summary = %#v, want session persistence", session)
	}
}

func TestObserveAndCheckEnforceInterruptedTurnInvariants(t *testing.T) {
	task, err := taskstate.New("lifecycle-session", "resume the interrupted task", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := task.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	sequence, ok := task.BeginTurn(taskstate.TurnRouteImplement)
	if !ok || !task.InterruptTurn(sequence, taskstate.TurnRouteRecover, "canceled before completion", "UNVERIFIED", taskstate.StopCanceled, 1, 0) {
		t.Fatal("could not construct interrupted lifecycle fixture")
	}

	scenario := scenarioByID(t, "headless-signal-active-turn")
	observation := Observe(scenario.ID, scenario.Surface, scenario.Trigger, task, CompletionProjection{Required: true}, context.Canceled)
	if violations := scenario.Check(observation); len(violations) != 0 {
		t.Fatalf("valid interrupted observation violations = %v", violations)
	}
	if !observation.Recoverable() || observation.Outcome.LastTurnState != string(taskstate.TurnInterrupted) {
		t.Fatalf("observation = %#v, want recoverable interrupted turn", observation)
	}

	observation.Completion = CompletionProjection{Required: true, Ready: true, Marker: true}
	violations := observation.InvariantViolations()
	if len(violations) == 0 || !containsViolation(violations, "projected as complete") {
		t.Fatalf("ready interrupted observation violations = %v, want fail-closed violation", violations)
	}
}

func TestObserveCapturesDurableOutcomeWithoutMutatingTask(t *testing.T) {
	task, err := taskstate.New("lifecycle-observation", "inspect the task", nil)
	if err != nil {
		t.Fatal(err)
	}
	sequence, ok := task.BeginTurn(taskstate.TurnRouteInspect)
	if !ok {
		t.Fatal("could not begin observation turn")
	}
	before := task.LastTurn()
	observation := Observe("observation", SurfaceTUI, TriggerCancellation, task, CompletionProjection{Required: true}, context.Canceled)
	if before == nil || observation.Outcome.TurnSequence != before.Sequence || observation.Outcome.LastTurnState != string(before.State) {
		t.Fatalf("observation outcome = %#v, want active turn %d", observation.Outcome, sequence)
	}
	if task.LastTurn() == nil || task.LastTurn().State != taskstate.TurnActive {
		t.Fatal("Observe mutated the task")
	}
}

func scenarioByID(t *testing.T, id string) Scenario {
	t.Helper()
	for _, scenario := range Scenarios() {
		if scenario.ID == id {
			return scenario
		}
	}
	t.Fatalf("scenario %q not found", id)
	return Scenario{}
}

func containsViolation(violations []string, want string) bool {
	for _, violation := range violations {
		if strings.Contains(violation, want) {
			return true
		}
	}
	return false
}
