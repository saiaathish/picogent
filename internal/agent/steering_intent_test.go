package agent_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
)

func TestSteeringAfterRestartPersistsIntentWithoutReplacingOutcome(t *testing.T) {
	workspace := t.TempDir()
	store := taskstate.NewStore(t.TempDir())
	const sessionID = "steering-restart"

	task, ok, err := taskstate.NewFromPrompt(sessionID, "fix the broken note workflow")
	if err != nil || !ok || task == nil {
		t.Fatalf("initial task = %#v, ok=%v, err=%v", task, ok, err)
	}
	if err := task.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	definition := append([]taskstate.Criterion(nil), task.DefinitionOfDone...)
	originalGoal := task.Goal
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}

	newAgent := func(response string) *agent.Agent {
		cfg := config.Default()
		cfg.Workspace = workspace
		cfg.Provider = config.ProviderOllama
		a := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{{
			Message: llm.Message{Role: "assistant", Content: response},
		}}}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
		a.SetTaskStore(store)
		if err := a.SetTaskSession(sessionID); err != nil {
			t.Fatal(err)
		}
		return a
	}

	first := newAgent("paused after steering")
	_, firstResult, err := first.Run(context.Background(), nil, llm.Message{
		Role: "user", Content: "instead, document the note workflow",
	}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	beforeRestart, err := store.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if beforeRestart.Intent == nil || beforeRestart.Intent.Class != "documentation" || beforeRestart.Intent.Outcome != originalGoal {
		t.Fatalf("steered intent before restart = %#v, want documentation over %q", beforeRestart.Intent, originalGoal)
	}
	if beforeRestart.IntentRevision <= task.IntentRevision {
		t.Fatalf("intent revision did not advance: initial=%d current=%d", task.IntentRevision, beforeRestart.IntentRevision)
	}
	if !reflect.DeepEqual(beforeRestart.DefinitionOfDone, definition) || beforeRestart.Goal != originalGoal {
		t.Fatalf("steering replaced durable outcome: goal=%q definition=%#v", beforeRestart.Goal, beforeRestart.DefinitionOfDone)
	}
	if firstResult.Task == nil || firstResult.Task.IntentRevision != beforeRestart.IntentRevision {
		t.Fatalf("returned snapshot diverged before restart: result=%#v persisted=%#v", firstResult.Task, beforeRestart)
	}
	if turn := beforeRestart.LastTurn(); turn == nil || turn.IntentRevision != beforeRestart.IntentRevision {
		t.Fatalf("steered turn did not bind intent revision: %#v", beforeRestart.LastTurn())
	}

	resumed := newAgent("resumed after steering")
	_, resumedResult, err := resumed.Run(context.Background(), nil, llm.Message{
		Role: "user", Content: "also audit the note workflow",
	}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	afterRestart, err := store.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if afterRestart.Intent == nil || afterRestart.Intent.Class != "review" || afterRestart.Intent.Outcome != originalGoal {
		t.Fatalf("resumed intent = %#v, want review over %q", afterRestart.Intent, originalGoal)
	}
	if afterRestart.IntentRevision <= beforeRestart.IntentRevision {
		t.Fatalf("restart did not advance intent revision: before=%d after=%d", beforeRestart.IntentRevision, afterRestart.IntentRevision)
	}
	if !reflect.DeepEqual(afterRestart.DefinitionOfDone, definition) || afterRestart.Goal != originalGoal {
		t.Fatalf("restart replaced durable outcome: goal=%q definition=%#v", afterRestart.Goal, afterRestart.DefinitionOfDone)
	}
	if turn := afterRestart.LastTurn(); turn == nil || turn.IntentRevision != afterRestart.IntentRevision {
		t.Fatalf("resumed turn did not bind intent revision: %#v", afterRestart.LastTurn())
	}
	if resumedResult.Task == nil || resumedResult.Task.IntentRevision != afterRestart.IntentRevision {
		t.Fatalf("returned snapshot diverged after restart: result=%#v persisted=%#v", resumedResult.Task, afterRestart)
	}
}
