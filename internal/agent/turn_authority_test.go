package agent

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/taskstate"
)

func copyAuthorityStore(t *testing.T, task *taskstate.Task) *taskstate.Store {
	t.Helper()
	store := taskstate.NewStore(t.TempDir())
	copy := cloneTask(task)
	want := copy.Revision
	copy.Revision = 0
	for copy.Revision < want {
		if err := store.Save(copy); err != nil {
			t.Fatal(err)
		}
	}
	return store
}

func TestTurnAuthorityRejectsStoreABAAndSamePointerSetter(t *testing.T) {
	for _, change := range []string{"ABA", "same pointer"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			a := newUndoHookAgent(t, root)
			defer a.Close()
			store := taskstate.NewStore(t.TempDir())
			a.SetTaskStore(store)
			if err := a.SetTaskSession("store-aba"); err != nil {
				t.Fatal(err)
			}
			call := fallbackCall(t, "create", "write_file", map[string]string{"path": "new/note.txt", "content": "after"})
			a.SetClient(&llm.Scripted{Responses: fallbackResponses(call)})
			h := &nativeOwnershipEvents{start: func(llm.ToolCall) {
				if change == "ABA" {
					a.SetTaskStore(taskstate.NewStore(t.TempDir()))
				}
				a.SetTaskStore(store)
			}}
			_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update new/note.txt"}, h)
			if err == nil || !strings.Contains(err.Error(), "ownership changed") || result.GoalDone {
				t.Fatalf("revoked store authority still ran: result=%+v err=%v", result, err)
			}
			if _, err := os.Stat(filepath.Join(root, "new")); !os.IsNotExist(err) {
				t.Fatalf("store revocation created parents: %v", err)
			}
		})
	}
}

type turnAuthorityEndEvents struct {
	nativeOwnershipEvents
	onEnd func()
}

func (h *turnAuthorityEndEvents) OnToolEnd(call llm.ToolCall, text string, err error) {
	h.nativeOwnershipEvents.OnToolEnd(call, text, err)
	if h.onEnd != nil {
		h.onEnd()
	}
}

func TestTurnAuthorityRejectsCopiedStoreBeforePostToolAccounting(t *testing.T) {
	root := t.TempDir()
	a := newUndoHookAgent(t, root)
	defer a.Close()
	store := taskstate.NewStore(t.TempDir())
	a.SetTaskStore(store)
	if err := a.SetTaskSession("post-tool-copy"); err != nil {
		t.Fatal(err)
	}
	call := fallbackCall(t, "create", "write_file", map[string]string{"path": "note.txt", "content": "after"})
	client := &llm.Scripted{Responses: fallbackResponses(call)}
	a.SetClient(client)
	var replacement *taskstate.Store
	var expected *taskstate.Task
	h := &turnAuthorityEndEvents{onEnd: func() {
		if replacement != nil {
			return
		}
		expected = a.TaskSnapshot()
		replacement = copyAuthorityStore(t, expected)
		expected, _ = replacement.Load("post-tool-copy")
		a.SetTaskStore(replacement)
	}}
	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update note.txt"}, h)
	got, loadErr := replacement.Load("post-tool-copy")
	if err == nil || loadErr != nil || !reflect.DeepEqual(expected, got) || len(client.Calls) != 1 || result.GoalDone {
		t.Fatalf("post-tool callback acquired copied authority: calls=%d result=%+v err=%v loadErr=%v unchanged=%v", len(client.Calls), result, err, loadErr, reflect.DeepEqual(expected, got))
	}
	if len(result.FilesChanged) != 1 || result.FilesChanged[0] != "note.txt" {
		t.Fatalf("completed publication vanished from refused result: %v", result.FilesChanged)
	}
	assertUndoFileContent(t, filepath.Join(root, "note.txt"), "after")
}

func TestTurnAuthorityRefusalDoesNotFinalizeReplacement(t *testing.T) {
	root := t.TempDir()
	a := newUndoHookAgent(t, root)
	defer a.Close()
	store := taskstate.NewStore(t.TempDir())
	a.SetTaskStore(store)
	if err := a.SetTaskSession("revoked-lazy-turn"); err != nil {
		t.Fatal(err)
	}
	call := fallbackCall(t, "create", "write_file", map[string]string{"path": "new/note.txt", "content": "after"})
	client := &llm.Scripted{Responses: fallbackResponses(call, call)}
	a.SetClient(client)
	var expected *taskstate.Task
	h := &nativeOwnershipEvents{state: func(task *taskstate.Task) {
		if expected != nil || task.LastTurn() == nil {
			return
		}
		a.taskMu.Lock()
		defer a.taskMu.Unlock()
		candidate := cloneTask(a.task)
		candidate.ID = "replacement-with-same-active-sequence"
		if err := store.Save(candidate); err != nil {
			t.Fatal(err)
		}
		a.task = candidate
		expected = cloneTask(candidate)
	}}
	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: fallbackWritePrompt}, h)
	got, loadErr := store.Load("revoked-lazy-turn")
	if expected == nil || err == nil || loadErr != nil || !reflect.DeepEqual(expected, got) || len(client.Calls) != 1 || result.GoalDone {
		t.Fatalf("refused run finalized replacement: calls=%d err=%v loadErr=%v unchanged=%v", len(client.Calls), err, loadErr, reflect.DeepEqual(expected, got))
	}
	if _, err := os.Stat(filepath.Join(root, "new")); !os.IsNotExist(err) {
		t.Fatalf("refused lazy turn created a parent: %v", err)
	}
}

func TestTurnAuthorityEagerRefusalInterruptsOriginalOnly(t *testing.T) {
	root := t.TempDir()
	a := newUndoHookAgent(t, root)
	defer a.Close()
	original := taskstate.NewStore(t.TempDir())
	a.SetTaskStore(original)
	if err := a.SetTaskSession("eager-refused"); err != nil {
		t.Fatal(err)
	}
	var replacement *taskstate.Store
	var expected *taskstate.Task
	h := &nativeOwnershipEvents{state: func(task *taskstate.Task) {
		if replacement != nil || task.LastTurn() == nil {
			return
		}
		expected = cloneTask(task)
		replacement = copyAuthorityStore(t, expected)
		expected, _ = replacement.Load("eager-refused")
		a.SetTaskStore(replacement)
	}}
	_, _, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update note.txt"}, h)
	old, oldErr := original.Load("eager-refused")
	got, newErr := replacement.Load("eager-refused")
	if err == nil || oldErr != nil || newErr != nil || old.LastTurn().State != taskstate.TurnInterrupted || !reflect.DeepEqual(expected, got) {
		t.Fatalf("refusal cleanup lost original/replacement boundary: err=%v oldErr=%v newErr=%v old=%+v unchanged=%v", err, oldErr, newErr, old.LastTurn(), reflect.DeepEqual(expected, got))
	}
}

func TestTurnAuthorityAutoVerificationPreservesPendingUndo(t *testing.T) {
	for _, boundary := range []string{"before verification", "after verification"} {
		t.Run(boundary, func(t *testing.T) {
			root := t.TempDir()
			a := newUndoHookAgent(t, root)
			defer a.Close()
			store := taskstate.NewStore(t.TempDir())
			a.SetTaskStore(store)
			const session = "revoked-auto-verification"
			if err := a.SetTaskSession(session); err != nil {
				t.Fatal(err)
			}
			checks := 0
			a.Tools.Ctx.VerifyTargets = func(context.Context, []string) (string, error) {
				checks++
				return "verify PASS", nil
			}
			var replacement *taskstate.Store
			var expected *taskstate.Task
			var pendingBefore []byte
			sealed, pending, err := undoJournalPaths(root, session)
			if err != nil {
				t.Fatal(err)
			}
			revoke := func() {
				if replacement != nil {
					return
				}
				var err error
				pendingBefore, err = os.ReadFile(pending)
				if err != nil {
					t.Fatal(err)
				}
				replacement = copyAuthorityStore(t, a.TaskSnapshot())
				expected, err = replacement.Load(session)
				if err != nil {
					t.Fatal(err)
				}
				a.SetTaskStore(replacement)
			}
			h := &turnAuthorityEndEvents{}
			h.start = func(call llm.ToolCall) {
				if call.ID == "verify-auto" && boundary == "before verification" {
					revoke()
				}
			}
			h.onEnd = func() {
				if checks == 1 && boundary == "after verification" {
					revoke()
				}
			}
			_, result, runErr := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update note.txt"}, h)
			wantChecks := 0
			if boundary == "after verification" {
				wantChecks = 1
			}
			if runErr == nil || !strings.Contains(runErr.Error(), "ownership changed") || checks != wantChecks || result.GoalDone || replacement == nil {
				t.Fatalf("revoked verifier continued: checks=%d err=%v result=%+v", checks, runErr, result)
			}
			got, err := replacement.Load(session)
			if err != nil || !reflect.DeepEqual(expected, got) {
				t.Fatalf("verification mutated replacement: err=%v unchanged=%v", err, reflect.DeepEqual(expected, got))
			}
			pendingAfter, err := os.ReadFile(pending)
			if err != nil || string(pendingAfter) != string(pendingBefore) {
				t.Fatalf("revoked terminal changed pending undo: %v", err)
			}
			if _, err := os.Stat(sealed); !os.IsNotExist(err) {
				t.Fatalf("revoked terminal sealed undo: %v", err)
			}
			assertUndoFileContent(t, filepath.Join(root, "note.txt"), "after")
		})
	}
}
