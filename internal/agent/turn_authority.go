package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/saiaathish/picogent/internal/taskstate"
)

func (a *Agent) claimTaskRun(binding nativeTaskBinding) error {
	a.taskMu.Lock()
	defer a.taskMu.Unlock()
	if a.taskRunBinding != nil || a.TaskStore != binding.store || a.TaskSession != binding.sessionID || a.taskSessionGeneration != binding.generation || a.taskStoreGeneration != binding.storeGeneration || !binding.owns(a.task) {
		return errTaskOwnershipChanged
	}
	a.taskRunBinding = &binding
	return nil
}

func (a *Agent) checkTaskRun(ctx context.Context) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	a.taskMu.RLock()
	defer a.taskMu.RUnlock()
	if a.taskRunBinding == nil {
		return errTaskOwnershipChanged
	}
	return a.checkTaskBindingLocked(*a.taskRunBinding)
}

func (a *Agent) checkTaskLockBinding(binding nativeTaskBinding) error {
	a.taskMu.RLock()
	defer a.taskMu.RUnlock()
	if a.TaskStore != binding.store || a.TaskSession != binding.sessionID || a.taskSessionGeneration != binding.generation || a.taskStoreGeneration != binding.storeGeneration {
		return errTaskOwnershipChanged
	}
	return nil
}

// Streaming uses a cheap in-process revocation fence per delta. The durable
// owner is rechecked after Chat; refusal retracts the partial bubble. Never
// use this memory-only check to authorize persistence or tool effects.
func (a *Agent) checkTaskRunMemory() error {
	a.taskMu.RLock()
	defer a.taskMu.RUnlock()
	if a.taskRunBinding == nil {
		return errTaskOwnershipChanged
	}
	return a.checkTaskBindingMemoryLocked(*a.taskRunBinding)
}

// Cleanup has the exact original durable binding, not the currently installed
// authority. It may interrupt its own still-active record, but never finalize
// narration, restore files, remove undo journals, or mutate a replacement.
func (a *Agent) releaseTaskRun() (*taskstate.Task, error) {
	a.taskMu.Lock()
	if a.taskRunBinding == nil {
		a.taskMu.Unlock()
		return nil, nil
	}
	binding := *a.taskRunBinding
	a.taskRunBinding = nil
	a.taskMu.Unlock()
	if binding.store == nil || binding.owner == nil || binding.owner.lastState != taskstate.TurnActive {
		return nil, nil
	}
	for attempt := 0; attempt < maxTaskMutationAttempts; attempt++ {
		current, err := binding.store.OwnershipSnapshot(binding.sessionID)
		if err != nil {
			return nil, fmt.Errorf("recover original exited turn: %w", err)
		}
		if !binding.owns(current) {
			return nil, nil // a replacement or another closer owns this record
		}
		if !current.InterruptTurn(binding.owner.turn, taskstate.TurnRouteRecover, "execution exited before its bound turn closed", "", taskstate.StopResourceUnavailable, 0, 0) {
			return nil, nil
		}
		if err := binding.store.Save(current); err != nil {
			if errors.Is(err, taskstate.ErrRevisionConflict) && attempt+1 < maxTaskMutationAttempts {
				continue
			}
			return nil, fmt.Errorf("save original exited turn recovery: %w", err)
		}
		a.taskMu.Lock()
		// A store ABA can end back on the original record. Refresh only its
		// original session/owner, never a different installed store/session.
		matches := a.TaskStore == binding.store && a.TaskSession == binding.sessionID && a.taskSessionGeneration == binding.generation && binding.owns(a.task)
		if matches {
			a.task = current
		}
		a.taskMu.Unlock()
		if matches {
			return cloneTask(current), nil
		}
		return nil, nil
	}
	return nil, taskstate.ErrRevisionConflict
}
