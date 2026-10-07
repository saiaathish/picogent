package agent

import (
	"errors"
	"fmt"

	"github.com/saiaathish/picogent/internal/goal"
	"github.com/saiaathish/picogent/internal/taskstate"
)

var errReplacedGoalConflict = errors.New("a newer workspace goal conflicts with the replaced outcome; retry with the current goal")

// reconcileReplacedWorkspaceGoal completes a durable save-before-retire
// transaction. It returns the admitted tuple, never a later runtime snapshot.
// After retirement, acknowledging the exact marker makes later goals independent
// of this historical replacement. Any failure stops admission before the provider.
func (a *Agent) reconcileReplacedWorkspaceGoal(workspace, admittedGoal string, admittedRevision uint64) (string, uint64, error) {
	task := a.TaskSnapshot()
	if task == nil || task.ReplacedWorkspaceGoal == nil {
		return admittedGoal, admittedRevision, nil
	}
	expected := *task.ReplacedWorkspaceGoal
	retirementErr := a.retireWorkspaceGoal(workspace, expected)
	if retirementErr != nil && !errors.Is(retirementErr, errReplacedGoalConflict) {
		return admittedGoal, admittedRevision, retirementErr
	}
	// A conflicting goal has a different identity and must never be cleared by
	// this historical replacement. Acknowledge the obsolete marker, but stop
	// this admission: a fresh turn must capture the current runtime tuple.
	_, err := a.mutateTaskResult(func(candidate *taskstate.Task) error {
		if candidate.ID != task.ID || candidate.IntentRevision != task.IntentRevision || candidate.TurnRevision != task.TurnRevision {
			return errors.New("outcome changed before replacement retirement was acknowledged")
		}
		if candidate.ReplacedWorkspaceGoal != nil && *candidate.ReplacedWorkspaceGoal != expected {
			return errors.New("workspace goal retirement changed before acknowledgement")
		}
		candidate.ReplacedWorkspaceGoal = nil
		return nil
	})
	if err != nil {
		return admittedGoal, admittedRevision, fmt.Errorf("acknowledge replaced workspace goal: %w", err)
	}
	if retirementErr != nil {
		return admittedGoal, admittedRevision, retirementErr
	}
	return "", 0, nil
}

func (a *Agent) retireWorkspaceGoal(workspace string, expected taskstate.GoalRetirement) error {
	a.stateMu.Lock()
	defer a.stateMu.Unlock()
	if a.Goal != "" && (a.Goal != expected.Text || a.GoalRevision != expected.Revision) {
		return errReplacedGoalConflict
	}
	cleared, err := goal.ClearIfState(workspace, expected.Text, expected.Revision)
	if err != nil {
		return fmt.Errorf("retire replaced workspace goal: %w", err)
	}
	if !cleared {
		current, err := goal.LoadState(workspace)
		if err != nil {
			return fmt.Errorf("check replaced workspace goal: %w", err)
		}
		if current.Text != "" {
			// Refresh only the exact obsolete runtime identity. This newer tuple
			// is for the next admission, never injected into the stopped turn.
			if a.Goal == expected.Text && a.GoalRevision == expected.Revision {
				a.Goal, a.GoalRevision = current.Text, current.Revision
			}
			return errReplacedGoalConflict
		}
	}
	if a.Goal == expected.Text && a.GoalRevision == expected.Revision {
		a.Goal, a.GoalRevision = "", 0
	}
	return nil
}
