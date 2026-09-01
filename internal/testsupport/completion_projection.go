// Package testsupport contains fixtures used only by cross-package tests.
// Product packages must not import it; the fixture keeps adapter tests aligned
// without creating another runtime planner, store, or outcome representation.
package testsupport

import "github.com/saiaathish/picogent/internal/taskstate"

// CompletionProjectionState names the durable boundary a surface test is
// expected to preserve. It is test vocabulary, not a product-facing state.
type CompletionProjectionState string

const (
	StateIncomplete      CompletionProjectionState = "incomplete"
	StateStaleProof      CompletionProjectionState = "stale-proof"
	StateRecoveryPending CompletionProjectionState = "recovery-pending"
	StateCurrentProof    CompletionProjectionState = "current-proof"
)

// CompletionProjectionCase is a fresh durable task snapshot plus the small
// turn context needed by the headless completion gate. Each call to
// NewCompletionProjectionCases creates independent tasks so one adapter test
// cannot mutate another surface's evidence.
type CompletionProjectionCase struct {
	Name      string
	State     CompletionProjectionState
	Task      *taskstate.Task
	Goal      string
	Marker    bool
	WantReady bool
}

// NewCompletionProjectionCases returns the four M-lane boundaries that the
// headless, GUI, and TUI adapters must project consistently.
func NewCompletionProjectionCases() ([]CompletionProjectionCase, error) {
	incomplete, err := newCompletionProjectionTask("projection-incomplete")
	if err != nil {
		return nil, err
	}

	stale, err := newCompletionProjectionTask("projection-stale")
	if err != nil {
		return nil, err
	}
	recordCriterionProof(stale)
	stale.RecordChanged("outcome.txt")

	recovery, err := newCompletionProjectionTask("projection-recovery")
	if err != nil {
		return nil, err
	}
	if _, ok := recovery.BeginTurn(taskstate.TurnRouteImplement); !ok {
		return nil, errProjectionFixture("begin recovery-pending turn")
	}

	current, err := newCompletionProjectionTask("projection-current")
	if err != nil {
		return nil, err
	}
	recordCriterionProof(current)
	if err := current.SetStatus(taskstate.StatusDone); err != nil {
		return nil, err
	}

	return []CompletionProjectionCase{
		{
			Name: "incomplete", State: StateIncomplete, Task: incomplete,
			Goal: incomplete.Goal, Marker: true, WantReady: false,
		},
		{
			Name: "stale-proof", State: StateStaleProof, Task: stale,
			Goal: stale.Goal, Marker: true, WantReady: false,
		},
		{
			Name: "recovery-pending", State: StateRecoveryPending, Task: recovery,
			Goal: recovery.Goal, Marker: true, WantReady: false,
		},
		{
			Name: "current-proof", State: StateCurrentProof, Task: current,
			Goal: current.Goal, Marker: true, WantReady: true,
		},
	}, nil
}

func newCompletionProjectionTask(sessionID string) (*taskstate.Task, error) {
	task, err := taskstate.New(sessionID, "make the outcome ready", []string{"implement the outcome"})
	if err != nil {
		return nil, err
	}
	task.DefinitionOfDone = []taskstate.Criterion{
		{Description: "the outcome is verified", Required: true},
	}
	if !task.SetIntent(&taskstate.IntentContract{
		Outcome: task.Goal,
		Class:   "implementation",
		Action:  "deliver the outcome",
	}) {
		return nil, errProjectionFixture("record outcome intent")
	}
	if err := task.SetStatus(taskstate.StatusWorking); err != nil {
		return nil, err
	}
	return task, nil
}

func recordCriterionProof(task *taskstate.Task) {
	task.RecordCriterionVerification(0, "PASS", "the outcome is verified", "verify")
}

type projectionFixtureError string

func (e projectionFixtureError) Error() string { return string(e) }

func errProjectionFixture(action string) error {
	return projectionFixtureError("could not " + action)
}
