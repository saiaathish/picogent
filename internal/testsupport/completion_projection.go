// Package testsupport contains fixtures used only by cross-package tests.
// Product packages must not import it; the fixture keeps adapter tests aligned
// without creating another runtime planner, store, or outcome representation.
package testsupport

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/workspace"
)

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

// WorkspaceBoundCompletionFixture is a persisted-proof fixture. Store.Load
// intentionally removes runtime trust; ReestablishWorkspaceVerification can
// restore it only after comparing this observation with the live workspace.
type WorkspaceBoundCompletionFixture struct {
	Task        *taskstate.Task
	Observation workspace.Observation
}

// NewCompletionProjectionCases returns the four M-lane boundaries that the
// headless, GUI, and TUI adapters must project consistently.
func NewCompletionProjectionCases() ([]CompletionProjectionCase, error) {
	incomplete, err := newCompletionProjectionTask("projection-incomplete")
	if err != nil {
		return nil, err
	}
	if err := finishProjectionTurn(incomplete, taskstate.TurnRouteImplement, "UNVERIFIED", taskstate.StopNone); err != nil {
		return nil, err
	}

	stale, err := newCompletionProjectionTask("projection-stale")
	if err != nil {
		return nil, err
	}
	if err := finishProjectionTurn(stale, taskstate.TurnRouteImplement, "PASS", taskstate.StopNone); err != nil {
		return nil, err
	}
	recordCriterionProof(stale)
	stale.RecordChanged("outcome.txt")

	recovery, err := newCompletionProjectionTask("projection-recovery")
	if err != nil {
		return nil, err
	}
	// Keep a prior proof in the ledger, then attach a side effect to the
	// interrupted turn. This makes recovery distinct from a task that never
	// had evidence: the old proof is stale and the turn contract carries the
	// explicit restart/recovery provenance.
	recordCriterionProof(recovery)
	if _, ok := recovery.BeginTurn(taskstate.TurnRouteImplement); !ok {
		return nil, errProjectionFixture("begin recovery-pending turn")
	}
	recovery.RecordChanged("outcome.txt")
	if !recovery.RecoverActiveTurn() {
		return nil, errProjectionFixture("recover recovery-pending turn")
	}

	current, err := newCompletionProjectionTask("projection-current")
	if err != nil {
		return nil, err
	}
	if err := finishProjectionTurn(current, taskstate.TurnRouteComplete, "PASS", taskstate.StopGoalComplete); err != nil {
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

func finishProjectionTurn(task *taskstate.Task, route taskstate.TurnRoute, evidence string, stop taskstate.StopReason) error {
	sequence, ok := task.BeginTurn(route)
	if !ok {
		return errProjectionFixture("begin projection turn")
	}
	if !task.FinishTurn(sequence, route, "record the bounded projection state", evidence, stop, 0, 0) {
		return errProjectionFixture("finish projection turn")
	}
	return nil
}

// NewWorkspaceBoundCompletionFixture creates a current, workspace-bound proof
// that can be saved, reloaded fail-closed, and rebound from a fresh snapshot.
func NewWorkspaceBoundCompletionFixture(root string) (WorkspaceBoundCompletionFixture, error) {
	if root == "" {
		return WorkspaceBoundCompletionFixture{}, errors.New("workspace root is required")
	}
	const file = "outcome.txt"
	if err := os.WriteFile(filepath.Join(root, file), []byte("stable outcome\n"), 0o600); err != nil {
		return WorkspaceBoundCompletionFixture{}, err
	}
	task, err := newCompletionProjectionTask("projection-reload")
	if err != nil {
		return WorkspaceBoundCompletionFixture{}, err
	}
	task.RecordChanged(file)
	if err := finishProjectionTurn(task, taskstate.TurnRouteVerify, "PASS", taskstate.StopNone); err != nil {
		return WorkspaceBoundCompletionFixture{}, err
	}
	observation, err := workspace.Capture(context.Background(), root, []string{file})
	if err != nil {
		return WorkspaceBoundCompletionFixture{}, err
	}
	task.AddVerificationForCriterion(0, "verify", true, "verify PASS workspace-bound outcome", &observation)
	if !task.CompletionReady() {
		return WorkspaceBoundCompletionFixture{}, errors.New("workspace-bound proof is not current")
	}
	return WorkspaceBoundCompletionFixture{Task: task, Observation: observation}, nil
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
