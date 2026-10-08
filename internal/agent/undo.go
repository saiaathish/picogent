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
	workspace          string
	checkpoint         *checkpoint.Checkpoint
	sessionID          string
	sessionGeneration  uint64
	taskID             string
	turnIntentRevision uint64
	taskStore          *taskstate.Store
	taskStoreEpoch     uint64
	taskStoreBound     bool
	turnSequence       uint64
	pendingUnpublished bool
	durable            bool
	journalSlot        string
	publishRejected    bool
	restored           bool
	restoreMessage     string
	restoreErr         error
}

type undoTaskStoreAuthority struct {
	store *taskstate.Store
	epoch uint64
}

const maxUndoTaskPersistenceAttempts = 3

func newTurnUndo(workspace, sessionID string, sessionGeneration uint64) *turnUndo {
	return &turnUndo{workspace: workspace, sessionID: sessionID, sessionGeneration: sessionGeneration}
}

func (u *turnUndo) bindTaskStore(store *taskstate.Store, epoch uint64) {
	if u == nil {
		return
	}
	u.taskStore = store
	u.taskStoreEpoch = epoch
	u.taskStoreBound = true
}

func (u *turnUndo) bindDurableTurn(task *taskstate.Task, sequence uint64) error {
	if u == nil || sequence == 0 {
		return nil
	}
	if task == nil || strings.TrimSpace(task.ID) == "" {
		return errors.New("durable undo requires the admitted task identity")
	}
	for _, turn := range task.Turns {
		if turn.Sequence == sequence {
			u.taskID = task.ID
			u.turnIntentRevision = turn.IntentRevision
			return nil
		}
	}
	return fmt.Errorf("durable undo cannot find admitted turn %d in task %s", sequence, task.ID)
}

// preparePublish records the exact native publication expectation immediately
// before the workspace atomic rename, including for process-local undo. When a
// durable turn identity is available, it also persists a recovery-pending
// record so a crash during a multi-file turn can recover published writes.
func (u *turnUndo) preparePublish(path string, data []byte, mode os.FileMode) (err error) {
	if u == nil || u.checkpoint == nil {
		return nil
	}
	defer func() {
		if err != nil {
			// Once a pre-publication hook rejects a write, the in-memory
			// checkpoint must not be sealed from the current workspace: the
			// current bytes may be an external edit that caused the rejection.
			u.publishRejected = true
		}
	}()
	changed, err := u.checkpoint.PrepareExpected(path, data, mode)
	if err != nil {
		return err
	}
	// Session and sequence identify a recovery journal; they do not gate the
	// in-memory expectation that keeps Seal from adopting a later user edit.
	if u.sessionID == "" || u.turnSequence == 0 {
		return nil
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
	if strings.TrimSpace(u.taskID) == "" {
		return errors.New("durable undo journal has no task owner identity")
	}
	identity, err := undoWorkspaceIdentity(u.workspace)
	if err != nil {
		return err
	}
	j := undoJournal{
		Version:        undoJournalVersion,
		State:          state,
		Workspace:      identity,
		SessionID:      u.sessionID,
		TurnSequence:   u.turnSequence,
		TaskID:         u.taskID,
		IntentRevision: u.turnIntentRevision,
		Checkpoint:     record,
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

// dropContentConflict removes an unprepared capture whose native edit was
// rejected before publication. An earlier exact publication on the same path
// remains conflict-aware undo; live bytes are never adopted as ownership.
func (u *turnUndo) dropContentConflict(path string) error {
	if u == nil || u.checkpoint == nil {
		return nil
	}
	return u.checkpoint.DropUnprepared(path)
}

func (u *turnUndo) seal() ([]string, error) {
	if u == nil || u.checkpoint == nil {
		return nil, errors.New("no native file changes were captured")
	}
	if err := u.checkpoint.SealPrepared(); err != nil {
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
	return a.undoLastTurnWithHook(nil)
}

func (a *Agent) undoLastTurnWithHook(beforeUndoLock func()) (string, error) {
	workspace := a.ConfigSnapshot().Workspace
	binding := a.nativeTaskBinding()
	releaseRun, err := a.acquireProjectRunLockForWorkspace(workspace, binding)
	if err != nil {
		return "", fmt.Errorf("project run is unavailable: %w", err)
	}
	defer releaseRun()
	if err := a.checkTaskLockBinding(binding); err != nil {
		return "", fmt.Errorf("undo authority changed while acquiring its lock: %w", err)
	}
	if beforeUndoLock != nil {
		beforeUndoLock()
	}
	a.undoMu.Lock()
	defer a.undoMu.Unlock()
	if strings.TrimSpace(a.ConfigSnapshot().Workspace) != strings.TrimSpace(workspace) {
		return "", fmt.Errorf("undo workspace: %w", errWorkspaceAuthorityChanged)
	}
	if a.undoReattachRequired {
		if a.latestUndo != nil && !a.latestUndo.durable {
			return "", errors.New("cached process-local undo no longer matches the current task-store authority")
		}
		return "", errors.New("task store authority changed; explicitly reattach the session before undo recovery")
	}
	if strings.TrimSpace(workspace) == "" {
		a.undoLoadErr = errors.New("undo recovery is unavailable without a configured workspace")
		return "", fmt.Errorf("undo is unavailable: %w", a.undoLoadErr)
	}
	if err := a.checkTaskLockBinding(binding); err != nil {
		return "", fmt.Errorf("undo authority changed while acquiring its lock: %w", err)
	}
	if a.latestUndo != nil && !a.undoBelongsToCurrentSession(a.latestUndo) {
		if !a.latestUndo.durable {
			return "", errors.New("cached process-local undo no longer matches the current session, workspace, or task-store authority")
		}
		return "", errors.New("cached undo belongs to a previous task store authority; reattach the session to recover it")
	}
	if a.latestUndo == nil {
		sessionID, generation := a.taskSessionSnapshot()
		if sessionID != "" {
			loaded, loadErr := loadValidatedDurableUndo(workspace, sessionID, generation, a.TaskSnapshot(), undoTaskStoreAuthority{store: binding.store, epoch: binding.storeGeneration})
			if loadErr != nil {
				a.undoLoadErr = loadErr
				return "", fmt.Errorf("undo is unavailable: %w", loadErr)
			}
			a.undoLoadErr = nil
			a.latestUndo = loaded
		}
	}
	if a.latestUndo != nil && a.latestUndo.durable {
		if refreshErr := a.refreshDurableUndoLocked(workspace); refreshErr != nil {
			a.undoLoadErr = refreshErr
			return "", fmt.Errorf("undo is unavailable: %w", refreshErr)
		}
	}
	if a.undoLoadErr != nil {
		return "", fmt.Errorf("undo is unavailable: %w", a.undoLoadErr)
	}
	if a.latestUndo == nil {
		return "nothing to undo", nil
	}
	if !a.undoBelongsToCurrentSession(a.latestUndo) {
		return "", errors.New("undo authority changed; recovery journal was preserved")
	}
	msg, complete, restoreErr := a.latestUndo.restore()
	if !complete {
		if restoreErr == nil {
			restoreErr = errors.New("undo failed: workspace restoration was incomplete")
		}
		return "", restoreErr
	}
	if !a.undoBelongsToCurrentSession(a.latestUndo) {
		return "", errors.New("files were restored, but undo authority changed; recovery journal was preserved")
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
	if !a.undoBelongsToCurrentSession(a.latestUndo) {
		return "", errors.New("files were restored, but undo authority changed; recovery journal was preserved")
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
	if !a.undoBelongsToCurrentSession(a.latestUndo) {
		return "", errors.New("workspace and task state were recovered, but undo authority changed; recovery journal was preserved")
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
	return !a.undoReattachRequired && a.latestUndo != nil && a.undoLoadErr == nil && a.undoBelongsToCurrentSession(a.latestUndo)
}

// refreshDurableUndoLocked re-reads the journal while the project run lock is
// held so a cached in-process checkpoint cannot restore an older turn after a
// different process has published a newer one. The task is refreshed from the
// store first because the cached task snapshot may be older than that journal.
func (a *Agent) refreshDurableUndoLocked(workspace string) error {
	if a == nil || a.latestUndo == nil || !a.latestUndo.durable {
		return nil
	}
	binding := a.nativeTaskBinding()
	sessionID, generation := binding.sessionID, binding.generation
	if sessionID == "" {
		return errors.New("durable undo session is unavailable")
	}
	if a.latestUndo.taskStoreBound && (a.latestUndo.taskStore != binding.store || a.latestUndo.taskStoreEpoch != binding.storeGeneration) {
		return errors.New("cached undo belongs to a previous task store authority")
	}
	var task *taskstate.Task
	if store := binding.store; store != nil {
		var err error
		task, err = store.OwnershipSnapshot(sessionID)
		if errors.Is(err, taskstate.ErrNotFound) {
			task, err = nil, nil
		}
		if err != nil {
			return fmt.Errorf("inspect durable task for undo: %w", err)
		}
	}
	loaded, err := loadValidatedDurableUndo(workspace, sessionID, generation, task, undoTaskStoreAuthority{store: binding.store, epoch: binding.storeGeneration})
	if err != nil {
		return err
	}
	if loaded == nil {
		if err := prepareDurableUndoTask(workspace, binding.store, task); err != nil {
			return err
		}
		a.taskMu.Lock()
		a.task = task
		a.taskLoadErr = nil
		a.taskMu.Unlock()
		a.latestUndo = nil
		a.undoLoadErr = nil
		return nil
	}
	if task != nil {
		if err := prepareDurableUndoTask(workspace, binding.store, task); err != nil {
			return err
		}
		if err := validateDurableUndoTask(loaded, task); err != nil {
			return err
		}
	}
	a.taskMu.Lock()
	a.task = task
	a.taskLoadErr = nil
	a.taskMu.Unlock()
	if task == nil {
		if err := validateDurableUndoTask(loaded, nil); err != nil {
			return err
		}
	}
	a.latestUndo = loaded
	a.undoLoadErr = nil
	return nil
}

// prepareDurableUndoTask restores the same trust checks used during task
// attachment before an ownership-only snapshot is published into the agent.
func prepareDurableUndoTask(workspace string, store *taskstate.Store, task *taskstate.Task) error {
	if task == nil {
		return nil
	}
	changed := task.NormalizeLegacyCompletion()
	revalidated, err := revalidatePersistedTask(workspace, task)
	if err != nil {
		return fmt.Errorf("revalidate durable task for undo: %w", err)
	}
	if !changed && !revalidated {
		return nil
	}
	if store == nil {
		return errors.New("persist revalidated durable task for undo: task store is unavailable")
	}
	if err := store.Save(task); err != nil {
		return fmt.Errorf("persist revalidated durable task for undo: %w", err)
	}
	return nil
}

func (a *Agent) finishTurnUndo(res *Result, u *turnUndo, nativeWriteRan bool) {
	if !nativeWriteRan {
		return
	}
	a.taskMu.RLock()
	bound := a.taskRunBinding != nil
	a.taskMu.RUnlock()
	if bound {
		if err := a.checkTaskRun(nil); err != nil {
			// A revoked turn must leave its pending recovery journal intact.
			res.UndoError = err.Error()
			return
		}
	}
	if u.publishRejected && u.durable {
		// The checkpoint may contain a valid earlier publication, but it is
		// no longer safe to seal it from the live workspace after a later
		// publication was rejected. Reload the pending journal so the
		// in-memory undo candidate remains sealed and retains the exact
		// pre-rejection expectation. Process-only checkpoints instead use
		// SealPrepared below, retaining known expectations without adopting
		// the live bytes that caused the rejection.
		a.undoMu.Lock()
		defer a.undoMu.Unlock()
		if !a.undoBelongsToCurrentSession(u) {
			// The current session/store no longer owns this turn. Preserve its
			// journal for explicit recovery admission instead of deleting it.
			a.latestUndo = nil
			a.undoLoadErr = errTaskOwnershipChanged
			res.UndoError = errTaskOwnershipChanged.Error()
			return
		}
		sessionID, generation := a.taskSessionSnapshot()
		binding := a.nativeTaskBinding()
		loaded, err := loadValidatedDurableUndo(u.workspace, sessionID, generation, a.TaskSnapshot(), undoTaskStoreAuthority{store: binding.store, epoch: binding.storeGeneration})
		if err != nil {
			res.UndoError = fmt.Errorf("durable undo journal remains retryable after rejected publication: %w", err).Error()
			a.undoLoadErr = err
			a.latestUndo = nil
			return
		}
		if loaded == nil {
			res.UndoError = "durable undo journal disappeared after rejected publication"
			a.undoLoadErr = errors.New(res.UndoError)
			a.latestUndo = nil
			return
		}
		a.undoLoadErr = nil
		a.latestUndo = loaded
		res.UndoAvailable = true
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
		a.undoMu.Lock()
		defer a.undoMu.Unlock()
		if !a.undoBelongsToCurrentSession(u) {
			a.latestUndo = nil
			a.undoLoadErr = errTaskOwnershipChanged
			res.UndoError = errTaskOwnershipChanged.Error()
			return
		}
		if err := u.discardPending(); err != nil {
			res.UndoError = err.Error()
		}
		return
	}
	a.undoMu.Lock()
	if !a.undoBelongsToCurrentSession(u) {
		a.latestUndo = nil
		a.undoLoadErr = errTaskOwnershipChanged
		a.undoMu.Unlock()
		// Keep the original journal. Only a later explicit session attachment
		// may validate it and bind it to a new task store authority.
		res.UndoError = errTaskOwnershipChanged.Error()
		res.UndoAvailable = false
		return
	}
	if err := u.persistSealed(); err != nil {
		res.UndoError = fmt.Errorf("durable undo journal was not finalized; recovery remains retryable: %w", err).Error()
		if !u.durable {
			// A checkpoint that never reached a durable journal cannot be safely
			// restored from this process after another process advances the turn.
			// Fail closed instead of advertising a stale process-local candidate.
			a.latestUndo = nil
			a.undoLoadErr = err
			res.UndoAvailable = false
			a.undoMu.Unlock()
			return
		}
	}
	res.UndoAvailable = true
	a.latestUndo = u
	a.undoLoadErr = nil
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
	if u.sessionID != a.TaskSession || u.sessionGeneration != a.taskSessionGeneration {
		return false
	}
	return !u.taskStoreBound || (u.taskStore == a.TaskStore && u.taskStoreEpoch == a.taskStoreGeneration)
}

// validateDurableUndoTask binds a journal to the durable turn history that
// authorized it. A later read-only turn leaves the latest native-file undo
// useful, but a later mutating turn supersedes it. If the referenced turn is
// no longer present, the bounded history cannot prove that the record is
// current, so recovery fails closed.
func validateDurableUndoTask(u *turnUndo, task *taskstate.Task) error {
	if u == nil {
		return nil
	}
	if task == nil {
		return errors.New("durable undo task state is unavailable for owner validation")
	}
	if task.SessionID != u.sessionID {
		return fmt.Errorf("durable undo journal task session mismatch")
	}
	if strings.TrimSpace(u.taskID) == "" {
		return errors.New("durable undo journal lacks task owner identity")
	}
	if task.ID != u.taskID {
		return fmt.Errorf("durable undo journal task identity mismatch")
	}
	found := false
	for _, turn := range task.Turns {
		if turn.Sequence == u.turnSequence {
			if turn.IntentRevision != u.turnIntentRevision {
				return fmt.Errorf("durable undo journal turn intent mismatch")
			}
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

// loadValidatedDurableUndo validates the persisted turn owner before it may
// clean up a pending record that never reached a workspace rename. Keeping the
// raw loader read-only ensures a replacement task cannot consume another
// task's recovery journal merely by attaching to the same session ID.
func loadValidatedDurableUndo(workspace, sessionID string, generation uint64, task *taskstate.Task, authorities ...undoTaskStoreAuthority) (*turnUndo, error) {
	u, err := loadLatestDurableUndo(workspace, sessionID, generation, authorities...)
	if err != nil {
		return nil, err
	}
	if err := validateDurableUndoTask(u, task); err != nil {
		return nil, err
	}
	if u == nil || !u.pendingUnpublished {
		return u, nil
	}
	if err := removeUndoJournal(workspace, sessionID, true); err != nil {
		return nil, err
	}
	u, err = loadLatestDurableUndo(workspace, sessionID, generation, authorities...)
	if err != nil {
		return nil, err
	}
	if err := validateDurableUndoTask(u, task); err != nil {
		return nil, err
	}
	return u, nil
}
