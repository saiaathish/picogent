package gui

import (
	"testing"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
)

func TestReconnectSnapshotDoesNotResurfaceRetiredTask(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	store := taskstate.NewStore(t.TempDir())
	const oldSession = "reconnect-old-session"
	task, err := taskstate.New(oldSession, "finish the old workspace task", []string{"implement", "verify"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	ag := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	ag.SetTaskStore(store)
	if err := ag.SetTaskSession(oldSession); err != nil {
		t.Fatal(err)
	}
	s := &server{
		cfg:       cfg,
		ag:        ag,
		sessionID: oldSession,
		hist:      []llm.Message{{Role: "user", Content: "old request"}},
	}
	if current := s.snapshot()["task"].(*taskstate.Task); current == nil || current.ID != task.ID {
		t.Fatalf("pre-rotation snapshot task = %#v", s.snapshot()["task"])
	}

	newSession, saveErr, taskErr := s.newSession()
	if taskErr != nil {
		t.Fatalf("load task session during rotation: %v", taskErr)
	}
	if saveErr != nil {
		t.Fatalf("save session during rotation: %v", saveErr)
	}
	current := s.snapshot()
	if current["session_id"] != newSession {
		t.Fatalf("reconnect session = %v, want %q", current["session_id"], newSession)
	}
	if task, ok := current["task"].(*taskstate.Task); ok && task != nil {
		t.Fatalf("reconnect resurfaced retired task: %#v", task)
	}
	if _, ok := current["messages"]; ok {
		t.Fatalf("reconnect resurfaced retired transcript: %#v", current["messages"])
	}
	if preserved, err := store.Load(oldSession); err != nil || preserved.ID != task.ID {
		t.Fatalf("retired task was not preserved for explicit resume: task=%#v err=%v", preserved, err)
	}
}
