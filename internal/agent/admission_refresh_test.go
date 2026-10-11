package agent

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/taskstate"
)

func newAdmissionRefreshAgent(t *testing.T, workspace, session string, store *taskstate.Store, client *llm.Scripted) *Agent {
	t.Helper()
	a := newUndoHookAgent(t, workspace)
	a.SetTaskStore(store)
	if err := a.SetTaskSession(session); err != nil {
		t.Fatal(err)
	}
	a.SetClient(client)
	t.Cleanup(a.Close)
	return a
}

func saveAdmissionRefreshTask(t *testing.T, store *taskstate.Store, session, goal string) *taskstate.Task {
	t.Helper()
	task, ok, err := taskstate.NewFromPrompt(session, goal)
	if err != nil || !ok {
		t.Fatalf("create concurrent task: ok=%v err=%v", ok, err)
	}
	if err := task.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}
	saved, err := store.OwnershipSnapshot(session)
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

func TestFreshAdmissionCASRefusalRefreshesOnlyOnNewRun(t *testing.T) {
	const session = "fresh-admission-refresh"
	root := t.TempDir()
	store := taskstate.NewStore(t.TempDir())
	client := &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "done"}}}}
	a := newAdmissionRefreshAgent(t, root, session, store, client)
	firstEvents := &intermediateOwnershipEvents{}
	var raced *taskstate.Task
	first := RunOptions{beforeProjectRunLock: func() {
		raced = saveAdmissionRefreshTask(t, store, session, "fix the backend bug")
	}}
	_, _, err := a.RunWithOptions(context.Background(), nil, llm.Message{Role: "user", Content: "document the API"}, firstEvents, first)
	if err == nil || len(client.Calls) != 0 || len(firstEvents.updates) != 0 || a.TaskSnapshot() != nil {
		t.Fatalf("initial fresh conflict was not refused before dispatch: calls=%d updates=%d task=%+v err=%v", len(client.Calls), len(firstEvents.updates), a.TaskSnapshot(), err)
	}
	afterRefusal, err := store.OwnershipSnapshot(session)
	if err != nil || !reflect.DeepEqual(raced, afterRefusal) {
		t.Fatalf("initial refusal changed the concurrent snapshot: before=%+v after=%+v err=%v", raced, afterRefusal, err)
	}
	if a.pendingAdmissionRefresh == nil {
		t.Fatal("CAS refusal did not retain a one-shot retry permission")
	}

	path, err := store.Path(session)
	if err != nil {
		t.Fatal(err)
	}
	savedBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	corruptEvents := &intermediateOwnershipEvents{}
	_, _, err = a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "document the API"}, corruptEvents)
	if err == nil || len(client.Calls) != 0 || len(corruptEvents.updates) != 0 || a.TaskSnapshot() != nil || a.pendingAdmissionRefresh == nil {
		t.Fatalf("unreadable retry state was adopted or dispatched: calls=%d updates=%d task=%+v pending=%v err=%v", len(client.Calls), len(corruptEvents.updates), a.TaskSnapshot(), a.pendingAdmissionRefresh != nil, err)
	}
	if err := os.WriteFile(path, savedBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	retryEvents := &intermediateOwnershipEvents{}
	_, _, err = a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "document the API"}, retryEvents)
	if err != nil || len(client.Calls) != 1 {
		t.Fatalf("new request did not recover from its exact saved owner: calls=%d err=%v", len(client.Calls), err)
	}
	got := a.TaskSnapshot()
	if got == nil || got.ID != raced.ID || got.Goal != raced.Goal || got.Attempts <= raced.Attempts || a.pendingAdmissionRefresh != nil {
		t.Fatalf("retry did not continue from refreshed authoritative task: got=%+v raced=%+v pending=%v", got, raced, a.pendingAdmissionRefresh != nil)
	}
}

func TestAdmissionRefreshThenExplicitReplacementUsesRefreshedOwner(t *testing.T) {
	const session = "replacement-admission-refresh"
	root := t.TempDir()
	store := taskstate.NewStore(t.TempDir())
	client := &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "inspect the new outcome"}}}}
	a := newAdmissionRefreshAgent(t, root, session, store, client)
	var raced *taskstate.Task
	first := RunOptions{beforeProjectRunLock: func() {
		raced = saveAdmissionRefreshTask(t, store, session, "fix the backend bug")
	}}
	_, _, err := a.RunWithOptions(context.Background(), nil, llm.Message{Role: "user", Content: "document the API"}, &intermediateOwnershipEvents{}, first)
	if err == nil || a.pendingAdmissionRefresh == nil {
		t.Fatalf("fixture did not refuse the stale fresh admission: pending=%v err=%v", a.pendingAdmissionRefresh != nil, err)
	}

	_, _, err = a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "replace the current task with review the API"}, &intermediateOwnershipEvents{})
	if err != nil || len(client.Calls) != 1 {
		t.Fatalf("new explicit replacement did not run: calls=%d err=%v", len(client.Calls), err)
	}
	got := a.TaskSnapshot()
	if got == nil || got.ID != raced.ID || got.SessionID != raced.SessionID || got.Goal != "Review the API" || got.IntentRevision <= raced.IntentRevision || got.Status == taskstate.StatusDone || a.pendingAdmissionRefresh != nil {
		t.Fatalf("replacement did not preserve owner identity and reset old completion: got=%+v raced=%+v pending=%v", got, raced, a.pendingAdmissionRefresh != nil)
	}
}

func TestSuccessorAdmissionCASRefusalRefreshesLatestCompletedOwner(t *testing.T) {
	base, root := completedAdmissionFixture(t)
	store := base.TaskStore
	previous := base.TaskSnapshot()
	client := &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "done"}}}}
	a := newAdmissionRefreshAgent(t, root, previous.SessionID, store, client)
	admitted := a.TaskSnapshot()
	if admitted == nil || admitted.ID != previous.ID || admitted.Status != taskstate.StatusDone {
		t.Fatalf("completed owner did not attach: %+v", admitted)
	}
	firstEvents := &intermediateOwnershipEvents{}
	var raced *taskstate.Task
	first := RunOptions{beforeProjectRunLock: func() {
		current := a.TaskSnapshot()
		if current == nil {
			t.Fatal("completed owner disappeared before the concurrent save")
		}
		current.NoteAttempt()
		if err := store.Save(current); err != nil {
			t.Fatal(err)
		}
		updated, err := store.OwnershipSnapshot(previous.SessionID)
		if err != nil {
			t.Fatal(err)
		}
		raced = updated
	}}
	_, _, err := a.RunWithOptions(context.Background(), nil, llm.Message{Role: "user", Content: "document the API"}, firstEvents, first)
	if err == nil || len(client.Calls) != 0 || len(firstEvents.updates) != 0 || !reflect.DeepEqual(admitted, a.TaskSnapshot()) {
		t.Fatalf("initial successor conflict was not refused: calls=%d updates=%d task=%+v err=%v", len(client.Calls), len(firstEvents.updates), a.TaskSnapshot(), err)
	}
	afterRefusal, err := store.OwnershipSnapshot(previous.SessionID)
	if err != nil || !reflect.DeepEqual(raced, afterRefusal) {
		t.Fatalf("successor refusal changed concurrent progress: before=%+v after=%+v err=%v", raced, afterRefusal, err)
	}

	_, _, err = a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "document the API"}, &intermediateOwnershipEvents{})
	if err != nil || len(client.Calls) != 1 {
		t.Fatalf("successor retry did not reach the provider: calls=%d err=%v", len(client.Calls), err)
	}
	got := a.TaskSnapshot()
	if got == nil || got.ID == raced.ID || got.Goal != "Document the API" || got.Revision <= raced.Revision || len(got.ChangedFiles) != 0 || len(got.Verification) != 0 || len(got.Evidence) != 0 {
		t.Fatalf("retry did not create a clean successor from the refreshed owner: got=%+v raced=%+v", got, raced)
	}
}

func TestAdmissionRefreshPermissionIsClearedByAuthorityRebinding(t *testing.T) {
	for _, change := range []string{"same-store setter", "store ABA", "new session"} {
		t.Run(change, func(t *testing.T) {
			const session = "admission-refresh-rebind"
			root := t.TempDir()
			store := taskstate.NewStore(t.TempDir())
			client := &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "done"}}}}
			a := newAdmissionRefreshAgent(t, root, session, store, client)
			var raced *taskstate.Task
			first := RunOptions{beforeProjectRunLock: func() {
				raced = saveAdmissionRefreshTask(t, store, session, "fix the backend bug")
			}}
			_, _, err := a.RunWithOptions(context.Background(), nil, llm.Message{Role: "user", Content: "document the API"}, &intermediateOwnershipEvents{}, first)
			if err == nil || a.pendingAdmissionRefresh == nil {
				t.Fatalf("fixture did not create a pending admission refresh: pending=%v err=%v", a.pendingAdmissionRefresh != nil, err)
			}

			expected := raced
			switch change {
			case "same-store setter":
				a.SetTaskStore(store)
				if err := a.SetTaskSession(session); err != nil {
					t.Fatal(err)
				}
			case "store ABA":
				a.SetTaskStore(taskstate.NewStore(t.TempDir()))
				a.SetTaskStore(store)
				if err := a.SetTaskSession(session); err != nil {
					t.Fatal(err)
				}
			case "new session":
				expected = saveAdmissionRefreshTask(t, store, "replacement-session", "fix the frontend bug")
				if err := a.SetTaskSession("replacement-session"); err != nil {
					t.Fatal(err)
				}
			}
			if a.pendingAdmissionRefresh != nil {
				t.Fatal("authority rebind retained the old one-shot refresh")
			}
			attached := a.TaskSnapshot()
			if attached == nil || attached.ID != expected.ID || attached.SessionID != expected.SessionID {
				t.Fatalf("explicit attachment did not preserve the new authority: got=%+v want=%+v", attached, expected)
			}

			_, _, err = a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "document the API"}, &intermediateOwnershipEvents{})
			if err != nil || len(client.Calls) != 1 {
				t.Fatalf("explicitly rebound session did not run: calls=%d err=%v", len(client.Calls), err)
			}
			if got := a.TaskSnapshot(); got == nil || got.ID != expected.ID || got.SessionID != expected.SessionID {
				t.Fatalf("retry crossed into a stale task authority: got=%+v want=%+v", got, expected)
			}
		})
	}
}
