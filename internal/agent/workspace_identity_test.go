package agent_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/tools"
)

type replaceWorkspaceOnApproval struct {
	allowAll
	once         sync.Once
	workspace    string
	oldWorkspace string
	err          error
}

func (h *replaceWorkspaceOnApproval) OnNeedPermission(_ context.Context, req perm.Request) (perm.Decision, error) {
	if req.Tool != "write_file" {
		return perm.Allow, nil
	}
	h.once.Do(func() {
		h.err = os.Rename(h.workspace, h.oldWorkspace)
		if h.err == nil {
			h.err = os.Mkdir(h.workspace, 0o700)
		}
	})
	if h.err != nil {
		return perm.Deny, h.err
	}
	return perm.Allow, nil
}

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
	reg := tools.NewRegistry(tools.Context{Workspace: workspace})
	a := agent.New(cfg, fake, reg, perm.New(config.ModeSafe, workspace, nil))
	handler := &replaceWorkspaceOnApproval{workspace: workspace, oldWorkspace: oldWorkspace}
	if _, _, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "create blocked.txt"}, handler); err != nil {
		t.Fatal(err)
	}
	if handler.err != nil {
		t.Fatalf("replace workspace during approval: %v", handler.err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "blocked.txt")); !os.IsNotExist(err) {
		t.Fatalf("replacement workspace received the approved write: %v", err)
	}
	if _, err := os.Stat(filepath.Join(oldWorkspace, "blocked.txt")); !os.IsNotExist(err) {
		t.Fatalf("original workspace received a write after replacement: %v", err)
	}
}
