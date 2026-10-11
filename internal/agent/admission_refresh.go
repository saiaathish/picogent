package agent

import (
	"errors"
	"fmt"
	"strings"

	"github.com/saiaathish/picogent/internal/taskstate"
)

// taskAdmissionRefresh is a one-shot permission for a later Run invocation to
// reload task state after a fresh/successor admission lost its initial CAS.
// It deliberately names the exact authority, not just the session text.
type taskAdmissionRefresh struct {
	store             *taskstate.Store
	sessionID         string
	sessionGeneration uint64
	storeGeneration   uint64
}

func (r taskAdmissionRefresh) matches(binding nativeTaskBinding) bool {
	return r.store != nil && r.sessionID != "" &&
		r.store == binding.store && r.sessionID == binding.sessionID &&
		r.sessionGeneration == binding.generation && r.storeGeneration == binding.storeGeneration
}

// refreshPendingAdmissionForNewRun consumes the refresh only at the start of
// a later Run, after its project lock is held and before its task owner is
// frozen. Mutation callbacks never call this path, so a refused old turn cannot
// be revived into a replacement task.
func (a *Agent) refreshPendingAdmissionForNewRun(binding nativeTaskBinding, workspace string) error {
	if a == nil {
		return nil
	}
	a.undoMu.Lock()
	defer a.undoMu.Unlock()
	a.taskMu.Lock()
	defer a.taskMu.Unlock()

	pending := a.pendingAdmissionRefresh
	if pending == nil {
		return nil
	}
	if !pending.matches(binding) {
		// A store/session change invalidates the old retry permission. Never
		// carry it across an ABA transition or into a newly attached session.
		a.pendingAdmissionRefresh = nil
		return nil
	}
	if a.taskRunBinding != nil {
		return errTaskOwnershipChanged
	}
	if a.TaskStore != binding.store || a.TaskSession != binding.sessionID || a.taskSessionGeneration != binding.generation || a.taskStoreGeneration != binding.storeGeneration {
		return errTaskOwnershipChanged
	}
	if a.undoReattachRequired {
		return errors.New("task store authority changed; explicitly reattach the session before retrying admission")
	}

	task, err := binding.store.OwnershipSnapshot(binding.sessionID)
	if errors.Is(err, taskstate.ErrNotFound) {
		task, err = nil, nil
	}
	if err != nil {
		return fmt.Errorf("reload durable task after admission conflict: %w", err)
	}

	var refreshedUndo *turnUndo
	var undoErr error
	if strings.TrimSpace(workspace) == "" {
		undoErr = errors.New("undo recovery is unavailable without a configured workspace")
	} else {
		refreshedUndo, undoErr = loadValidatedDurableUndo(workspace, binding.sessionID, binding.generation, task, undoTaskStoreAuthority{store: binding.store, epoch: binding.storeGeneration})
		if undoErr != nil && !errors.Is(undoErr, errLegacyUndoJournal) {
			return fmt.Errorf("validate undo recovery after admission conflict: %w", undoErr)
		}
	}
	if task != nil {
		if err := prepareTaskForAttachment(workspace, binding.store, task); err != nil {
			return fmt.Errorf("prepare durable task after admission conflict: %w", err)
		}
	}
	if refreshedUndo != nil && task != nil {
		if err := validateDurableUndoTask(refreshedUndo, task); err != nil {
			return fmt.Errorf("validate refreshed undo owner after admission conflict: %w", err)
		}
	}

	// Keep a process-only checkpoint when it is still bound to this exact
	// session/store authority. Durable undo is reloaded above against the newly
	// observed task owner; no journal is removed by this refresh.
	processUndo := a.latestUndo
	keepProcessUndo := strings.TrimSpace(workspace) != "" && processUndo != nil && !processUndo.durable &&
		processUndo.sessionID == binding.sessionID && processUndo.sessionGeneration == binding.generation &&
		(!processUndo.taskStoreBound || processUndo.taskStore == binding.store && processUndo.taskStoreEpoch == binding.storeGeneration)

	a.task = task
	a.taskLoadErr = nil
	if refreshedUndo != nil {
		a.latestUndo = refreshedUndo
		a.undoLoadErr = nil
	} else if keepProcessUndo {
		a.latestUndo = processUndo
		a.undoLoadErr = nil
	} else {
		a.latestUndo = nil
		a.undoLoadErr = undoErr
	}
	a.pendingAdmissionRefresh = nil
	return nil
}
