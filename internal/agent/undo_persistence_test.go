package agent

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/checkpoint"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
	workspacepkg "github.com/saiaathish/picogent/internal/workspace"
)

func TestUndoInvalidatesDurableWorkspaceEvidenceAndReopensDoneTask(t *testing.T) {
	a, store, task := newDurableUndoFixture(t, taskstate.StatusDone)
	originalFiles := append([]string(nil), task.ChangedFiles...)
	originalTurns := append([]taskstate.TurnRecord(nil), task.Turns...)

	msg, err := a.UndoLastTurn()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "restored fixed.txt") {
		t.Fatalf("undo message = %q", msg)
	}
	assertUndoFileContent(t, filepath.Join(a.ConfigSnapshot().Workspace, "fixed.txt"), "before\n")

	got := a.TaskSnapshot()
	if got == nil || got.Status != taskstate.StatusVerifying || !got.NeedsVerification() {
		t.Fatalf("in-memory task = %#v, want verifying/unverified", got)
	}
	if got.VerifiedChangeSeq != -1 || got.Revision != task.Revision+1 {
		t.Fatalf("invalidated task generation = revision %d verified %d", got.Revision, got.VerifiedChangeSeq)
	}
	if !reflect.DeepEqual(got.ChangedFiles, originalFiles) || got.ChangeSeq != task.ChangeSeq || !reflect.DeepEqual(got.Turns, originalTurns) {
		t.Fatalf("undo changed durable history = files %#v seq %d turns %#v", got.ChangedFiles, got.ChangeSeq, got.Turns)
	}
	latest := got.Verification[len(got.Verification)-1]
	if latest.Passed || !strings.HasPrefix(latest.Summary, "verify INCONCLUSIVE") {
		t.Fatalf("invalidated verification = %#v", latest)
	}
	if a.UndoAvailable() {
		t.Fatal("restored checkpoint remained available")
	}

	persisted, err := store.Load(task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != taskstate.StatusVerifying || persisted.Revision != got.Revision || persisted.Verification[len(persisted.Verification)-1].Passed {
		t.Fatalf("persisted invalidation = %#v", persisted)
	}
}

func TestUndoRecoversAfterCASConflict(t *testing.T) {
	a, store, task := newDurableUndoFixture(t, taskstate.StatusWorking)
	other, err := store.Load(task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	other.NoteAttempt()
	other.RecordChanged("concurrent.txt")
	other.Turns[0].Hypothesis = "concurrent history marker"
	if err := store.Save(other); err != nil {
		t.Fatal(err)
	}

	if _, err = a.UndoLastTurn(); err != nil {
		t.Fatal(err)
	}
	assertUndoFileContent(t, filepath.Join(a.ConfigSnapshot().Workspace, "fixed.txt"), "before\n")
	if a.UndoAvailable() {
		t.Fatal("checkpoint remained available after files were restored")
	}
	got := a.TaskSnapshot()
	if got == nil || got.Revision != other.Revision+1 || got.Attempts != other.Attempts || got.ChangeSeq != other.ChangeSeq || !reflect.DeepEqual(got.ChangedFiles, other.ChangedFiles) || !reflect.DeepEqual(got.Turns, other.Turns) || got.VerifiedChangeSeq != -1 || got.Verification[len(got.Verification)-1].Passed {
		t.Fatalf("recovered CAS task = %#v", got)
	}
	persisted, err := store.Load(task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Revision != other.Revision+1 || persisted.Attempts != other.Attempts || persisted.ChangeSeq != other.ChangeSeq || !reflect.DeepEqual(persisted.ChangedFiles, other.ChangedFiles) || !reflect.DeepEqual(persisted.Turns, other.Turns) || persisted.VerifiedChangeSeq != -1 || persisted.Verification[len(persisted.Verification)-1].Passed {
		t.Fatalf("recovered persisted task = %#v", persisted)
	}
}

func TestUndoRetainsCheckpointUntilDurableRecovery(t *testing.T) {
	a, store, task := newDurableUndoFixture(t, taskstate.StatusWorking)
	path, err := store.Path(task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	_, err = a.UndoLastTurn()
	if err == nil || !strings.Contains(err.Error(), "files restored but durable task state was not saved") || !strings.Contains(err.Error(), "retry /undo") || !errors.Is(err, taskstate.ErrRevisionConflict) || !errors.Is(err, taskstate.ErrNotFound) {
		t.Fatalf("unrecoverable undo CAS failure = %v", err)
	}
	assertUndoFileContent(t, filepath.Join(a.ConfigSnapshot().Workspace, "fixed.txt"), "before\n")
	if !a.UndoAvailable() {
		t.Fatal("checkpoint was discarded before durable recovery completed")
	}
	if got := a.TaskSnapshot(); got == nil || got.Revision != task.Revision || got.VerifiedChangeSeq != task.ChangeSeq || !got.Verification[len(got.Verification)-1].Passed {
		t.Fatalf("unrecoverable CAS published invalidated task = %#v", got)
	}

	// Recreate the missing durable record, then retry. The workspace is already
	// restored, so this must retry only the task mutation.
	recreated := cloneTask(task)
	recreated.Revision = 0
	if err := store.Save(recreated); err != nil {
		t.Fatal(err)
	}
	if _, err := a.UndoLastTurn(); err != nil {
		t.Fatalf("undo retry = %v", err)
	}
	if a.UndoAvailable() {
		t.Fatal("checkpoint remained available after durable recovery")
	}
	got := a.TaskSnapshot()
	if got == nil || got.Revision != recreated.Revision+1 || got.VerifiedChangeSeq != -1 || got.Verification[len(got.Verification)-1].Passed {
		t.Fatalf("retried invalidation = %#v", got)
	}
}

func TestUndoRebasesStoreNormalizationBeforeInvalidation(t *testing.T) {
	a, store, task := newDurableUndoFixture(t, taskstate.StatusDone)
	loaded, err := store.Load(task.SessionID)
	if err != nil || loaded.Status != taskstate.StatusWorking {
		t.Fatalf("legacy completion normalization = %#v, err=%v", loaded, err)
	}

	if _, err := a.UndoLastTurn(); err != nil {
		t.Fatal(err)
	}
	assertUndoFileContent(t, filepath.Join(a.ConfigSnapshot().Workspace, "fixed.txt"), "before\n")
	got := a.TaskSnapshot()
	if got == nil || got.Status != taskstate.StatusWorking || got.VerifiedChangeSeq != -1 || !got.NeedsVerification() {
		t.Fatalf("rebased undo task = %#v", got)
	}
	if got.Revision != loaded.Revision+1 || got.ChangeSeq != task.ChangeSeq || !reflect.DeepEqual(got.ChangedFiles, task.ChangedFiles) {
		t.Fatalf("rebased undo history = revision %d seq %d files %#v", got.Revision, got.ChangeSeq, got.ChangedFiles)
	}
}

func TestUndoConflictDoesNotInvalidateDurableEvidence(t *testing.T) {
	a, store, task := newDurableUndoFixture(t, taskstate.StatusWorking)
	path := filepath.Join(a.ConfigSnapshot().Workspace, "fixed.txt")
	if err := os.WriteFile(path, []byte("newer user edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := a.UndoLastTurn()
	if err == nil || !strings.Contains(err.Error(), "newer changes") {
		t.Fatalf("undo conflict = %v", err)
	}
	assertUndoFileContent(t, path, "newer user edit\n")
	if !a.UndoAvailable() {
		t.Fatal("conflicted checkpoint was discarded")
	}
	got := a.TaskSnapshot()
	if got == nil || got.Revision != task.Revision || got.VerifiedChangeSeq != task.ChangeSeq || !got.Verification[len(got.Verification)-1].Passed {
		t.Fatalf("conflict changed durable evidence = %#v", got)
	}
	persisted, err := store.Load(task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Revision != task.Revision || !persisted.Verification[len(persisted.Verification)-1].Passed {
		t.Fatalf("conflict changed persisted evidence = %#v", persisted)
	}
}

func TestCachedUndoRefreshesAfterNewerDurableTurn(t *testing.T) {
	a, store, task := newDurableUndoFixture(t, taskstate.StatusWorking)
	oldUndo := a.latestUndo
	oldUndo.turnSequence = task.Turns[0].Sequence
	oldUndo.durable = true
	oldUndo.journalSlot = undoJournalSealed
	oldUndo.workspaceInstance = testUndoWorkspaceInstance(t, oldUndo.workspace)
	oldRecord, err := oldUndo.checkpoint.Export()
	if err != nil {
		t.Fatal(err)
	}
	if err := oldUndo.saveJournal(oldRecord, undoJournalSealed); err != nil {
		t.Fatal(err)
	}

	newPath := filepath.Join(a.ConfigSnapshot().Workspace, "new.txt")
	newCheckpoint, err := checkpoint.Capture(a.ConfigSnapshot().Workspace, []string{"new.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte("newer agent edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := newCheckpoint.Seal(); err != nil {
		t.Fatal(err)
	}
	current, err := store.Load(task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	sequence, ok := current.BeginTurn(taskstate.TurnRouteImplement)
	if !ok {
		t.Fatal("newer turn did not start")
	}
	current.RecordChanged("new.txt")
	if !current.FinishTurn(sequence, taskstate.TurnRouteImplement, "edit the newer file", "UNVERIFIED", taskstate.StopNone, 1, 1) {
		t.Fatal("newer turn did not finish")
	}
	if err := store.Save(current); err != nil {
		t.Fatal(err)
	}
	newUndo := &turnUndo{
		workspace:          a.ConfigSnapshot().Workspace,
		checkpoint:         newCheckpoint,
		sessionID:          task.SessionID,
		sessionGeneration:  oldUndo.sessionGeneration,
		turnSequence:       sequence,
		taskID:             current.ID,
		turnIntentRevision: current.LastTurn().IntentRevision,
		workspaceInstance:  oldUndo.workspaceInstance,
		durable:            true,
		journalSlot:        undoJournalSealed,
	}
	newRecord, err := newCheckpoint.Export()
	if err != nil {
		t.Fatal(err)
	}
	if err := newUndo.saveJournal(newRecord, undoJournalSealed); err != nil {
		t.Fatal(err)
	}

	message, err := a.UndoLastTurn()
	if err != nil || !strings.Contains(message, "removed new.txt") {
		t.Fatalf("cached undo after newer durable turn = (%q, %v)", message, err)
	}
	assertUndoFileContent(t, filepath.Join(a.ConfigSnapshot().Workspace, "fixed.txt"), "after\n")
	if _, err := os.Stat(newPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("newer turn file was not undone: %v", err)
	}
}

func TestUndoRefreshErrorClearsAfterJournalRecovery(t *testing.T) {
	a, _, task := newDurableUndoFixture(t, taskstate.StatusWorking)
	undo := a.latestUndo
	undo.turnSequence = task.Turns[0].Sequence
	undo.durable = true
	undo.journalSlot = undoJournalSealed
	undo.workspaceInstance = testUndoWorkspaceInstance(t, undo.workspace)
	record, err := undo.checkpoint.Export()
	if err != nil {
		t.Fatal(err)
	}
	sealedPath, _, err := undoJournalPaths(a.ConfigSnapshot().Workspace, task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := undo.saveJournal(record, undoJournalSealed); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sealedPath, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := a.UndoLastTurn(); err == nil || !strings.Contains(err.Error(), "undo is unavailable") {
		t.Fatalf("malformed refresh error = %v", err)
	}
	if a.UndoAvailable() {
		t.Fatal("undo remained available after refresh failure")
	}

	if err := undo.saveJournal(record, undoJournalSealed); err != nil {
		t.Fatal(err)
	}
	message, err := a.UndoLastTurn()
	if err != nil || !strings.Contains(message, "restored fixed.txt") {
		t.Fatalf("undo after journal recovery = (%q, %v)", message, err)
	}
}

func TestUndoPreservesCompletedRestoreWarning(t *testing.T) {
	warning := errors.New("checkpoint restored but temporary file cleanup failed")
	result := checkpoint.RestoreResult{Restored: []string{"fixed.txt"}, Complete: true}

	msg, complete, err := formatUndoRestore(result, warning)
	if !complete {
		t.Fatal("completed restore was treated as incomplete")
	}
	if !strings.Contains(msg, "restored fixed.txt") {
		t.Fatalf("restore message = %q", msg)
	}
	if err == nil || !strings.Contains(err.Error(), "cleanup failed") || !errors.Is(err, warning) {
		t.Fatalf("restore warning = %v", err)
	}
}

func TestChangingTaskSessionDiscardsUndoCheckpoint(t *testing.T) {
	a, _, _ := newDurableUndoFixture(t, taskstate.StatusWorking)

	if !a.UndoAvailable() {
		t.Fatal("fixture did not provide an undo checkpoint")
	}
	if err := a.SetTaskSession("new-session"); err != nil {
		t.Fatal(err)
	}
	if a.UndoAvailable() {
		t.Fatal("session change retained the old undo checkpoint")
	}
	if msg, err := a.UndoLastTurn(); err != nil || msg != "nothing to undo" {
		t.Fatalf("undo after session change = (%q, %v)", msg, err)
	}
	assertUndoFileContent(t, filepath.Join(a.ConfigSnapshot().Workspace, "fixed.txt"), "after\n")
}

func TestLateTurnCannotRepublishUndoAfterSessionChange(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "fixed.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cp, err := checkpoint.Capture(root, []string{"fixed.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Workspace = root
	cfg.Provider = config.ProviderOllama
	a := New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: root}), perm.New(config.ModeFast, root, nil))
	a.TaskSession = "old-session"
	late := &turnUndo{workspace: root, checkpoint: cp, sessionID: "old-session"}
	if err := a.SetTaskSession("new-session"); err != nil {
		t.Fatal(err)
	}

	var result Result
	a.finishTurnUndo(&result, late, true)
	if result.UndoAvailable || a.UndoAvailable() {
		t.Fatal("late old-session turn republished an undo checkpoint")
	}
}

func TestLegacyUndoJournalParsesButCannotRecoverWithoutOwner(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cp, err := checkpoint.Capture(root, []string{"note.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cp.Seal(); err != nil {
		t.Fatal(err)
	}
	record, err := cp.Export()
	if err != nil {
		t.Fatal(err)
	}
	const sessionID = "legacy-undo"
	identity, err := undoWorkspaceIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(undoJournal{
		Version:      undoJournalLegacyVersion,
		State:        undoJournalSealed,
		Workspace:    identity,
		SessionID:    sessionID,
		TurnSequence: 1,
		Checkpoint:   record,
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, ".picogent", "undo")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(dir, sessionID+".json")
	if err := os.WriteFile(journalPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	parsed, err := loadUndoJournal(root, sessionID, false)
	if err != nil || parsed == nil || parsed.Version != undoJournalLegacyVersion {
		t.Fatalf("legacy journal parse = (%#v, %v)", parsed, err)
	}
	if loaded, err := loadLatestDurableUndo(root, sessionID, 1); err == nil || loaded != nil || !strings.Contains(err.Error(), "owner identity") {
		t.Fatalf("legacy journal recovery = (%#v, %v)", loaded, err)
	}
	store := taskstate.NewStore(t.TempDir())
	task, err := taskstate.New(sessionID, "continue the existing task", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := task.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Workspace = root
	cfg.Mode = config.ModeFast
	cfg.Provider = config.ProviderOllama
	a := New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: root}), perm.New(config.ModeFast, root, nil))
	a.SetTaskStore(store)
	if err := a.SetTaskSession(sessionID); err != nil {
		t.Fatalf("legacy undo journal blocked task attachment: %v", err)
	}
	if attached := a.TaskSnapshot(); attached == nil || attached.ID != task.ID {
		t.Fatalf("attached task = %#v, want %#v", attached, task)
	}
	if a.UndoAvailable() {
		t.Fatal("legacy journal was advertised as recoverable")
	}
	if _, err := a.UndoLastTurn(); err == nil || !strings.Contains(err.Error(), "owner identity") {
		t.Fatalf("legacy undo attempt = %v, want an owner-identity refusal", err)
	}
	assertUndoFileContent(t, path, "after\n")
	journalAfter, err := os.ReadFile(journalPath)
	if err != nil || !reflect.DeepEqual(data, journalAfter) {
		t.Fatalf("legacy journal was not preserved: err=%v unchanged=%v", err, reflect.DeepEqual(data, journalAfter))
	}
}

func TestDurableUndoRequiresMatchingTurnIntent(t *testing.T) {
	a, _, task := newDurableUndoFixture(t, taskstate.StatusWorking)
	undo := a.latestUndo
	undo.turnSequence = task.LastTurn().Sequence
	undo.turnIntentRevision++
	if err := validateDurableUndoTask(undo, task); err == nil || !strings.Contains(err.Error(), "turn intent mismatch") {
		t.Fatalf("undo with a different turn intent = %v", err)
	}
}

func TestVersionThreeUndoJournalRequiresIntentRevisionField(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cp, err := checkpoint.Capture(root, []string{"note.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cp.PrepareExpected(path, []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	record, err := cp.Export()
	if err != nil {
		t.Fatal(err)
	}
	const sessionID = "missing-intent-revision"
	identity, err := undoWorkspaceIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	instance := testUndoWorkspaceInstance(t, root)
	journal := undoJournal{
		Version: undoJournalVersion, State: undoJournalSealed, Workspace: identity, WorkspaceInstance: instance,
		SessionID: sessionID, TurnSequence: 1, TaskID: "intent-owner",
		IntentRevision: 0, Checkpoint: record,
	}
	if err := saveUndoJournal(root, sessionID, journal, false); err != nil {
		t.Fatal(err)
	}
	if _, err := loadUndoJournal(root, sessionID, false); err != nil {
		t.Fatalf("valid zero intent revision was rejected: %v", err)
	}
	sealedPath, _, err := undoJournalPaths(root, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(sealedPath)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "turn_intent_revision")
	data, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sealedPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadUndoJournal(root, sessionID, false); err == nil || !strings.Contains(err.Error(), "missing turn intent revision") {
		t.Fatalf("v3 journal without intent revision = %v", err)
	}
}

func TestValidateUndoJournalUsesWorkspaceFilesystemIdentity(t *testing.T) {
	workspace, alias := workspacePathAlias(t)
	path := filepath.Join(workspace, "note.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cp, err := checkpoint.Capture(workspace, []string{"note.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cp.PrepareExpected(path, []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	record, err := cp.Export()
	if err != nil {
		t.Fatal(err)
	}
	identity, err := undoWorkspaceIdentity(workspace)
	if err != nil {
		t.Fatal(err)
	}
	const sessionID = "workspace-alias-validation"
	journal := undoJournal{
		Version: undoJournalVersion, State: undoJournalSealed, Workspace: identity, WorkspaceInstance: testUndoWorkspaceInstance(t, workspace),
		SessionID: sessionID, TurnSequence: 1, TaskID: "workspace-alias-owner",
		Checkpoint: record,
	}
	if err := validateUndoJournal(journal, alias, sessionID); err != nil {
		t.Fatalf("journal rejected an alias of its workspace: %v", err)
	}
	if err := validateUndoJournal(journal, t.TempDir(), sessionID); err == nil || !strings.Contains(err.Error(), "workspace mismatch") {
		t.Fatalf("journal for a distinct workspace = %v", err)
	}
}

func TestRemoveUndoJournalAcceptsWorkspaceAlias(t *testing.T) {
	root, alias := workspacePathAlias(t)
	instance := testUndoWorkspaceInstance(t, root)
	const sessionID = "workspace-alias-cleanup"
	_, pendingPath, err := undoJournalPaths(root, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(pendingPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pendingPath, []byte("pending"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeUndoJournal(alias, sessionID, true, instance); err != nil {
		t.Fatalf("remove journal through workspace alias: %v", err)
	}
	if _, err := os.Stat(pendingPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pending journal remains after alias cleanup: %v", err)
	}
}

func TestRemoveUndoJournalDistinguishesMissingLeafFromParent(t *testing.T) {
	root := t.TempDir()
	instance := testUndoWorkspaceInstance(t, root)
	const missingParentSession = "missing-undo-parent"
	err := removeUndoJournal(root, missingParentSession, true, instance)
	if !errors.Is(err, workspacepkg.ErrWorkspaceParentNotExist) {
		t.Fatalf("cleanup with missing journal parent = %v, want parent-missing error", err)
	}

	undoDir := filepath.Join(root, ".picogent", "undo")
	if err := os.MkdirAll(undoDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := removeUndoJournal(root, "missing-undo-leaf", true, instance); err != nil {
		t.Fatalf("cleanup with only the journal leaf missing: %v", err)
	}
}

func TestDurableUndoRejectsChangedWorkspaceMarker(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cp, err := checkpoint.Capture(root, []string{"note.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cp.Seal(); err != nil {
		t.Fatal(err)
	}
	instance := testUndoWorkspaceInstance(t, root)
	undo := &turnUndo{
		workspace: root, checkpoint: cp, sessionID: "marker-guard", turnSequence: 1,
		workspaceInstance: instance,
	}
	markerPath := filepath.Join(root, filepath.FromSlash(undoWorkspaceInstanceMarker))
	token := "0" + instance.Token[1:]
	if token == instance.Token {
		token = "1" + instance.Token[1:]
	}
	if err := os.WriteFile(markerPath, []byte(token+"\n"), 0o600); err != nil {
		t.Fatalf("change workspace marker: %v", err)
	}

	if _, complete, err := undo.restore(); err == nil || complete || !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("restore with changed marker = complete:%v err:%v", complete, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "agent\n" {
		t.Fatalf("file changed after marker rejection: %q, %v", got, err)
	}

	if err := os.WriteFile(markerPath, []byte(instance.Token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, complete, err := undo.restore(); err != nil || !complete {
		t.Fatalf("restore after marker repair = complete:%v err:%v", complete, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "before\n" {
		t.Fatalf("retry did not restore checkpoint: %q, %v", got, err)
	}
}

func TestCopiedWorkspaceCannotReuseUndoJournal(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cp, err := checkpoint.Capture(root, []string{"note.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cp.PrepareExpected(path, []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	record, err := cp.Export()
	if err != nil {
		t.Fatal(err)
	}
	instance := testUndoWorkspaceInstance(t, root)
	identity, err := undoWorkspaceIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	const sessionID = "copied-workspace"
	journal := undoJournal{
		Version: undoJournalVersion, State: undoJournalSealed, Workspace: identity,
		WorkspaceInstance: instance, SessionID: sessionID, TurnSequence: 1,
		TaskID: "copied-workspace-owner", Checkpoint: record,
	}
	if err := saveUndoJournal(root, sessionID, journal, false); err != nil {
		t.Fatal(err)
	}
	sealedPath, _, err := undoJournalPaths(root, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	journalData, err := os.ReadFile(sealedPath)
	if err != nil {
		t.Fatal(err)
	}
	markerPath := filepath.Join(root, filepath.FromSlash(undoWorkspaceInstanceMarker))
	markerData, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatal(err)
	}

	parked := filepath.Join(parent, "parked-workspace")
	if err := os.Rename(root, parked); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	markerPath = filepath.Join(root, filepath.FromSlash(undoWorkspaceInstanceMarker))
	if err := os.MkdirAll(filepath.Dir(markerPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(markerPath, markerData, 0o600); err != nil {
		t.Fatal(err)
	}
	sealedPath, _, err = undoJournalPaths(root, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(sealedPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sealedPath, journalData, 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := loadUndoJournal(root, sessionID, false)
	if err == nil || loaded != nil || !strings.Contains(err.Error(), "workspace instance") {
		t.Fatalf("copied journal load = (%#v, %v), want workspace-instance rejection", loaded, err)
	}
	if got, err := os.ReadFile(filepath.Join(root, "note.txt")); err != nil || string(got) != "after\n" {
		t.Fatalf("replacement workspace file = %q, err=%v", got, err)
	}
	if after, err := os.ReadFile(sealedPath); err != nil || !reflect.DeepEqual(after, journalData) {
		t.Fatalf("rejected journal changed: err=%v unchanged=%v", err, reflect.DeepEqual(after, journalData))
	}
}

func TestPendingJournalCleanupRefusesCopiedWorkspace(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	instance := testUndoWorkspaceInstance(t, root)
	const sessionID = "copied-pending-cleanup"
	_, pendingPath, err := undoJournalPaths(root, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(pendingPath), 0o700); err != nil {
		t.Fatal(err)
	}
	pendingData := []byte("replacement pending journal\n")
	if err := os.WriteFile(pendingPath, pendingData, 0o600); err != nil {
		t.Fatal(err)
	}
	markerPath := filepath.Join(root, filepath.FromSlash(undoWorkspaceInstanceMarker))
	markerData, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatal(err)
	}

	parked := filepath.Join(parent, "parked-workspace")
	if err := os.Rename(root, parked); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	markerPath = filepath.Join(root, filepath.FromSlash(undoWorkspaceInstanceMarker))
	if err := os.MkdirAll(filepath.Dir(markerPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(markerPath, markerData, 0o600); err != nil {
		t.Fatal(err)
	}
	_, pendingPath, err = undoJournalPaths(root, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(pendingPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pendingPath, pendingData, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := removeUndoJournal(root, sessionID, true, instance); err == nil || !strings.Contains(err.Error(), "workspace instance") {
		t.Fatalf("copied pending journal cleanup = %v, want workspace-instance rejection", err)
	}
	if after, err := os.ReadFile(pendingPath); err != nil || !reflect.DeepEqual(after, pendingData) {
		t.Fatalf("replacement pending journal changed: err=%v unchanged=%v", err, reflect.DeepEqual(after, pendingData))
	}
}

func TestVersionTwoUndoJournalCannotBeBoundRetroactively(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cp, err := checkpoint.Capture(root, []string{"note.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cp.PrepareExpected(path, []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	record, err := cp.Export()
	if err != nil {
		t.Fatal(err)
	}
	identity, err := undoWorkspaceIdentity(root)
	if err != nil {
		t.Fatal(err)
	}
	const sessionID = "legacy-path-only"
	journal := undoJournal{
		Version: undoJournalTaskOwnerVersion, State: undoJournalSealed, Workspace: identity,
		SessionID: sessionID, TurnSequence: 1, TaskID: "legacy-owner",
		IntentRevision: 0, Checkpoint: record,
	}
	data, err := json.Marshal(journal)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "workspace_instance")
	data, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	sealedPath, _, err := undoJournalPaths(root, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(sealedPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sealedPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	_ = testUndoWorkspaceInstance(t, root) // A current marker cannot retroactively bind a path-only journal.

	loaded, err := loadLatestDurableUndo(root, sessionID, 1)
	if !errors.Is(err, errLegacyUndoWorkspaceBinding) || loaded != nil {
		t.Fatalf("v2 journal recovery = (%#v, %v), want fail-closed compatibility error", loaded, err)
	}
	if after, err := os.ReadFile(sealedPath); err != nil || !reflect.DeepEqual(after, data) {
		t.Fatalf("legacy journal changed: err=%v unchanged=%v", err, reflect.DeepEqual(after, data))
	}
}

func TestUnpublishedPendingUndoPreservesJournalBeforeOwnerValidation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const sessionID = "pending-owner-validation"
	storeA := taskstate.NewStore(t.TempDir())
	storeB := taskstate.NewStore(t.TempDir())
	owner, err := taskstate.New(sessionID, "original outcome", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	sequence, ok := owner.BeginTurn(taskstate.TurnRouteImplement)
	if !ok || !owner.FinishTurn(sequence, taskstate.TurnRouteImplement, "write note", "UNVERIFIED", taskstate.StopNone, 1, 1) {
		t.Fatal("owner turn did not finish")
	}
	if err := storeA.Save(owner); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Workspace = root
	cfg.Mode = config.ModeFast
	cfg.Provider = config.ProviderOllama
	a := New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: root}), perm.New(config.ModeFast, root, nil))
	a.SetTaskStore(storeA)
	if err := a.SetTaskSession(sessionID); err != nil {
		t.Fatal(err)
	}
	cp, err := checkpoint.Capture(root, []string{"note.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cp.PrepareExpected(path, []byte("agent edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	record, err := cp.Export()
	if err != nil {
		t.Fatal(err)
	}
	undo := &turnUndo{
		workspace: root, checkpoint: cp, sessionID: sessionID,
		sessionGeneration: a.taskSessionGeneration, turnSequence: sequence,
		taskID: owner.ID, turnIntentRevision: owner.LastTurn().IntentRevision,
		workspaceInstance: testUndoWorkspaceInstance(t, root),
	}
	undo.bindTaskStore(storeA, a.taskStoreGeneration)
	if err := undo.saveJournal(record, undoJournalPending); err != nil {
		t.Fatal(err)
	}
	_, pendingPath, err := undoJournalPaths(root, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	pendingBefore, err := os.ReadFile(pendingPath)
	if err != nil {
		t.Fatal(err)
	}

	replacement, err := taskstate.New(sessionID, "replacement outcome", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := replacement.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	if replacementSequence, ok := replacement.BeginTurn(taskstate.TurnRouteImplement); !ok || replacementSequence != sequence {
		t.Fatalf("replacement turn sequence = %d, want %d", replacementSequence, sequence)
	}
	if err := storeB.Save(replacement); err != nil {
		t.Fatal(err)
	}
	replacementPath, err := storeB.Path(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	replacementBefore, err := os.ReadFile(replacementPath)
	if err != nil {
		t.Fatal(err)
	}

	a.SetTaskStore(storeB)
	if err := a.SetTaskSession(sessionID); err == nil || !strings.Contains(err.Error(), "task identity mismatch") {
		t.Fatalf("reattachment to replacement task = %v", err)
	}
	assertUndoFileContent(t, path, "before\n")
	pendingAfter, err := os.ReadFile(pendingPath)
	if err != nil || !reflect.DeepEqual(pendingBefore, pendingAfter) {
		t.Fatalf("rejected reattachment changed pending journal: err=%v unchanged=%v", err, reflect.DeepEqual(pendingBefore, pendingAfter))
	}
	replacementAfter, err := os.ReadFile(replacementPath)
	if err != nil || !reflect.DeepEqual(replacementBefore, replacementAfter) {
		t.Fatalf("rejected reattachment changed replacement task: err=%v unchanged=%v", err, reflect.DeepEqual(replacementBefore, replacementAfter))
	}
	if _, err := a.UndoLastTurn(); err == nil || !strings.Contains(err.Error(), "reattach") {
		t.Fatalf("undo after rejected reattachment = %v", err)
	}
}

func TestPendingUndoCleanupRefreshesStaleOwnerSnapshot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const sessionID = "pending-stale-owner-refresh"
	owner, err := taskstate.New(sessionID, "original outcome", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	sequence, ok := owner.BeginTurn(taskstate.TurnRouteImplement)
	if !ok || !owner.FinishTurn(sequence, taskstate.TurnRouteImplement, "write note", "UNVERIFIED", taskstate.StopNone, 1, 1) {
		t.Fatal("owner turn did not finish")
	}

	cp, err := checkpoint.Capture(root, []string{"note.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cp.PrepareExpected(path, []byte("agent edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	record, err := cp.Export()
	if err != nil {
		t.Fatal(err)
	}
	undo := &turnUndo{
		workspace: root, checkpoint: cp, sessionID: sessionID,
		turnSequence: sequence, taskID: owner.ID,
		turnIntentRevision: owner.LastTurn().IntentRevision,
		workspaceInstance:  testUndoWorkspaceInstance(t, root),
	}
	if err := undo.saveJournal(record, undoJournalPending); err != nil {
		t.Fatal(err)
	}
	_, pendingPath, err := undoJournalPaths(root, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	pendingBefore, err := os.ReadFile(pendingPath)
	if err != nil {
		t.Fatal(err)
	}

	currentStore := taskstate.NewStore(t.TempDir())
	replacement, err := taskstate.New(sessionID, "replacement outcome", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := replacement.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	if replacementSequence, ok := replacement.BeginTurn(taskstate.TurnRouteImplement); !ok || replacementSequence != sequence {
		t.Fatalf("replacement turn sequence = %d, want %d", replacementSequence, sequence)
	}
	if err := currentStore.Save(replacement); err != nil {
		t.Fatal(err)
	}

	if _, err := loadValidatedDurableUndo(root, sessionID, 1, owner, undoTaskStoreAuthority{store: currentStore}); err == nil || !strings.Contains(err.Error(), "task identity mismatch") {
		t.Fatalf("stale owner snapshot authorized pending recovery cleanup: %v", err)
	}
	pendingAfter, err := os.ReadFile(pendingPath)
	if err != nil || !reflect.DeepEqual(pendingBefore, pendingAfter) {
		t.Fatalf("replacement owner's pending journal changed: err=%v unchanged=%v", err, reflect.DeepEqual(pendingBefore, pendingAfter))
	}
}

func TestUndoRefreshValidatesOwnerBeforeNormalizingReplacementTask(t *testing.T) {
	a, store, owner := newDurableUndoFixture(t, taskstate.StatusWorking)
	u := a.latestUndo
	u.turnSequence = owner.LastTurn().Sequence
	u.durable = true
	u.journalSlot = undoJournalSealed
	u.workspaceInstance = testUndoWorkspaceInstance(t, u.workspace)
	record, err := u.checkpoint.Export()
	if err != nil {
		t.Fatal(err)
	}
	if err := u.saveJournal(record, undoJournalSealed); err != nil {
		t.Fatal(err)
	}

	replacement, err := taskstate.New(owner.SessionID, "replacement task", nil)
	if err != nil {
		t.Fatal(err)
	}
	replacement.Status = taskstate.StatusDone // a legacy terminal marker without current proof
	if err := replacement.Validate(); err != nil {
		t.Fatalf("replacement fixture is invalid: %v", err)
	}
	data, err := json.Marshal(replacement)
	if err != nil {
		t.Fatal(err)
	}
	replacementPath, err := store.Path(owner.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(replacementPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	replacementBefore, err := os.ReadFile(replacementPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.UndoLastTurn(); err == nil || !strings.Contains(err.Error(), "task identity mismatch") {
		t.Fatalf("undo with replacement durable task = %v", err)
	}
	replacementAfter, err := os.ReadFile(replacementPath)
	if err != nil || !reflect.DeepEqual(replacementBefore, replacementAfter) {
		t.Fatalf("rejected undo normalized replacement task: err=%v unchanged=%v", err, reflect.DeepEqual(replacementBefore, replacementAfter))
	}
	assertUndoFileContent(t, filepath.Join(a.ConfigSnapshot().Workspace, "fixed.txt"), "after\n")
}

func TestUndoRefreshWithoutJournalNormalizesAndRevalidatesTask(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     taskstate.Status
		legacyDone bool
		wantStatus taskstate.Status
	}{
		{name: "legacy done marker", status: taskstate.StatusWorking, legacyDone: true, wantStatus: taskstate.StatusWorking},
		{name: "stale workspace proof", status: taskstate.StatusDone, wantStatus: taskstate.StatusVerifying},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, store, owner := newDurableUndoFixture(t, tc.status)
			u := a.latestUndo
			u.turnSequence = owner.LastTurn().Sequence
			u.durable = true
			u.journalSlot = undoJournalSealed
			u.workspaceInstance = testUndoWorkspaceInstance(t, u.workspace)
			record, err := u.checkpoint.Export()
			if err != nil {
				t.Fatal(err)
			}
			if err := u.saveJournal(record, undoJournalSealed); err != nil {
				t.Fatal(err)
			}

			if tc.legacyDone {
				untrusted := cloneTask(owner)
				untrusted.Status = taskstate.StatusDone
				untrusted.Verification = nil
				untrusted.VerifiedChangeSeq = -1
				data, err := json.Marshal(untrusted)
				if err != nil {
					t.Fatal(err)
				}
				path, err := store.Path(owner.SessionID)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
			} else {
				path := filepath.Join(a.ConfigSnapshot().Workspace, "fixed.txt")
				if err := os.WriteFile(path, []byte("changed after verification\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			journalPath := filepath.Join(a.ConfigSnapshot().Workspace, ".picogent", "undo", owner.SessionID+".json")
			if err := os.Remove(journalPath); err != nil {
				t.Fatal(err)
			}
			message, err := a.UndoLastTurn()
			if err != nil || message != "nothing to undo" {
				t.Fatalf("undo without journal = (%q, %v)", message, err)
			}
			got := a.TaskSnapshot()
			if got == nil || got.Status != tc.wantStatus {
				t.Fatalf("refreshed task = %#v, want status %q", got, tc.wantStatus)
			}
			if got.Status == taskstate.StatusDone {
				t.Fatal("untrusted completion became active after missing-journal refresh")
			}
			persisted, err := store.OwnershipSnapshot(owner.SessionID)
			if err != nil || persisted == nil || persisted.Status != tc.wantStatus {
				t.Fatalf("persisted refreshed task = %#v, err=%v, want status %q", persisted, err, tc.wantStatus)
			}
			if !tc.legacyDone {
				latest := got.Verification[len(got.Verification)-1]
				if latest.Passed || !strings.HasPrefix(latest.Summary, "verify INCONCLUSIVE") {
					t.Fatalf("stale proof remained trusted: %#v", latest)
				}
			}
			if a.UndoAvailable() {
				t.Fatal("missing durable journal remained advertised as undoable")
			}
		})
	}
}

func newDurableUndoFixture(t *testing.T, status taskstate.Status) (*Agent, *taskstate.Store, *taskstate.Task) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "fixed.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cp, err := checkpoint.Capture(root, []string{"fixed.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	observation, err := workspacepkg.Capture(t.Context(), root, []string{"fixed.txt"})
	if err != nil {
		t.Fatal(err)
	}

	task, err := taskstate.New("undo-persistence", "restore the workspace safely", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := task.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	sequence, ok := task.BeginTurn(taskstate.TurnRouteImplement)
	if !ok {
		t.Fatal("turn did not start")
	}
	task.RecordChanged("fixed.txt")
	if !task.FinishTurn(sequence, taskstate.TurnRouteImplement, "edit the fixed file", "PASS", taskstate.StopNone, 1, 1) {
		t.Fatal("turn did not finish")
	}
	task.AddVerificationWithObservation("verify fixed.txt", true, "verify PASS\n1 passed", &observation)
	if status == taskstate.StatusDone {
		if err := task.SetStatus(taskstate.StatusDone); err != nil {
			t.Fatal(err)
		}
	}
	store := taskstate.NewStore(t.TempDir())
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Workspace = root
	cfg.Mode = config.ModeFast
	cfg.Provider = config.ProviderOllama
	a := New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: root}), perm.New(config.ModeFast, root, nil))
	a.TaskStore = store
	a.TaskSession = task.SessionID
	a.task = task
	if err := cp.Seal(); err != nil {
		t.Fatal(err)
	}
	a.latestUndo = &turnUndo{
		workspace: root, checkpoint: cp, sessionID: task.SessionID,
		taskID: task.ID, turnIntentRevision: task.LastTurn().IntentRevision,
	}
	return a, store, task
}

func testUndoWorkspaceInstance(t *testing.T, root string) undoWorkspaceInstance {
	t.Helper()
	instance, err := ensureUndoWorkspaceInstance(root)
	if err != nil {
		t.Fatal(err)
	}
	return instance
}

func assertUndoFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}
