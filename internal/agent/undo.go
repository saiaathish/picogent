package agent

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/saiaathish/picogent/internal/checkpoint"
	"github.com/saiaathish/picogent/internal/taskstate"
)

// turnUndo aggregates the per-path snapshots captured before native file tools
// run. A path is captured once, even when the model edits it in several tool
// rounds during the same turn.
type turnUndo struct {
	workspace         string
	checkpoint        *checkpoint.Checkpoint
	sessionID         string
	sessionGeneration uint64
	turnSequence      uint64
	durable           bool
	journalSlot       string
	restored          bool
	restoreMessage    string
	restoreErr        error
}

const maxUndoTaskPersistenceAttempts = 3

func newTurnUndo(workspace, sessionID string, sessionGeneration uint64) *turnUndo {
	return &turnUndo{workspace: workspace, sessionID: sessionID, sessionGeneration: sessionGeneration}
}

// preparePublish persists a recovery-pending record immediately before the
// workspace atomic rename. The record contains only paths whose expected
// post-write state is already known, so a crash during a multi-file turn can
// recover the writes that actually reached publication.
func (u *turnUndo) preparePublish(path string, data []byte, mode os.FileMode) error {
	if u == nil || u.checkpoint == nil || u.sessionID == "" || u.turnSequence == 0 {
		return nil
	}
	changed, err := u.checkpoint.PrepareExpected(path, data, mode)
	if err != nil {
		return err
	}
	record, err := u.checkpoint.Export()
	if err != nil {
		return err
	}
	if len(record.Entries) == 0 {
		if u.journalSlot == undoJournalPending {
			return u.discardPending()
		}
		return nil
	}
	if !changed && !u.durable {
		return nil
	}
	if err := u.saveJournal(record, undoJournalPending); err != nil {
		return err
	}
	u.durable = true
	u.journalSlot = undoJournalPending
	return nil
}

func (u *turnUndo) saveJournal(record checkpoint.Record, state string) error {
	return u.saveJournalAt(record, state, state == undoJournalPending)
}

func (u *turnUndo) saveJournalAt(record checkpoint.Record, state string, pending bool) error {
	if u == nil || u.sessionID == "" || u.turnSequence == 0 {
		return nil
	}
	identity, err := undoWorkspaceIdentity(u.workspace)
	if err != nil {
		return err
	}
	j := undoJournal{
		Version:      undoJournalVersion,
		State:        state,
		Workspace:    identity,
		SessionID:    u.sessionID,
		TurnSequence: u.turnSequence,
		Checkpoint:   record,
	}
	return saveUndoJournal(u.workspace, u.sessionID, j, pending)
}

func (u *turnUndo) persistSealed() error {
	if u == nil || u.checkpoint == nil || u.sessionID == "" || u.turnSequence == 0 {
		return nil
	}
	record, err := u.checkpoint.Export()
	if err != nil {
		return err
	}
	if len(record.Entries) == 0 {
		return u.discardPending()
	}
	if err := u.saveJournal(record, undoJournalSealed); err != nil {
		return err
	}
	u.durable = true
	u.journalSlot = undoJournalSealed
	if err := removeUndoJournal(u.workspace, u.sessionID, true); err != nil {
		return err
	}
	return nil
}

func (u *turnUndo) persistRestored() error {
	if u == nil || !u.durable || u.checkpoint == nil {
		return nil
	}
	record, err := u.checkpoint.Export()
	if err != nil {
		return err
	}
	if len(record.Entries) == 0 {
		return errors.New("restored undo checkpoint has no entries")
	}
	slot := u.journalSlot
	if slot != undoJournalPending && slot != undoJournalSealed {
		slot = undoJournalSealed
	}
	if err := u.saveJournalAt(record, undoJournalRestored, slot == undoJournalPending); err != nil {
		return err
	}
	return nil
}

func (u *turnUndo) discardPending() error {
	if u == nil || u.sessionID == "" || u.turnSequence == 0 || u.journalSlot != undoJournalPending {
		return nil
	}
	if err := removeUndoJournal(u.workspace, u.sessionID, true); err != nil {
		return err
	}
	u.durable = false
	u.journalSlot = ""
	return nil
}

func (u *turnUndo) finalizeJournal() error {
	if u == nil || !u.durable {
		return nil
	}
	if err := removeAllUndoJournals(u.workspace, u.sessionID); err != nil {
		return err
	}
	u.durable = false
	u.journalSlot = ""
	return nil
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
	// A complete restore consumes the checkpoint. Cache its result so a later
	// retry of durable task persistence does not attempt to restore it again.
	if u.restored {
		return u.restoreMessage, true, u.restoreErr
	}
	result, err := u.checkpoint.Restore()
	msg, complete, restoreErr := formatUndoRestore(result, err)
	if complete {
		u.restored = true
		u.restoreMessage = msg
		u.restoreErr = restoreErr
	}
	return msg, complete, restoreErr
}

func formatUndoRestore(result checkpoint.RestoreResult, err error) (string, bool, error) {
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
	releaseRun, err := a.acquireProjectRunLockForWorkspace(a.ConfigSnapshot().Workspace)
	if err != nil {
		return "", fmt.Errorf("project run is unavailable: %w", err)
	}
	defer releaseRun()
	a.undoMu.Lock()
	defer a.undoMu.Unlock()
	if a.latestUndo == nil {
		workspace := a.ConfigSnapshot().Workspace
		sessionID, generation := a.taskSessionSnapshot()
		if sessionID != "" {
			loaded, loadErr := loadLatestDurableUndo(workspace, sessionID, generation)
			if loadErr != nil {
				a.undoLoadErr = loadErr
				return "", fmt.Errorf("undo is unavailable: %w", loadErr)
			}
			if task := a.TaskSnapshot(); task != nil {
				if validationErr := validateDurableUndoTask(loaded, task); validationErr != nil {
					a.undoLoadErr = validationErr
					return "", fmt.Errorf("undo is unavailable: %w", validationErr)
				}
			}
			a.undoLoadErr = nil
			a.latestUndo = loaded
		}
	}
	if a.undoLoadErr != nil {
		return "", fmt.Errorf("undo is unavailable: %w", a.undoLoadErr)
	}
	if a.latestUndo == nil {
		return "nothing to undo", nil
	}
	if !a.undoBelongsToCurrentSession(a.latestUndo) {
		a.latestUndo = nil
		return "nothing to undo", nil
	}
	msg, complete, restoreErr := a.latestUndo.restore()
	if !complete {
		if restoreErr == nil {
			restoreErr = errors.New("undo failed: workspace restoration was incomplete")
		}
		return "", restoreErr
	}
	if err := a.latestUndo.persistRestored(); err != nil {
		stateErr := fmt.Errorf("files restored but durable undo recovery was not recorded; retry /undo: %w", err)
		if restoreErr != nil {
			return "", errors.Join(stateErr, restoreErr)
		}
		return "", stateErr
	}
	if a.latestUndo.durable && a.TaskStoreSnapshot() != nil && a.TaskSnapshot() == nil {
		return "", errors.New("files restored but durable task state is unavailable; restore task state and retry /undo")
	}
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
	err = a.persistUndoTaskMutation(undoMutation)
	if err != nil && !errors.Is(err, errTaskMutationSkipped) {
		stateErr := fmt.Errorf("files restored but durable task state was not saved; retry /undo to finish recovery: %w", err)
		if restoreErr != nil {
			return "", errors.Join(stateErr, restoreErr)
		}
		return "", stateErr
	}
	if err := a.latestUndo.finalizeJournal(); err != nil {
		stateErr := fmt.Errorf("workspace and durable task state were recovered but undo cleanup was not saved; retry /undo: %w", err)
		if restoreErr != nil {
			return "", errors.Join(stateErr, restoreErr)
		}
		return "", stateErr
	}
	a.latestUndo = nil
	if restoreErr != nil {
		return "", restoreErr
	}
	return msg, nil
}

// persistUndoTaskMutation retries an undo invalidation against the newest
// durable task after a compare-and-swap conflict. Undo has already changed
// workspace files by this point, so it must preserve concurrent task progress
// instead of retrying a stale in-memory snapshot.
func (a *Agent) persistUndoTaskMutation(mutate func(*taskstate.Task) error) error {
	var lastErr error
	for attempt := 0; attempt < maxUndoTaskPersistenceAttempts; attempt++ {
		_, err := a.mutateTaskResult(mutate)
		if err == nil || errors.Is(err, errTaskMutationSkipped) {
			return err
		}
		lastErr = err
		if !errors.Is(err, taskstate.ErrRevisionConflict) || attempt == maxUndoTaskPersistenceAttempts-1 {
			return err
		}
		if err := a.rebaseTaskFromStore(); err != nil {
			return errors.Join(lastErr, err)
		}
	}
	return lastErr
}

// UndoAvailable reports whether the latest completed native-file turn still
// has an in-memory checkpoint that can be restored.
func (a *Agent) UndoAvailable() bool {
	a.undoMu.Lock()
	defer a.undoMu.Unlock()
	return a.latestUndo != nil && a.undoLoadErr == nil
}

func (a *Agent) finishTurnUndo(res *Result, u *turnUndo, nativeWriteRan bool) {
	if !nativeWriteRan {
		return
	}
	paths, err := u.seal()
	if err != nil {
		res.UndoError = err.Error()
		a.undoMu.Lock()
		if u.durable && a.undoBelongsToCurrentSession(u) {
			a.latestUndo = u
			res.UndoAvailable = true
		} else {
			a.latestUndo = nil
		}
		a.undoMu.Unlock()
		return
	}
	res.FilesChanged = paths
	if len(paths) == 0 {
		if err := u.discardPending(); err != nil {
			res.UndoError = err.Error()
		}
		return
	}
	a.undoMu.Lock()
	if !a.undoBelongsToCurrentSession(u) {
		a.undoMu.Unlock()
		if err := u.discardPending(); err != nil {
			res.UndoError = err.Error()
		}
		res.UndoAvailable = false
		return
	}
	if err := u.persistSealed(); err != nil {
		res.UndoError = fmt.Errorf("durable undo journal was not finalized; recovery remains retryable: %w", err).Error()
	}
	res.UndoAvailable = true
	a.latestUndo = u
	a.undoMu.Unlock()
}

func (a *Agent) undoBelongsToCurrentSession(u *turnUndo) bool {
	if a == nil || u == nil {
		return false
	}
	currentWorkspace, err := undoWorkspaceIdentity(a.ConfigSnapshot().Workspace)
	if err != nil {
		return false
	}
	checkpointWorkspace, err := undoWorkspaceIdentity(u.workspace)
	if err != nil || currentWorkspace != checkpointWorkspace {
		return false
	}
	a.taskMu.RLock()
	defer a.taskMu.RUnlock()
	return u.sessionID == a.TaskSession && u.sessionGeneration == a.taskSessionGeneration
}

// validateDurableUndoTask binds a journal to the durable turn history that
// authorized it. A later read-only turn leaves the latest native-file undo
// useful, but a later mutating turn supersedes it. If the referenced turn is
// no longer present, the bounded history cannot prove that the record is
// current, so recovery fails closed.
func validateDurableUndoTask(u *turnUndo, task *taskstate.Task) error {
	if u == nil || task == nil {
		return nil
	}
	if task.SessionID != u.sessionID {
		return fmt.Errorf("durable undo journal task session mismatch")
	}
	found := false
	for _, turn := range task.Turns {
		if turn.Sequence == u.turnSequence {
			found = true
			continue
		}
		if found && (turn.MutationCount > 0 || len(turn.ChangedFiles) > 0) {
			return fmt.Errorf("durable undo journal was superseded by mutating turn %d", turn.Sequence)
		}
	}
	if !found {
		return fmt.Errorf("durable undo journal turn sequence %d is stale", u.turnSequence)
	}
	return nil
}
