package agent_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
)

func TestDurableTaskPersistsOutsideHistoryAndResumes(t *testing.T) {
	workspace := t.TempDir()
	store := taskstate.NewStore(t.TempDir())
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{Workspace: workspace})
	a := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "done"}}}}, reg, perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = store
	a.SetTaskSession("session-1")

	history, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "audit the auth flow"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Task == nil || result.Task.Status != taskstate.StatusDone {
		t.Fatalf("task=%#v", result.Task)
	}
	for _, msg := range history {
		if msg.Role == "system" && msg.Content == "" {
			t.Fatal("empty internal message persisted")
		}
	}

	resumed := agent.New(cfg, &llm.Scripted{}, reg, perm.New(config.ModeFast, workspace, nil))
	resumed.TaskStore = store
	resumed.SetTaskSession("session-1")
	if got := resumed.TaskSnapshot(); got == nil || got.ID != result.Task.ID || got.Goal != result.Task.Goal {
		t.Fatalf("resumed=%#v want=%#v", got, result.Task)
	}
}

func TestDurableTaskContinuesPastRoutineDeferral(t *testing.T) {
	workspace := t.TempDir()
	args, _ := json.Marshal(map[string]string{"path": "fixed.txt", "content": "fixed"})
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", Content: "I found the issue. Would you like me to fix it?"}},
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "1", Name: "write_file", Arguments: string(args)}}}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}}
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{Workspace: workspace})
	a := agent.New(cfg, fake, reg, perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = taskstate.NewStore(t.TempDir())
	a.SetTaskSession("session-2")

	history, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "fix the broken signup flow"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.Calls) != 3 {
		t.Fatalf("model calls=%d want 3", len(fake.Calls))
	}
	if result.Task == nil || result.Task.Status != taskstate.StatusDone {
		t.Fatalf("task=%#v", result.Task)
	}
	if len(result.FilesChanged) != 1 || filepath.Clean(result.FilesChanged[0]) != "fixed.txt" {
		t.Fatalf("changed=%v", result.FilesChanged)
	}
	for _, msg := range history {
		if msg.Role == "system" && msg.Content == "Internal task-loop instruction: the original request already authorizes the work. Do not ask whether to continue. Take the next obvious safe action with tools. Stop only for permission, a genuine user choice, repeated verification failure, an unavailable resource, or exhausted budget." {
			t.Fatal("internal continuation prompt persisted in chat history")
		}
	}
}
