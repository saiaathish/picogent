package agent_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
)

func TestApprovedWriteRejectsWorkspaceRootReplacement(t *testing.T) {
	parent := t.TempDir()
	workspace := filepath.Join(parent, "workspace")
	oldWorkspace := filepath.Join(parent, "workspace-old")
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(workspace)
		_ = os.RemoveAll(oldWorkspace)
	})

	// Capture the approval binding against the original root, then replace the
	// directory at the same path with an ordinary attacker-owned tree. Windows
	// cannot reliably rename that root while an agent Run holds unrelated
	// project locks, so the stale binding is reinjected through ClassifyPath
	// instead of racing a mid-prompt rename.
	seed := perm.ClassifyPath("write_file", "blocked.txt", workspace, "write blocked.txt")
	approvedIdentity := seed.WorkspaceIdentity()
	if approvedIdentity == nil {
		t.Fatal("expected workspace identity binding")
	}
	if err := os.Rename(workspace, oldWorkspace); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(err)
	}

	args, err := json.Marshal(map[string]string{"path": "blocked.txt", "content": "must not publish"})
	if err != nil {
		t.Fatal(err)
	}
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "write", Name: "write_file", Arguments: string(args)}}}},
		{Message: llm.Message{Role: "assistant", Content: "root changed; the write was rejected"}},
	}}
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	reg := tools.NewRegistry(tools.Context{
		Workspace: workspace,
		ClassifyPath: func(tool, path, ws, summary string) perm.Request {
			req := perm.ClassifyPath(tool, path, ws, summary)
			return perm.BindWorkspaceIdentity(req, approvedIdentity)
		},
	})
	a := agent.New(cfg, fake, reg, perm.New(config.ModeSafe, workspace, nil))
	// Keep the agent's run lock outside the hostile workspace. Windows holds
	// the lock file open for the duration of Run, so the fixture must not make
	// that unrelated runtime lock prevent directory replacement itself.
	a.TaskStore = taskstate.NewStore(t.TempDir())
	if _, _, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "create blocked.txt"}, allowAll{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "blocked.txt")); !os.IsNotExist(err) {
		t.Fatalf("replacement workspace received the approved write: %v", err)
	}
	if _, err := os.Stat(filepath.Join(oldWorkspace, "blocked.txt")); !os.IsNotExist(err) {
		t.Fatalf("original workspace received a write after replacement: %v", err)
	}
}
