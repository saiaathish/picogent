package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/saiaathish/picogent/internal/taskstate"
)

var errTaskSessionSwitchInProgress = errors.New("durable task session switch in progress")

// reserveTaskSessionSwitch revokes the active run before waiting for the
// project lock, and prevents a queued run from slipping ahead of the switch.
func (a *Agent) reserveTaskSessionSwitch(binding nativeTaskBinding) (func(), error) {
	a.taskMu.Lock()
	if a.TaskStore != binding.store || a.TaskSession != binding.sessionID || a.taskSessionGeneration != binding.generation || a.taskStoreGeneration != binding.storeGeneration {
		a.taskMu.Unlock()
		return nil, errTaskOwnershipChanged
	}
	if a.taskSessionSwitching {
		a.taskMu.Unlock()
		return nil, errTaskSessionSwitchInProgress
	}
	a.taskSessionSwitching = true
	cancelRun := a.taskRunCancel
	a.taskMu.Unlock()
	if cancelRun != nil {
		cancelRun()
	}
	return func() {
		a.taskMu.Lock()
		a.taskSessionSwitching = false
		a.taskMu.Unlock()
	}, nil
}

func (a *Agent) claimTaskRun(parent context.Context, binding nativeTaskBinding) (context.Context, error) {
	if parent == nil {
		parent = context.Background()
	}
	runCtx, cancel := context.WithCancel(parent)
	a.taskMu.Lock()
	if a.taskSessionSwitching {
		a.taskMu.Unlock()
		cancel()
		return nil, errTaskSessionSwitchInProgress
	}
	if a.taskRunBinding != nil || a.TaskStore != binding.store || a.TaskSession != binding.sessionID || a.taskSessionGeneration != binding.generation || a.taskStoreGeneration != binding.storeGeneration || !binding.owns(a.task) {
		a.taskMu.Unlock()
		cancel()
		return nil, errTaskOwnershipChanged
	}
	a.taskRunBinding = &binding
	a.taskRunCancel = cancel
	a.taskMu.Unlock()
	return runCtx, nil
}

func (a *Agent) checkTaskRun(ctx context.Context) error {
	a.taskMu.RLock()
	defer a.taskMu.RUnlock()
	if a.taskRunBinding == nil {
		return errTaskOwnershipChanged
	}
	if err := a.checkTaskBindingLocked(*a.taskRunBinding); err != nil {
		return err
	}
	if ctx != nil {
		if a.taskSessionSwitching {
			return context.Canceled
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return nil
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
	if err := a.checkTaskBindingMemoryLocked(*a.taskRunBinding); err != nil {
		return err
	}
	if a.taskSessionSwitching {
		return context.Canceled
	}
	return nil
}

// Cleanup has the exact original durable binding, not the currently installed
// authority. It may interrupt its own still-active record, but never finalize
// narration, restore files, remove undo journals, or mutate a replacement.
func (a *Agent) releaseTaskRun() (*taskstate.Task, error) {
	a.taskMu.Lock()
	if a.taskRunBinding == nil {
		cancel := a.taskRunCancel
		a.taskRunCancel = nil
		a.taskMu.Unlock()
		if cancel != nil {
			cancel()
		}
		return nil, nil
	}
	binding := *a.taskRunBinding
	cancel := a.taskRunCancel
	a.taskRunBinding = nil
	a.taskRunCancel = nil
	a.taskMu.Unlock()
	if cancel != nil {
		cancel()
	}
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
