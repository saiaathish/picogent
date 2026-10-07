package agent

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/goal"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
	"github.com/saiaathish/picogent/internal/workspace"
)

func replacementAgent(t *testing.T, cfg config.Config, store *taskstate.Store, client *llm.Scripted) *Agent {
	t.Helper()
	a := New(cfg, client, tools.NewRegistry(tools.Context{Workspace: cfg.Workspace}), perm.New(config.ModeFast, cfg.Workspace, nil))
	t.Cleanup(a.Close)
	a.SetTaskStore(store)
	if err := a.SetTaskSession("replacement-session"); err != nil {
		t.Fatal(err)
	}
	return a
}

func replacementFixture(t *testing.T) (*Agent, *taskstate.Store, config.Config) {
	t.Helper()
	t.Setenv("PICOGENT_HOME", t.TempDir())
	cfg := config.Default()
	cfg.Workspace, cfg.Provider = t.TempDir(), config.ProviderOllama
	store := taskstate.NewStore(t.TempDir())
	a := replacementAgent(t, cfg, store, &llm.Scripted{})
	if failed, err := a.beginDurableTask("fix the backend bug", NopHandler{}); failed || err != nil {
		t.Fatal(err)
	}
	return a, store, cfg
}

func TestExplicitReplacementAdmissionAndOrdinarySteering(t *testing.T) {
	for _, prompt := range []string{"also audit the frontend", "instead, document the API", "focus on mobile first", "what does replace the current goal with document the API mean?"} {
		t.Run(prompt, func(t *testing.T) {
			a, _, _ := replacementFixture(t)
			before := a.TaskSnapshot()
			if failed, err := a.beginDurableTask(prompt, NopHandler{}); failed || err != nil {
				t.Fatal(err)
			}
			after := a.TaskSnapshot()
			if after.Goal != before.Goal || !reflect.DeepEqual(after.DefinitionOfDone, before.DefinitionOfDone) {
				t.Fatal("ordinary steering replaced the outcome")
			}
		})
	}
	a, store, _ := replacementFixture(t)
	before := a.TaskSnapshot()
	if failed, err := a.beginDurableTask("replace the current goal with document the API", NopHandler{}); failed || err != nil {
		t.Fatal(err)
	}
	after := a.TaskSnapshot()
	if after.ID != before.ID || after.Goal != "Document the API" || after.IntentRevision <= before.IntentRevision || reflect.DeepEqual(after.DefinitionOfDone, before.DefinitionOfDone) {
		t.Fatalf("replacement not admitted: %#v", after)
	}
	persisted, err := store.Load(after.SessionID)
	if err != nil || persisted.Goal != after.Goal || persisted.IntentRevision != after.IntentRevision {
		t.Fatalf("replacement not durable: %#v %v", persisted, err)
	}
}

func TestReplacementRetiresOldWorkspaceGoalAfterRestart(t *testing.T) {
	a, store, cfg := replacementFixture(t)
	const old = "finish the original backend outcome"
	revision, err := goal.SetState(cfg.Workspace, old)
	if err != nil {
		t.Fatal(err)
	}
	a.SetGoalState(old, revision)
	if failed, err := a.beginDurableTask("replace the current goal with document the API", NopHandler{}); failed || err != nil {
		t.Fatal(err)
	}
	// Pending snapshots must not expose a writable alias.
	snapshot := a.TaskSnapshot()
	snapshot.ReplacedWorkspaceGoal.Text = "tampered"
	if a.TaskSnapshot().ReplacedWorkspaceGoal.Text != old {
		t.Fatal("retirement snapshot aliases live state")
	}
	// Crash window: the new task is durable, but the old workspace goal has
	// not yet been retired. A restarted ordinary turn must reconcile first.
	client := &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "Blocked: documentation evidence still needs collection."}}}}
	restarted := replacementAgent(t, cfg, store, client)
	restarted.SetGoalState(old, revision)
	_, result, err := restarted.Run(context.Background(), nil, llm.Message{Role: "user", Content: "continue"}, NopHandler{})
	if err != nil || result.Task == nil || result.Task.Goal != "Document the API" || len(client.Calls) == 0 {
		t.Fatalf("restart did not resume replacement: %#v %v", result, err)
	}
	if strings.Contains(client.Calls[0].Messages[0].Content, old) || !strings.Contains(client.Calls[0].Messages[0].Content, "Document the API") {
		t.Fatal("provider received the retired workspace goal")
	}
	state, err := goal.LoadState(cfg.Workspace)
	if err != nil || state.Text != "" {
		t.Fatalf("old workspace goal remained: %#v %v", state, err)
	}
	if text, _ := restarted.GoalStateSnapshot(); text != "" {
		t.Fatal("runtime still injects the old goal")
	}
	persisted, err := store.Load("replacement-session")
	if err != nil || persisted.ReplacedWorkspaceGoal != nil {
		t.Fatalf("retirement was not acknowledged: %#v %v", persisted, err)
	}
}

func TestReplacementCannotRetireSameTextNewerGoal(t *testing.T) {
	for _, memoryNewer := range []bool{false, true} {
		t.Run(map[bool]string{false: "persisted", true: "runtime"}[memoryNewer], func(t *testing.T) {
			a, _, cfg := replacementFixture(t)
			const old = "finish the backend"
			revision, err := goal.SetState(cfg.Workspace, old)
			if err != nil {
				t.Fatal(err)
			}
			a.SetGoalState(old, revision)
			if failed, err := a.beginDurableTask("replace the current task with document the API", NopHandler{}); failed || err != nil {
				t.Fatal(err)
			}
			newer, err := goal.SetState(cfg.Workspace, old)
			if err != nil {
				t.Fatal(err)
			}
			if memoryNewer {
				a.SetGoalState(old, newer)
			}
			client := &llm.Scripted{}
			a.SetClient(client)
			_, _, err = a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "continue"}, NopHandler{})
			if err == nil || len(client.Calls) != 0 {
				t.Fatal("conflicting newer goal did not stop before provider dispatch")
			}
			state, err := goal.LoadState(cfg.Workspace)
			if err != nil || state.Text != old || state.Revision != newer {
				t.Fatalf("newer goal was retired: %#v %v", state, err)
			}
		})
	}
}

func TestReplacementRetirementFailureKeepsDurableReplacement(t *testing.T) {
	a, store, cfg := replacementFixture(t)
	a.SetGoalState("finish the backend", 1)
	if failed, err := a.beginDurableTask("replace the current task with document the API", NopHandler{}); failed || err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PICOGENT_HOME", blocked)
	if _, _, err := a.reconcileReplacedWorkspaceGoal(cfg.Workspace, "finish the backend", 1); err == nil {
		t.Fatal("retirement failure was silently accepted")
	}
	persisted, err := store.Load("replacement-session")
	if err != nil || persisted.Goal != "Document the API" || persisted.ReplacedWorkspaceGoal == nil {
		t.Fatalf("retryable replacement was lost: %#v %v", persisted, err)
	}
	if text, revision := a.GoalStateSnapshot(); text != "finish the backend" || revision != 1 {
		t.Fatal("failed retirement changed runtime goal")
	}
}

func TestReplacementRejectsDelayedCompletionAndCancellation(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		t.Run(map[bool]string{false: "completion", true: "cancellation"}[cancel], func(t *testing.T) {
			a, store, cfg := replacementFixture(t)
			oldSequence, ok := a.beginDurableTurn(taskstate.TurnRouteImplement, NopHandler{})
			if !ok {
				t.Fatal("old turn did not begin")
			}
			newer, err := store.Load("replacement-session")
			if err != nil {
				t.Fatal(err)
			}
			if err := newer.ReplaceOutcome("document the API"); err != nil {
				t.Fatal(err)
			}
			newSequence, ok := newer.BeginTurn(taskstate.TurnRouteInspect)
			if !ok {
				t.Fatal("replacement turn did not begin")
			}
			if err := store.Save(newer); err != nil {
				t.Fatal(err)
			}
			if cancel {
				closed, err := a.closeDurableTurn(oldSequence, true, taskstate.TurnRouteRecover, "old canceled", "PASS", taskstate.StopCanceled, 1, 0, NopHandler{})
				if err != nil || closed {
					t.Fatalf("old cancellation applied: %v %v", closed, err)
				}
			} else {
				_, projection, done, closed, superseded, err := a.finishAndCloseDurableTurn(context.Background(), cfg.Workspace, oldSequence,
					"Goal complete: old work done", "", TaskAgent, "verify PASS", nil, true, "", "", 1, 0, NopHandler{})
				if err != nil || projection.Ready || done || closed || !superseded {
					t.Fatalf("old completion applied: ready=%v done=%v closed=%v superseded=%v err=%v", projection.Ready, done, closed, superseded, err)
				}
			}
			persisted, err := store.Load("replacement-session")
			if err != nil || persisted.Goal != "Document the API" || persisted.LastTurn().Sequence != newSequence || persisted.LastTurn().State != taskstate.TurnActive {
				t.Fatalf("new turn changed by old callback: %#v %v", persisted, err)
			}
		})
	}
}

func TestReplacementAdmissionPinsOriginalRuntimeGoal(t *testing.T) {
	a, _, cfg := replacementFixture(t)
	const old = "finish the backend"
	revision, err := goal.SetState(cfg.Workspace, old)
	if err != nil {
		t.Fatal(err)
	}
	a.SetGoalState(old, revision)
	admitted := a.RuntimeSnapshot()
	newer, err := goal.SetState(cfg.Workspace, old)
	if err != nil {
		t.Fatal(err)
	}
	a.SetGoalState(old, newer)
	if failed, err := a.beginDurableTaskInState("replace the current goal with document the API", NopHandler{}, false, admitted); failed || err != nil {
		t.Fatal(err)
	}
	if a.TaskSnapshot().ReplacedWorkspaceGoal.Revision != revision {
		t.Fatal("admission captured a newer runtime identity")
	}
	if _, _, err := a.reconcileReplacedWorkspaceGoal(cfg.Workspace, old, revision); err == nil {
		t.Fatal("newer goal did not block retirement")
	}
	current, err := goal.LoadState(cfg.Workspace)
	if err != nil || current.Text != old || current.Revision != newer {
		t.Fatalf("newer goal was changed: %+v %v", current, err)
	}
}

func TestReplacementAcknowledgementAllowsLaterWorkspaceGoal(t *testing.T) {
	a, store, cfg := replacementFixture(t)
	old := strings.Repeat("original outcome ", 80)
	old = strings.TrimSpace(old)
	revision, err := goal.SetState(cfg.Workspace, old)
	if err != nil {
		t.Fatal(err)
	}
	a.SetGoalState(old, revision)
	if failed, err := a.beginDurableTask("replace the current goal with document the API", NopHandler{}); failed || err != nil {
		t.Fatal(err)
	}
	text, rev, err := a.reconcileReplacedWorkspaceGoal(cfg.Workspace, old, revision)
	if err != nil || text != "" || rev != 0 {
		t.Fatalf("retirement failed: %q %d %v", text, rev, err)
	}
	const later = "finish the mobile app"
	laterRevision, err := goal.SetState(cfg.Workspace, later)
	if err != nil {
		t.Fatal(err)
	}
	restarted := replacementAgent(t, cfg, store, &llm.Scripted{})
	restarted.SetGoalState(later, laterRevision)
	text, rev, err = restarted.reconcileReplacedWorkspaceGoal(cfg.Workspace, later, laterRevision)
	if err != nil || text != later || rev != laterRevision {
		t.Fatalf("historical replacement rejected later goal: %q %d %v", text, rev, err)
	}
}

func TestReplacementOfCompletedTaskPreservesIdentity(t *testing.T) {
	a, _, _ := replacementFixture(t)
	if _, err := a.mutateTaskResult(func(task *taskstate.Task) error {
		for _, index := range task.RequiredCriterionIndices() {
			task.RecordCriterionVerification(index, "PASS", "old criteria checked", "old verifier")
		}
		return task.SetStatus(taskstate.StatusDone)
	}); err != nil {
		t.Fatal(err)
	}
	before := a.TaskSnapshot()
	if before.NeedsVerification() {
		t.Fatal("fixture still needs verification")
	}
	if failed, err := a.beginDurableTask("replace the current task with document the API", NopHandler{}); failed || err != nil {
		t.Fatal(err)
	}
	after := a.TaskSnapshot()
	if after.ID != before.ID || after.IntentRevision != before.IntentRevision+1 || after.Goal != "Document the API" || after.CompletionReady() {
		t.Fatalf("completed replacement lost its identity or reused proof: %+v", after)
	}
}

func TestReplacementInvalidPayloadStopsBeforeProvider(t *testing.T) {
	for _, hasTask := range []bool{false, true} {
		for _, payload := range []string{"a better outcome", "fix README; keep the current goal"} {
			t.Run(payload+map[bool]string{false: "/new", true: "/existing"}[hasTask], func(t *testing.T) {
				a, store, _ := replacementFixture(t)
				if !hasTask {
					if err := a.SetTaskSession("new-session"); err != nil {
						t.Fatal(err)
					}
				}
				before := a.TaskSnapshot()
				client := &llm.Scripted{}
				a.SetClient(client)
				_, _, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "replace the current task with " + payload}, NopHandler{})
				if err == nil || len(client.Calls) != 0 || !reflect.DeepEqual(before, a.TaskSnapshot()) {
					t.Fatalf("invalid replacement was dispatched or mutated state: %v", err)
				}
				if !hasTask {
					if _, err := store.Load("new-session"); err == nil {
						t.Fatal("invalid replacement created a task")
					}
				}
			})
		}
	}
}

func TestReplacementAcknowledgementFailureRetriesAfterGoalClear(t *testing.T) {
	a, store, cfg := replacementFixture(t)
	const old = "finish the backend"
	revision, err := goal.SetState(cfg.Workspace, old)
	if err != nil {
		t.Fatal(err)
	}
	a.SetGoalState(old, revision)
	if failed, err := a.beginDurableTask("replace the current goal with document the API", NopHandler{}); failed || err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(t.TempDir(), "blocked-store")
	if err := os.WriteFile(blocked, []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	a.SetTaskStore(taskstate.NewStore(blocked))
	if _, _, err := a.reconcileReplacedWorkspaceGoal(cfg.Workspace, old, revision); err == nil {
		t.Fatal("failed acknowledgement was accepted")
	}
	pending, err := store.Load("replacement-session")
	if err != nil || pending.ReplacedWorkspaceGoal == nil {
		t.Fatalf("failed acknowledgement lost pending marker: %+v %v", pending, err)
	}
	state, err := goal.LoadState(cfg.Workspace)
	if err != nil || state.Text != "" {
		t.Fatalf("fixture did not reach save-after-goal-clear window: %+v %v", state, err)
	}
	restarted := replacementAgent(t, cfg, store, &llm.Scripted{})
	if _, _, err := restarted.reconcileReplacedWorkspaceGoal(cfg.Workspace, "", 0); err != nil {
		t.Fatal(err)
	}
	persisted, err := store.Load("replacement-session")
	if err != nil || persisted.ReplacedWorkspaceGoal != nil || persisted.Goal != "Document the API" {
		t.Fatalf("retry did not acknowledge exact replacement: %+v %v", persisted, err)
	}
}

func TestReplacementConflictCanRecoverWithoutRetiringNewerGoal(t *testing.T) {
	for _, beforeClear := range []bool{false, true} {
		t.Run(map[bool]string{false: "after-clear", true: "before-clear"}[beforeClear], func(t *testing.T) {
			a, store, cfg := replacementFixture(t)
			const old = "finish the backend"
			revision, err := goal.SetState(cfg.Workspace, old)
			if err != nil {
				t.Fatal(err)
			}
			a.SetGoalState(old, revision)
			if failed, err := a.beginDurableTask("replace the current goal with document the API", NopHandler{}); failed || err != nil {
				t.Fatal(err)
			}
			if !beforeClear {
				if cleared, err := goal.ClearIfState(cfg.Workspace, old, revision); err != nil || !cleared {
					t.Fatalf("could not reach cleared-before-ack crash window: %v", err)
				}
			}
			// Same-text ABA must also remain independent of the old marker.
			newRevision, err := goal.SetState(cfg.Workspace, old)
			if err != nil {
				t.Fatal(err)
			}
			client := &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "Blocked: inspect the new outcome."}}}}
			restarted := replacementAgent(t, cfg, store, client)
			restarted.SetGoalState(old, newRevision)
			_, _, err = restarted.Run(context.Background(), nil, llm.Message{Role: "user", Content: "continue"}, NopHandler{})
			if err == nil || len(client.Calls) != 0 {
				t.Fatal("conflict did not stop the admitted turn before dispatch")
			}
			persisted, err := store.Load("replacement-session")
			if err != nil || persisted.ReplacedWorkspaceGoal != nil || persisted.Goal != "Document the API" {
				t.Fatalf("obsolete marker stranded the session: %+v %v", persisted, err)
			}
			if text, rev := restarted.GoalStateSnapshot(); text != old || rev != newRevision {
				t.Fatal("conflict changed newer runtime goal")
			}
			current, err := goal.LoadState(cfg.Workspace)
			if err != nil || current.Text != old || current.Revision != newRevision {
				t.Fatal("conflict retired the newer persisted goal")
			}
			_, _, err = restarted.Run(context.Background(), nil, llm.Message{Role: "user", Content: "replace the current goal with fix the frontend bug"}, NopHandler{})
			if err != nil || len(client.Calls) == 0 || restarted.TaskSnapshot().Goal != "Fix the frontend bug" {
				t.Fatalf("fresh replacement could not recover the session: %v", err)
			}
		})
	}
}

func TestReplacementConflictRefreshesOnlyObsoleteRuntime(t *testing.T) {
	a, store, cfg := replacementFixture(t)
	const old = "finish the backend"
	revision, err := goal.SetState(cfg.Workspace, old)
	if err != nil {
		t.Fatal(err)
	}
	a.SetGoalState(old, revision)
	if failed, err := a.beginDurableTask("replace the current goal with document the API", NopHandler{}); failed || err != nil {
		t.Fatal(err)
	}
	const newer = "finish the mobile app"
	newRevision, err := goal.SetState(cfg.Workspace, newer)
	if err != nil {
		t.Fatal(err)
	}
	client := &llm.Scripted{}
	a.SetClient(client)
	_, _, err = a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "continue"}, NopHandler{})
	if err == nil || len(client.Calls) != 0 {
		t.Fatal("stale admission was dispatched")
	}
	if text, rev := a.GoalStateSnapshot(); text != newer || rev != newRevision {
		t.Fatal("next admission retained an obsolete runtime identity")
	}
	persisted, err := store.Load("replacement-session")
	if err != nil || persisted.ReplacedWorkspaceGoal != nil {
		t.Fatal("obsolete retirement marker was not acknowledged")
	}
}

func TestReplacementConflictAcknowledgementFailureKeepsRetryableMarker(t *testing.T) {
	a, store, cfg := replacementFixture(t)
	const old = "finish the backend"
	revision, err := goal.SetState(cfg.Workspace, old)
	if err != nil {
		t.Fatal(err)
	}
	a.SetGoalState(old, revision)
	if failed, err := a.beginDurableTask("replace the current goal with document the API", NopHandler{}); failed || err != nil {
		t.Fatal(err)
	}
	const newer = "finish the mobile app"
	newRevision, err := goal.SetState(cfg.Workspace, newer)
	if err != nil {
		t.Fatal(err)
	}
	a.SetGoalState(newer, newRevision)
	blocked := filepath.Join(t.TempDir(), "blocked-store")
	if err := os.WriteFile(blocked, []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	a.SetTaskStore(taskstate.NewStore(blocked))
	if _, _, err := a.reconcileReplacedWorkspaceGoal(cfg.Workspace, newer, newRevision); err == nil {
		t.Fatal("failed conflict acknowledgement was accepted")
	}
	pending, err := store.Load("replacement-session")
	if err != nil || pending.ReplacedWorkspaceGoal == nil {
		t.Fatal("failed conflict acknowledgement lost its retryable marker")
	}
	current, err := goal.LoadState(cfg.Workspace)
	if err != nil || current.Text != newer || current.Revision != newRevision {
		t.Fatal("failed conflict acknowledgement changed the newer goal")
	}
	restarted := replacementAgent(t, cfg, store, &llm.Scripted{})
	restarted.SetGoalState(newer, newRevision)
	if _, _, err := restarted.reconcileReplacedWorkspaceGoal(cfg.Workspace, newer, newRevision); err == nil {
		t.Fatal("conflict retry should still stop its admitted turn")
	}
	if text, rev, err := restarted.reconcileReplacedWorkspaceGoal(cfg.Workspace, newer, newRevision); err != nil || text != newer || rev != newRevision {
		t.Fatal("acknowledged conflict still stranded the next admission")
	}
}

type replacementNewGoalHandler struct {
	NopHandler
	agent     *Agent
	goal      string
	revision  uint64
	published bool
}

func (h *replacementNewGoalHandler) OnTaskState(task *taskstate.Task) {
	if task != nil && task.LastTurn() != nil && task.LastTurn().State == taskstate.TurnActive && !h.published {
		h.agent.SetGoalState(h.goal, h.revision)
		h.published = true
	}
}

func TestReplacementProviderUsesAdmittedGoalTuple(t *testing.T) {
	a, _, cfg := replacementFixture(t)
	const old = "finish the backend"
	revision, err := goal.SetState(cfg.Workspace, old)
	if err != nil {
		t.Fatal(err)
	}
	a.SetGoalState(old, revision)
	client := &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "Blocked: documentation evidence still needs collection."}}}}
	a.SetClient(client)
	handler := &replacementNewGoalHandler{agent: a, goal: "NEWER mobile outcome", revision: revision + 2}
	_, _, err = a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "replace the current goal with document the API"}, handler)
	if err != nil || !handler.published || len(client.Calls) == 0 {
		t.Fatalf("fixture failed: published=%v calls=%d err=%v", handler.published, len(client.Calls), err)
	}
	prompt := client.Calls[0].Messages[0].Content
	if strings.Contains(prompt, old) || strings.Contains(prompt, handler.goal) || !strings.Contains(prompt, "Document the API") {
		t.Fatal("provider prompt adopted an unadmitted goal")
	}
	text, rev := a.GoalStateSnapshot()
	if text != handler.goal || rev != handler.revision {
		t.Fatal("old turn changed newer runtime goal")
	}
}

func TestReplacementReloadKeepsHistoricalVerificationRetired(t *testing.T) {
	a, store, cfg := replacementFixture(t)
	observation, err := workspace.Capture(context.Background(), cfg.Workspace, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.mutateTaskResult(func(task *taskstate.Task) error {
		task.AddVerificationForCriteria(task.RequiredCriterionIndices(), "check old outcome", true, "verify PASS — original checks passed", &observation)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if failed, err := a.beginDurableTask("replace the current task with document the API", NopHandler{}); failed || err != nil {
		t.Fatal(err)
	}
	before, err := store.Load("replacement-session")
	if err != nil {
		t.Fatal(err)
	}
	client := &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "Blocked: documentation evidence still needs collection."}}}}
	restarted := replacementAgent(t, cfg, store, client)
	after := restarted.TaskSnapshot()
	if !reflect.DeepEqual(before.Verification, after.Verification) || !reflect.DeepEqual(before.Evidence, after.Evidence) {
		t.Fatal("session restore rewrote retired historical results or rebound new criteria")
	}
	if len(after.Verification) == 0 || !after.Verification[0].Passed || !after.Verification[0].Retired || after.ConsecutiveVerificationFailures() != 0 {
		t.Fatal("historical success changed into a current failure")
	}
	_, result, err := restarted.Run(context.Background(), nil, llm.Message{Role: "user", Content: "continue"}, NopHandler{})
	if err != nil || result.Verified != "" || result.Completion.Ready || result.GoalDone {
		t.Fatalf("old proof projected as current: %+v %v", result, err)
	}
	final, err := store.Load("replacement-session")
	if err != nil || !reflect.DeepEqual(before.Verification, final.Verification) {
		t.Fatalf("turn revalidation rewrote retired history: %+v %v", final, err)
	}
	if len(client.Calls) == 0 || !strings.Contains(client.Calls[0].Messages[0].Content, "HISTORICAL PASS") {
		t.Fatal("provider context did not distinguish historical check results")
	}
}
