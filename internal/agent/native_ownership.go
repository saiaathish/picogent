package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/saiaathish/picogent/internal/taskstate"
)

// A native tool owns the admitted turn, not a later task loaded during a
// callback. Persistence revisions may advance under the same owner; task,
// intent, turn and session-generation changes revoke the original authority.
type nativeTaskBinding struct {
	store           *taskstate.Store
	sessionID       string
	generation      uint64
	storeGeneration uint64
	owner           *taskMutationOwner
}

// Capture the saved admission snapshot before notifying caller callbacks.
// Lazy admission may adopt that exact new task/turn, never state rebound by
// the notification itself. All other events retain their normal handler.
type nativeAdmissionEvents struct {
	EventHandler
	snapshot *taskstate.Task
}

func (h *nativeAdmissionEvents) OnTaskState(task *taskstate.Task) {
	h.snapshot = cloneTask(task)
	emitTaskState(h.EventHandler, task)
}

func (b nativeTaskBinding) withAdmittedTask(task *taskstate.Task) nativeTaskBinding {
	if task != nil {
		owner := taskOwner(task)
		b.owner = &owner
	}
	return b
}

func (a *Agent) nativeTaskBinding() nativeTaskBinding {
	a.taskMu.RLock()
	defer a.taskMu.RUnlock()
	binding := nativeTaskBinding{store: a.TaskStore, sessionID: a.TaskSession, generation: a.taskSessionGeneration, storeGeneration: a.taskStoreGeneration}
	if a.task != nil {
		owner := taskOwner(a.task)
		binding.owner = &owner
	}
	return binding
}

func (b nativeTaskBinding) owns(task *taskstate.Task) bool {
	if b.owner == nil {
		return task == nil
	}
	return task != nil && taskOwner(task) == *b.owner
}

func (a *Agent) checkNativeTaskBinding(ctx context.Context, b nativeTaskBinding) error {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	a.taskMu.RLock()
	defer a.taskMu.RUnlock()
	return a.checkTaskBindingLocked(b)
}

func (a *Agent) checkTaskBindingLocked(b nativeTaskBinding) error {
	if err := a.checkTaskBindingMemoryLocked(b); err != nil {
		return err
	}
	if b.store == nil || b.sessionID == "" {
		return nil // process-only undo still uses the caller's session binding
	}
	current, err := b.store.OwnershipSnapshot(b.sessionID)
	if errors.Is(err, taskstate.ErrNotFound) && b.owner == nil {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check native write task ownership: %w", err)
	}
	if !b.owns(current) {
		return errTaskOwnershipChanged
	}
	return nil
}

func (a *Agent) checkTaskBindingMemoryLocked(b nativeTaskBinding) error {
	if a.TaskStore != b.store || a.TaskSession != b.sessionID || a.taskSessionGeneration != b.generation || a.taskStoreGeneration != b.storeGeneration || !b.owns(a.task) {
		return errTaskOwnershipChanged
	}
	if a.taskLoadErr != nil {
		return fmt.Errorf("native write requires available task state: %w", a.taskLoadErr)
	}
	return nil
}
