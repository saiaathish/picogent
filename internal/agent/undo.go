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

func (u *turnUndo) restore() (string, error) {
	result, err := u.checkpoint.Restore()
	if err != nil {
		if len(result.Conflicts) > 0 {
			paths := make([]string, 0, len(result.Conflicts))
			for _, conflict := range result.Conflicts {
				paths = append(paths, conflict.Path)
			}
			sort.Strings(paths)
			return "", fmt.Errorf("undo blocked because newer changes exist in %s", strings.Join(paths, ", "))
		}
		return "", fmt.Errorf("undo failed: %w", err)
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
		return "last turn had nothing left to undo", nil
	}
	return "Undid last turn: " + strings.Join(parts, "; "), nil
}

// UndoLastTurn restores the latest completed turn that changed native workspace
// files. Read-only turns do not discard the most recent undo checkpoint.
func (a *Agent) UndoLastTurn() (string, error) {
	a.undoMu.Lock()
	defer a.undoMu.Unlock()
	if a.latestUndo == nil {
		return "nothing to undo", nil
	}
	msg, err := a.latestUndo.restore()
	if err != nil {
		return "", err
	}
	a.latestUndo = nil
	_, err = a.mutateTaskResult(func(task *taskstate.Task) error {
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
	})
	if err != nil && !errors.Is(err, errTaskMutationSkipped) {
		return "", fmt.Errorf("files restored but durable task state was not saved: %w", err)
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
