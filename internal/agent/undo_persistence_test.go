package agent

import (
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
	a.latestUndo = &turnUndo{workspace: root, checkpoint: cp, sessionID: task.SessionID}
	return a, store, task
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
