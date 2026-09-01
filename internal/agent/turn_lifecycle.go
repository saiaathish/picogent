package agent

import (
	"errors"
	"fmt"
	"strings"

	"github.com/saiaathish/picogent/internal/taskstate"
)

func (a *Agent) beginDurableTurn(route taskstate.TurnRoute, ev EventHandler) (uint64, bool) {
	var sequence uint64
	if !a.mutateTask(ev, func(task *taskstate.Task) error {
		var ok bool
		sequence, ok = task.BeginTurn(route)
		if !ok {
			return errors.New("durable turn could not be started")
		}
		return nil
	}) {
		return 0, false
	}
	return sequence, true
}

func (a *Agent) closeDurableTurn(sequence uint64, interrupted bool, route taskstate.TurnRoute, hypothesis, evidence string, stop taskstate.StopReason, toolRounds, mutations int, ev EventHandler) (bool, error) {
	snapshot, err := a.mutateTaskResult(func(task *taskstate.Task) error {
		var closed bool
		if interrupted {
			closed = task.InterruptTurn(sequence, route, hypothesis, evidence, stop, toolRounds, mutations)
		} else {
			closed = task.FinishTurn(sequence, route, hypothesis, evidence, stop, toolRounds, mutations)
		}
		if !closed {
			return errTaskMutationSkipped
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errTaskMutationSkipped) {
			return false, nil
		}
		var persistenceErr *taskPersistenceError
		if errors.As(err, &persistenceErr) {
			err = fmt.Errorf("durable task state was not saved: %w", err)
			if ev != nil {
				ev.OnError(err)
			}
			return false, err
		}
		a.reportTaskUpdateError(ev, err)
		return false, nil
	}
	if snapshot == nil {
		return false, nil
	}
	emitTaskState(ev, snapshot)
	return true, nil
}

// finishAndCloseDurableTurn combines the terminal task transition and the
// durable turn close into one save-before-publish/CAS commit. The active turn
// was already persisted before the provider call, and mutation/verification
// transitions are persisted as they occur; only the two final records can be
// safely committed together. If task finalization is logically refused (for
// example, proof is still missing), the old task state is restored in the
// candidate and the turn is still closed, matching finishDurableTask followed
// by closeDurableTurn without an intermediate durable write.
func (a *Agent) finishAndCloseDurableTurn(sequence uint64, text, blocker string, mode TaskMode, evidence string, filesChanged []string, completionMarker bool, goal, scopeBoundary string, toolRounds, mutations int, ev EventHandler) (*taskstate.Task, CompletionProjection, bool, bool, bool, error) {
	var finishErr error
	var completion CompletionProjection
	var goalDone bool
	snapshot, err := a.mutateTaskResult(func(task *taskstate.Task) error {
		before := cloneTask(task)
		finishErr = applyDurableTaskFinish(task, text, blocker)
		if finishErr != nil {
			// Preserve the existing behavior for a logical completion refusal:
			// close the active turn against the last valid task state, while
			// reporting the refusal after the combined commit succeeds.
			*task = *before
		}
		completion = completionProjection(task, goal, completionMarker, verificationStatus(evidence) == "PASS", len(filesChanged), scopeBoundary)
		goalDone = completionMarker && completion.Ready
		stop := taskstate.StopNone
		if goalDone || task.Status == taskstate.StatusDone {
			stop = taskstate.StopGoalComplete
		} else if task.Status == taskstate.StatusBlocked {
			stop = task.StopReason
		}
		route := durableTurnRouteForOutcome(task, mode, evidence, blocker, filesChanged, goalDone, false, false)
		hypothesis := durableTurnHypothesis(task, mode, evidence, blocker, filesChanged, goalDone, false, false)
		closed := task.FinishTurn(sequence, route, hypothesis, evidence, stop, toolRounds, mutations)
		if !closed {
			return errTaskMutationSkipped
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errTaskMutationSkipped) {
			// mutateTaskResult rebases the in-memory task before replaying the
			// mutation. A stale sequence means that replay correctly refused to
			// close a replacement turn; return the rebased snapshot so callers can
			// report the current task without applying the stale result to it.
			return a.TaskSnapshot(), CompletionProjection{}, false, false, true, nil
		}
		var persistenceErr *taskPersistenceError
		if errors.As(err, &persistenceErr) {
			err = fmt.Errorf("durable task state was not saved: %w", err)
			if ev != nil {
				ev.OnError(err)
			}
			return a.TaskSnapshot(), CompletionProjection{}, false, false, false, err
		}
		a.reportTaskUpdateError(ev, err)
		return a.TaskSnapshot(), CompletionProjection{}, false, false, false, nil
	}
	if finishErr != nil && !errors.Is(finishErr, errTaskMutationSkipped) {
		a.reportTaskUpdateError(ev, finishErr)
	}
	if snapshot == nil {
		return nil, completion, goalDone, false, false, nil
	}
	emitTaskState(ev, snapshot)
	return snapshot, completion, goalDone, true, false, nil
}

func durableTurnStartRoute(task *taskstate.Task, mode TaskMode) taskstate.TurnRoute {
	if task != nil {
		if last := task.LastTurn(); last != nil && (last.State == taskstate.TurnInterrupted || last.Route == string(taskstate.TurnRouteRecover)) {
			return taskstate.TurnRouteRecover
		}
		if task.Status == taskstate.StatusVerifying || task.NeedsVerification() {
			return taskstate.TurnRouteVerify
		}
		if len(task.ChangedFiles) > 0 {
			return taskstate.TurnRouteImplement
		}
		if mode.ReadOnly() {
			return taskstate.TurnRouteInspect
		}
		if len(task.Turns) == 0 && task.Attempts <= 1 {
			return taskstate.TurnRouteAdmission
		}
	}
	if mode.ReadOnly() {
		return taskstate.TurnRouteInspect
	}
	return taskstate.TurnRouteOther
}

func durableTurnRouteForOutcome(task *taskstate.Task, mode TaskMode, evidence, blocker string, filesChanged []string, goalDone, interrupted, failed bool) taskstate.TurnRoute {
	if interrupted || failed {
		return taskstate.TurnRouteRecover
	}
	if strings.TrimSpace(blocker) != "" || (task != nil && task.Status == taskstate.StatusBlocked) {
		return taskstate.TurnRouteBlocked
	}
	if goalDone || (task != nil && task.Status == taskstate.StatusDone) {
		return taskstate.TurnRouteComplete
	}
	if strings.TrimSpace(evidence) != "" || (task != nil && task.Status == taskstate.StatusVerifying) {
		return taskstate.TurnRouteVerify
	}
	if len(filesChanged) > 0 {
		return taskstate.TurnRouteImplement
	}
	if mode.ReadOnly() {
		return taskstate.TurnRouteInspect
	}
	return taskstate.TurnRouteOther
}

func durableTurnHypothesis(task *taskstate.Task, mode TaskMode, evidence, blocker string, filesChanged []string, goalDone, interrupted, failed bool) string {
	switch {
	case interrupted:
		return "turn canceled before completion"
	case failed:
		return "provider or permission call failed"
	case strings.TrimSpace(blocker) != "" || (task != nil && task.Status == taskstate.StatusBlocked):
		return "turn stopped at a durable blocker"
	case goalDone || (task != nil && task.Status == taskstate.StatusDone):
		return "completed the requested outcome"
	case strings.TrimSpace(evidence) != "" || (task != nil && task.Status == taskstate.StatusVerifying):
		return "checked evidence for the requested outcome"
	case len(filesChanged) > 0:
		return "implemented the requested change"
	case mode.ReadOnly():
		return "inspected the requested outcome"
	default:
		return "completed the requested turn"
	}
}
