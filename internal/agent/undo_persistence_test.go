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

func TestUndoReportsCASFailureAfterRestoration(t *testing.T) {
	a, store, task := newDurableUndoFixture(t, taskstate.StatusWorking)
	other, err := store.Load(task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	other.NoteAttempt()
	if err := store.Save(other); err != nil {
		t.Fatal(err)
	}

	_, err = a.UndoLastTurn()
	if err == nil || !strings.Contains(err.Error(), "files restored but durable task state was not saved") || !errors.Is(err, taskstate.ErrRevisionConflict) {
		t.Fatalf("undo CAS failure = %v", err)
	}
	assertUndoFileContent(t, filepath.Join(a.ConfigSnapshot().Workspace, "fixed.txt"), "before\n")
	if a.UndoAvailable() {
		t.Fatal("checkpoint remained available after files were restored")
	}
	if got := a.TaskSnapshot(); got == nil || got.Revision != task.Revision || got.VerifiedChangeSeq != task.ChangeSeq || !got.Verification[len(got.Verification)-1].Passed {
		t.Fatalf("failed CAS published invalidated task = %#v", got)
	}
	persisted, err := store.Load(task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Revision != other.Revision || persisted.Attempts != other.Attempts {
		t.Fatalf("stale undo overwrote durable state = %#v", persisted)
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
	a.latestUndo = &turnUndo{workspace: root, checkpoint: cp}
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
