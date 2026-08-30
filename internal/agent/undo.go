package agent

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/saiaathish/picogent/internal/checkpoint"
	"github.com/saiaathish/picogent/internal/taskstate"
)

// turnUndo aggregates the per-path snapshots captured before native file tools
// run. A path is captured once, even when the model edits it in several tool
// rounds during the same turn.
type turnUndo struct {
	workspace  string
	checkpoint *checkpoint.Checkpoint
}

func newTurnUndo(workspace string) *turnUndo {
	return &turnUndo{workspace: workspace}
}

func (u *turnUndo) capture(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("write path is empty")
	}
	if u.checkpoint == nil {
		cp, err := checkpoint.Capture(u.workspace, []string{path})
		if err != nil {
			return err
		}
		u.checkpoint = cp
		return nil
	}
	return u.checkpoint.Add([]string{path})
}

func (u *turnUndo) seal() ([]string, error) {
	if u == nil || u.checkpoint == nil {
		return nil, errors.New("no native file changes were captured")
	}
	if err := u.checkpoint.Seal(); err != nil {
		return nil, err
	}
	return u.checkpoint.ChangedPaths()
}

func (u *turnUndo) restore() (string, bool, error) {
	result, err := u.checkpoint.Restore()
	// Restore can return a cleanup warning after all workspace mutations have
	// been published. Complete is authoritative for whether the checkpoint is
	// consumed and durable evidence must be invalidated.
	if err != nil && !result.Complete {
		if len(result.Conflicts) > 0 {
			paths := make([]string, 0, len(result.Conflicts))
			for _, conflict := range result.Conflicts {
				paths = append(paths, conflict.Path)
			}
			sort.Strings(paths)
			return "", false, fmt.Errorf("undo blocked because newer changes exist in %s", strings.Join(paths, ", "))
		}
		return "", false, fmt.Errorf("undo failed: %w", err)
	}

	restored, removed, unchanged := result.Restored, result.Removed, result.Unchanged
	sort.Strings(restored)
	sort.Strings(removed)
	sort.Strings(unchanged)
	parts := make([]string, 0, 3)
	if len(restored) > 0 {
		parts = append(parts, "restored "+strings.Join(restored, ", "))
	}
	if len(removed) > 0 {
		parts = append(parts, "removed "+strings.Join(removed, ", "))
	}
	if len(unchanged) > 0 {
		parts = append(parts, "already unchanged "+strings.Join(unchanged, ", "))
	}
	if len(parts) == 0 {
		msg := "last turn had nothing left to undo"
		if err != nil {
			return msg, result.Complete, fmt.Errorf("%s; undo completed with warning: %w", msg, err)
		}
		return msg, result.Complete, nil
	}
	msg := "Undid last turn: " + strings.Join(parts, "; ")
	if err != nil {
		return msg, result.Complete, fmt.Errorf("%s; undo completed with warning: %w", msg, err)
	}
	return msg, result.Complete, nil
}

// UndoLastTurn restores the latest completed turn that changed native workspace
// files. Read-only turns do not discard the most recent undo checkpoint.
func (a *Agent) UndoLastTurn() (string, error) {
	a.undoMu.Lock()
	defer a.undoMu.Unlock()
	if a.latestUndo == nil {
		return "nothing to undo", nil
	}
	msg, complete, restoreErr := a.latestUndo.restore()
	if !complete {
		if restoreErr == nil {
			restoreErr = errors.New("undo failed: workspace restoration was incomplete")
		}
		return "", restoreErr
	}
	a.latestUndo = nil
	undoMutation := func(task *taskstate.Task) error {
		wasDone := task.Status == taskstate.StatusDone
		changed := task.InvalidateWorkspaceEvidence("undo restored workspace files")
		if wasDone {
			if err := task.SetStatus(taskstate.StatusVerifying); err != nil {
				return err
			}
			changed = true
		}
		if !changed {
			return errTaskMutationSkipped
		}
		return nil
	}
	_, err := a.mutateTaskResult(undoMutation)
	if errors.Is(err, taskstate.ErrRevisionConflict) && a.rebaseLegacyCompletionNormalization() {
		_, err = a.mutateTaskResult(undoMutation)
	}
	if err != nil && !errors.Is(err, errTaskMutationSkipped) {
		stateErr := fmt.Errorf("files restored but durable task state was not saved: %w", err)
		if restoreErr != nil {
			return "", errors.Join(stateErr, restoreErr)
		}
		return "", stateErr
	}
	if restoreErr != nil {
		return "", restoreErr
	}
	return msg, nil
}

// UndoAvailable reports whether the latest completed native-file turn still
// has an in-memory checkpoint that can be restored.
func (a *Agent) UndoAvailable() bool {
	a.undoMu.Lock()
	defer a.undoMu.Unlock()
	return a.latestUndo != nil
}

func (a *Agent) finishTurnUndo(res *Result, u *turnUndo, nativeWriteRan bool) {
	if !nativeWriteRan {
		return
	}
	paths, err := u.seal()
	if err != nil {
		res.UndoError = err.Error()
		a.undoMu.Lock()
		a.latestUndo = nil
		a.undoMu.Unlock()
		return
	}
	res.FilesChanged = paths
	if len(paths) == 0 {
		return
	}
	res.UndoAvailable = true
	a.undoMu.Lock()
	a.latestUndo = u
	a.undoMu.Unlock()
}
