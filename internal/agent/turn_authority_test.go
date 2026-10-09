package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
)

type blockingAuthorityClient struct {
	calls chan llm.ChatRequest
}

func (c *blockingAuthorityClient) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	c.calls <- req
	<-ctx.Done()
	return llm.ChatResponse{}, ctx.Err()
}

type authorityRunOutcome struct {
	result Result
	err    error
}

type taskStoreRebindPermissionEvents struct {
	NopHandler
	agent       *Agent
	replacement *taskstate.Store
	prompts     int
}

func (e *taskStoreRebindPermissionEvents) OnNeedPermission(context.Context, perm.Request) (perm.Decision, error) {
	e.prompts++
	e.agent.SetTaskStore(e.replacement)
	return perm.Allow, nil
}

func newAuthorityRunAgent(t *testing.T, root string, client llm.Client, mode config.Mode) *Agent {
	t.Helper()
	cfg := config.Default()
	cfg.Workspace = root
	cfg.Mode = mode
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{Workspace: root})
	a := New(cfg, client, reg, perm.New(mode, root, nil))
	a.SetTaskStore(taskstate.NewStore(t.TempDir()))
	if err := a.SetTaskSession("run-authority"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	return a
}

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

func TestTurnAuthorityRejectsStoreChangedAfterLockAcquisition(t *testing.T) {
	for _, change := range []string{"copied store", "same pointer", "ABA"} {
		t.Run(change, func(t *testing.T) {
			a := newUndoHookAgent(t, t.TempDir())
			defer a.Close()
			original := taskstate.NewStore(t.TempDir())
			a.SetTaskStore(original)
			client := &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "stale answer"}}}}
			a.SetClient(client)
			opts := RunOptions{afterProjectRunLock: func() {
				if change != "same pointer" {
					a.SetTaskStore(taskstate.NewStore(t.TempDir()))
				}
				if change != "copied store" {
					a.SetTaskStore(original)
				}
			}}
			_, _, err := a.RunWithOptions(context.Background(), nil, llm.Message{Role: "user", Content: "explain this project"}, NopHandler{}, opts)
			if err == nil || !strings.Contains(err.Error(), "ownership changed") || len(client.Calls) != 0 {
				t.Fatalf("claimed a different authority from the held lock: err=%v calls=%d", err, len(client.Calls))
			}
		})
	}
}

func TestTurnAuthorityAdmitsClosedProgressAfterLockWait(t *testing.T) {
	a := newUndoHookAgent(t, t.TempDir())
	defer a.Close()
	a.SetTaskStore(taskstate.NewStore(t.TempDir()))
	if err := a.SetTaskSession("queued-closed-progress"); err != nil {
		t.Fatal(err)
	}
	if failed, err := a.beginDurableTask("investigate the broken build", NopHandler{}); failed || err != nil {
		t.Fatal(err)
	}
	sequence, started := a.beginDurableTurn(taskstate.TurnRouteInspect, NopHandler{})
	if !started {
		t.Fatal("start prior cooperative turn")
	}
	client := &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "inspection result"}}}}
	a.SetClient(client)
	opts := RunOptions{beforeProjectRunLock: func() {
		closed, err := a.closeDurableTurn(sequence, false, taskstate.TurnRouteInspect, "earlier inspection finished", "", taskstate.StopNone, 0, 0, NopHandler{})
		if err != nil || !closed {
			t.Fatalf("close preceding turn: %v", err)
		}
	}}
	_, result, err := a.RunWithOptions(context.Background(), nil, llm.Message{Role: "user", Content: "continue investigating the build"}, NopHandler{}, opts)
	if err != nil || len(client.Calls) != 1 || result.Task == nil || result.Task.TurnRevision <= sequence {
		t.Fatalf("locked substrate refused legitimate queued progress: result=%+v calls=%d err=%v", result, len(client.Calls), err)
	}
}

type authorityStreamingClient struct{ canceled bool }

func (c *authorityStreamingClient) Chat(ctx context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	req.OnDelta("Goal complete: ")
	c.canceled = ctx.Err() != nil
	// Deliberately violate cancellation so the delivery gate must also refuse.
	req.OnDelta("stale narration")
	return llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: "Goal complete: stale narration"}}, nil
}

type authorityTextEvents struct {
	nativeOwnershipEvents
	onDelta func()
	visible string
	finals  []string
}

func (h *authorityTextEvents) OnTextDelta(text string) {
	h.visible += text
	if h.onDelta != nil {
		h.onDelta()
	}
}

func (h *authorityTextEvents) OnText(text string) { h.visible += text }
func (h *authorityTextEvents) OnTextFinal(text string) {
	h.visible = text
	h.finals = append(h.finals, text)
}

func TestTurnAuthorityRetractsRevokedStreamingNarration(t *testing.T) {
	a := newUndoHookAgent(t, t.TempDir())
	defer a.Close()
	store := taskstate.NewStore(t.TempDir())
	a.SetTaskStore(store)
	if err := a.SetTaskSession("revoked-stream"); err != nil {
		t.Fatal(err)
	}
	client := &authorityStreamingClient{}
	a.SetClient(client)
	h := &authorityTextEvents{}
	h.onDelta = func() {
		if h.visible == "Goal complete: " {
			a.SetTaskStore(store)
		}
	}
	history, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "finish the project"}, h)
	if err == nil || !strings.Contains(err.Error(), "ownership changed") || !client.canceled || h.visible != "" || len(h.finals) != 1 || result.GoalDone || result.Text != "" {
		t.Fatalf("revoked stream reached the surface: canceled=%v visible=%q finals=%v result=%+v err=%v", client.canceled, h.visible, h.finals, result, err)
	}
	for _, message := range history {
		if message.Role == "assistant" && strings.Contains(message.Content, "stale narration") {
			t.Fatal("revoked narration was retained in history")
		}
	}
}

func TestTurnAuthorityRefusesTextAfterTerminalCallbackRevocation(t *testing.T) {
	a := newUndoHookAgent(t, t.TempDir())
	defer a.Close()
	store := taskstate.NewStore(t.TempDir())
	a.SetTaskStore(store)
	if err := a.SetTaskSession("terminal-callback"); err != nil {
		t.Fatal(err)
	}
	a.SetClient(&llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "stale terminal answer"}}}})
	h := &authorityTextEvents{}
	h.state = func(task *taskstate.Task) {
		if last := task.LastTurn(); last != nil && last.State == taskstate.TurnCompleted {
			a.SetTaskStore(store)
		}
	}
	history, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "investigate the broken build"}, h)
	if err == nil || h.visible != "" || result.Text != "" || result.GoalDone {
		t.Fatalf("terminal callback revocation published a result: visible=%q result=%+v err=%v", h.visible, result, err)
	}
	for _, message := range history {
		if message.Role == "assistant" && strings.Contains(message.Content, "stale terminal answer") {
			t.Fatal("revoked terminal response was retained in history")
		}
	}
}

func TestTaskStoreReplacementCancelsBlockedProvider(t *testing.T) {
	root := t.TempDir()
	client := &blockingAuthorityClient{calls: make(chan llm.ChatRequest, 2)}
	a := newAuthorityRunAgent(t, root, client, config.ModeFast)
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan authorityRunOutcome, 1)
	go func() {
		_, result, err := a.Run(runCtx, nil, llm.Message{Role: "user", Content: "inspect the project"}, allowUndoTest{})
		done <- authorityRunOutcome{result: result, err: err}
	}()
	select {
	case <-client.calls:
	case <-time.After(5 * time.Second):
		t.Fatal("provider call did not start")
	}

	a.SetTaskStore(taskstate.NewStore(t.TempDir()))
	select {
	case outcome := <-done:
		if outcome.err == nil || !errors.Is(outcome.err, context.Canceled) || !strings.Contains(outcome.err.Error(), "ownership changed") {
			t.Fatalf("revoked provider run = %+v, want canceled ownership refusal", outcome)
		}
		if outcome.result.GoalDone || outcome.result.Text != "" {
			t.Fatalf("revoked provider response was finalized: %+v", outcome.result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("store replacement did not cancel the blocked provider")
	}
	if len(client.calls) != 0 {
		t.Fatalf("provider started %d additional calls after revocation", len(client.calls))
	}
}

func TestTaskSessionSwitchCancelsBlockedProvider(t *testing.T) {
	root := t.TempDir()
	client := &blockingAuthorityClient{calls: make(chan llm.ChatRequest, 2)}
	a := newAuthorityRunAgent(t, root, client, config.ModeFast)
	if failed, err := a.beginDurableTask("fix the broken note workflow", NopHandler{}); failed || err != nil {
		t.Fatalf("seed durable task: failed=%v err=%v", failed, err)
	}
	original := a.nativeTaskBinding()
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan authorityRunOutcome, 1)
	go func() {
		_, result, err := a.Run(runCtx, nil, llm.Message{Role: "user", Content: "inspect the project"}, allowUndoTest{})
		done <- authorityRunOutcome{result: result, err: err}
	}()
	select {
	case <-client.calls:
	case <-time.After(5 * time.Second):
		t.Fatal("provider call did not start")
	}

	sessionChanged := make(chan error, 1)
	go func() { sessionChanged <- a.SetTaskSession("replacement-session") }()
	select {
	case outcome := <-done:
		if outcome.err == nil || !errors.Is(outcome.err, context.Canceled) {
			t.Fatalf("session-switch provider run = %+v, want canceled run", outcome)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("session switch did not cancel the blocked provider before waiting on the run lock")
	}
	select {
	case err := <-sessionChanged:
		if err != nil {
			t.Fatalf("switch durable task session: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("session switch did not finish after the canceled run released its lock")
	}
	if got, _ := a.taskSessionSnapshot(); got != "replacement-session" {
		t.Fatalf("active session = %q, want replacement-session", got)
	}
	oldTask, err := original.store.Load(original.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	last := oldTask.LastTurn()
	if last == nil || last.State != taskstate.TurnInterrupted || last.StopReason != taskstate.StopCanceled {
		t.Fatalf("old session turn after switch = %#v, want interrupted/canceled", last)
	}
}

func TestTaskSessionSwitchCancelsBlockedTool(t *testing.T) {
	root := t.TempDir()
	call := fallbackCall(t, "blocked-read", "read_file", map[string]string{"path": "note.txt"})
	client := &llm.Scripted{Responses: fallbackResponses(call)}
	a := newAuthorityRunAgent(t, root, client, config.ModeFast)
	if failed, err := a.beginDurableTask("fix the broken note workflow", NopHandler{}); failed || err != nil {
		t.Fatalf("seed durable task: failed=%v err=%v", failed, err)
	}
	toolStarted := make(chan struct{}, 1)
	a.runTool = func(ctx context.Context, _ llm.ToolCall, _ tools.Tool, _ tools.Context) (string, error) {
		toolStarted <- struct{}{}
		<-ctx.Done()
		return "", ctx.Err()
	}
	done := make(chan authorityRunOutcome, 1)
	go func() {
		_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "fix the broken note workflow"}, NopHandler{})
		done <- authorityRunOutcome{result: result, err: err}
	}()
	select {
	case <-toolStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("tool did not start")
	}

	sessionChanged := make(chan error, 1)
	go func() { sessionChanged <- a.SetTaskSession("replacement-session") }()
	select {
	case outcome := <-done:
		if outcome.err == nil || !errors.Is(outcome.err, context.Canceled) {
			t.Fatalf("session-switch tool run = %+v, want canceled run", outcome)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("session switch did not cancel the blocked tool")
	}
	select {
	case err := <-sessionChanged:
		if err != nil {
			t.Fatalf("switch durable task session: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("session switch did not finish after the canceled tool released its lock")
	}
	if len(client.Calls) != 1 {
		t.Fatalf("model calls after tool cancellation = %d, want only the pre-tool call", len(client.Calls))
	}
}

func TestTaskSessionSwitchPreventsNewRunClaim(t *testing.T) {
	a := newAuthorityRunAgent(t, t.TempDir(), &llm.Scripted{}, config.ModeFast)
	binding := a.nativeTaskBinding()
	releaseSwitch, err := a.reserveTaskSessionSwitch(binding)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSwitch()
	if _, err := a.claimTaskRun(context.Background(), binding); !errors.Is(err, errTaskSessionSwitchInProgress) {
		t.Fatalf("run claim during session switch = %v, want switch-in-progress refusal", err)
	}
}

func TestTaskStoreReplacementCancelsBlockedTool(t *testing.T) {
	root := t.TempDir()
	call := fallbackCall(t, "blocked-read", "read_file", map[string]string{"path": "note.txt"})
	client := &llm.Scripted{Responses: fallbackResponses(call)}
	a := newAuthorityRunAgent(t, root, client, config.ModeFast)
	toolStarted := make(chan struct{}, 1)
	a.runTool = func(ctx context.Context, _ llm.ToolCall, _ tools.Tool, _ tools.Context) (string, error) {
		toolStarted <- struct{}{}
		<-ctx.Done()
		return "", ctx.Err()
	}
	events := &nativeOwnershipEvents{}
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan authorityRunOutcome, 1)
	go func() {
		_, result, err := a.Run(runCtx, nil, llm.Message{Role: "user", Content: "inspect note.txt"}, events)
		done <- authorityRunOutcome{result: result, err: err}
	}()
	select {
	case <-toolStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("tool did not start")
	}

	a.SetTaskStore(taskstate.NewStore(t.TempDir()))
	select {
	case outcome := <-done:
		if outcome.err == nil || !errors.Is(outcome.err, context.Canceled) || !strings.Contains(outcome.err.Error(), "ownership changed") {
			t.Fatalf("revoked tool run = %+v, want canceled ownership refusal", outcome)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("store replacement did not cancel the blocked tool")
	}
	if len(client.Calls) != 1 {
		t.Fatalf("model calls = %d, want only the pre-tool call", len(client.Calls))
	}
	if len(events.ends) != 1 || !strings.Contains(events.ends[0], context.Canceled.Error()) {
		t.Fatalf("started tool completion was not reported accurately: %v", events.ends)
	}
}

func TestPermissionCallbackTaskStoreReplacementStopsTool(t *testing.T) {
	root := t.TempDir()
	call := fallbackCall(t, "prompted-write", "write_file", map[string]string{"path": "note.txt", "content": "must not write"})
	client := &llm.Scripted{Responses: fallbackResponses(call)}
	a := newAuthorityRunAgent(t, root, client, config.ModeSafe)
	a.Tools.UpdateContext(func(c *tools.Context) {
		c.ClassifyPath = func(tool, path, _, _ string) perm.Request {
			return perm.Request{Tool: tool, Path: path, OutsideWorkspace: true}
		}
	})
	replacement := taskstate.NewStore(t.TempDir())
	events := &taskStoreRebindPermissionEvents{agent: a, replacement: replacement}
	runToolCalls := 0
	a.runTool = func(context.Context, llm.ToolCall, tools.Tool, tools.Context) (string, error) {
		runToolCalls++
		return "wrote note.txt", nil
	}
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, result, err := a.Run(runCtx, nil, llm.Message{Role: "user", Content: "update note.txt"}, events)
	if err == nil || !strings.Contains(err.Error(), "ownership changed") {
		t.Fatalf("permission callback rebinding = result=%+v err=%v, want ownership refusal", result, err)
	}
	if events.prompts != 1 || runToolCalls != 0 || len(client.Calls) != 1 {
		t.Fatalf("stale tool ran after permission callback: prompts=%d tools=%d modelCalls=%d", events.prompts, runToolCalls, len(client.Calls))
	}
	if _, statErr := os.Stat(filepath.Join(root, "note.txt")); !os.IsNotExist(statErr) {
		t.Fatalf("revoked permission callback allowed a write: %v", statErr)
	}
}

func TestTaskStoreReplacementDuringHealthAdmissionStopsBeforeCompaction(t *testing.T) {
	root := t.TempDir()
	client := &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "unexpected model response"}}}}
	a := newAuthorityRunAgent(t, root, client, config.ModeFast)
	healthStarted := make(chan struct{}, 1)
	a.runTool = func(ctx context.Context, call llm.ToolCall, _ tools.Tool, _ tools.Context) (string, error) {
		if call.Name != "project_health" {
			t.Fatalf("unexpected admission tool = %q", call.Name)
		}
		healthStarted <- struct{}{}
		<-ctx.Done()
		return "", ctx.Err()
	}
	history := make([]llm.Message, 20)
	for i := range history {
		history[i] = llm.Message{Role: "user", Content: strings.Repeat("context ", 5000)}
	}
	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan authorityRunOutcome, 1)
	go func() {
		_, result, err := a.Run(runCtx, history, llm.Message{Role: "user", Content: "make this ready to launch"}, allowUndoTest{})
		done <- authorityRunOutcome{result: result, err: err}
	}()
	select {
	case <-healthStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("project-health admission did not start")
	}

	a.SetTaskStore(taskstate.NewStore(t.TempDir()))
	select {
	case outcome := <-done:
		if outcome.err == nil || !strings.Contains(outcome.err.Error(), "ownership changed") {
			t.Fatalf("revoked health admission = %+v, want ownership refusal", outcome)
		}
		if outcome.result.Context.Budget != 0 {
			t.Fatalf("context compaction started after revocation: %+v", outcome.result.Context)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("store replacement did not cancel health admission")
	}
	if len(client.Calls) != 0 {
		t.Fatalf("model calls after health-admission revocation = %d, want 0", len(client.Calls))
	}
}
