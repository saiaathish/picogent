package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/workspace"
)

type cancelDuringProofContext struct {
	context.Context
	cancel context.CancelFunc
	calls  int
}

func (c *cancelDuringProofContext) Err() error {
	c.calls++
	if c.calls == 2 {
		c.cancel()
		return nil
	}
	return c.Context.Err()
}

func completionCandidate(t *testing.T) (*Agent, string, uint64) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "fixed.txt"), []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	task, err := taskstate.New("atomic-completion", "fix the file", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := task.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	task.RecordChanged("fixed.txt")
	sequence, ok := task.BeginTurn(taskstate.TurnRouteVerify)
	if !ok {
		t.Fatal("begin turn")
	}
	addCompletionProof(t, root, task)
	store := taskstate.NewStore(t.TempDir())
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}
	return &Agent{TaskStore: store, task: task}, root, sequence
}

func addCompletionProof(t *testing.T, root string, task *taskstate.Task) {
	t.Helper()
	observation, err := workspace.Capture(context.Background(), root, []string{"fixed.txt"})
	if err != nil {
		t.Fatal(err)
	}
	task.AddVerificationWithObservation("verify", true, "verify PASS\n1 passed", &observation)
}

func finishCompletionCandidate(a *Agent, root string, sequence uint64) (*taskstate.Task, CompletionProjection, bool, bool, bool, error) {
	return a.finishAndCloseDurableTurn(context.Background(), root, sequence, "Goal complete: fixed", "", TaskAgent, "verify PASS", []string{"fixed.txt"}, true, "fix the file", "", 1, 1, nil)
}

func TestAtomicCompletionRechecksProofAfterSaveFailureAndRecovery(t *testing.T) {
	a, root, sequence := completionCandidate(t)
	goodStore := a.TaskStore
	if err := os.WriteFile(filepath.Join(root, "fixed.txt"), []byte("user edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	badRoot := filepath.Join(t.TempDir(), "file-not-directory")
	if err := os.WriteFile(badRoot, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	a.TaskStore = taskstate.NewStore(badRoot)
	_, projection, goalDone, closed, _, err := finishCompletionCandidate(a, root, sequence)
	if err == nil || goalDone || closed || projection.Ready {
		t.Fatalf("failed commit leaked completion: err=%v projection=%#v goal=%v closed=%v", err, projection, goalDone, closed)
	}
	// Save-before-publish retains the old trusted state. Retrying after the
	// store recovers must recheck it, rather than treating old trust as fresh.
	if !a.task.Verification[len(a.task.Verification)-1].Passed {
		t.Fatal("failed Save changed published task state")
	}
	a.TaskStore = goodStore
	task, projection, goalDone, closed, _, err := finishCompletionCandidate(a, root, sequence)
	if err != nil || !closed || goalDone || projection.Ready || task.Status != taskstate.StatusVerifying || !task.NeedsVerification() {
		t.Fatalf("recovered finish reused stale proof: task=%#v projection=%#v goal=%v closed=%v err=%v", task, projection, goalDone, closed, err)
	}
	persisted, err := goodStore.Load(task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Verification[len(persisted.Verification)-1].Passed || persisted.Status == taskstate.StatusDone {
		t.Fatal("recovered store retained stale completion")
	}
}

func TestAtomicCompletionDoesNotInvalidateReplacementProofOnCASReplay(t *testing.T) {
	a, root, sequence := completionCandidate(t)
	current, err := a.TaskStore.Load(a.task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "fixed.txt"), []byte("replacement"), 0o644); err != nil {
		t.Fatal(err)
	}
	newSequence, ok := current.BeginTurn(taskstate.TurnRouteVerify)
	if !ok || newSequence == sequence {
		t.Fatal("replace turn")
	}
	addCompletionProof(t, root, current)
	if err := a.TaskStore.Save(current); err != nil {
		t.Fatal(err)
	}
	_, projection, goalDone, closed, superseded, err := finishCompletionCandidate(a, root, sequence)
	if err != nil || closed || goalDone || projection.Ready || !superseded {
		t.Fatalf("stale turn accepted: err=%v projection=%#v goal=%v closed=%v superseded=%v", err, projection, goalDone, closed, superseded)
	}
	persisted, err := a.TaskStore.Load(current.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	latest := persisted.Verification[len(persisted.Verification)-1]
	if !latest.Passed || !strings.HasPrefix(latest.Summary, "verify PASS") || persisted.Revision != current.Revision || persisted.LastTurn().State != taskstate.TurnActive {
		t.Fatalf("old completion revoked replacement proof: %#v", persisted)
	}
}

func TestAtomicCompletionRechecksLatestProofOnSameTurnCASReplay(t *testing.T) {
	a, root, sequence := completionCandidate(t)
	current, err := a.TaskStore.Load(a.task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "fixed.txt"), []byte("newly verified"), 0o644); err != nil {
		t.Fatal(err)
	}
	addCompletionProof(t, root, current)
	if err := a.TaskStore.Save(current); err != nil {
		t.Fatal(err)
	}
	task, projection, goalDone, closed, superseded, err := finishCompletionCandidate(a, root, sequence)
	if err != nil || !closed || !goalDone || !projection.Ready || superseded || task.Status != taskstate.StatusDone {
		t.Fatalf("fresh candidate proof rejected: task=%#v projection=%#v goal=%v closed=%v superseded=%v err=%v", task, projection, goalDone, closed, superseded, err)
	}
}

func TestAtomicCompletionCancellationDuringProofRefreshLeavesTurnRecoverable(t *testing.T) {
	a, root, sequence := completionCandidate(t)
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &cancelDuringProofContext{Context: base, cancel: cancel}
	task, projection, goalDone, closed, _, err := a.finishAndCloseDurableTurn(ctx, root, sequence, "Goal complete: fixed", "", TaskAgent, "verify PASS", []string{"fixed.txt"}, true, "fix the file", "", 1, 1, nil)
	if !errors.Is(err, context.Canceled) || goalDone || closed || projection.Ready {
		t.Fatalf("canceled proof refresh = task=%#v projection=%#v goal=%v closed=%v err=%v", task, projection, goalDone, closed, err)
	}
	persisted, err := a.TaskStore.Load(task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status == taskstate.StatusDone || persisted.LastTurn() == nil || persisted.LastTurn().State != taskstate.TurnActive {
		t.Fatalf("proof refresh committed despite cancellation: %#v", persisted)
	}
	interrupted, err := a.closeDurableTurn(sequence, true, taskstate.TurnRouteRecover, "canceled during proof refresh", "", taskstate.StopCanceled, 1, 1, nil)
	if err != nil || !interrupted {
		t.Fatalf("close canceled owned turn: interrupted=%v err=%v", interrupted, err)
	}
	persisted, err = a.TaskStore.Load(task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status == taskstate.StatusDone || persisted.LastTurn() == nil || persisted.LastTurn().State != taskstate.TurnInterrupted || persisted.LastTurn().StopReason != taskstate.StopCanceled {
		t.Fatalf("canceled proof refresh did not remain recoverable: %#v", persisted)
	}
}
