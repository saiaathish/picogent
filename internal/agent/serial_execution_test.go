package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
)

type serialAllowAll struct{ NopHandler }

func (serialAllowAll) OnNeedPermission(context.Context, perm.Request) (perm.Decision, error) {
	return perm.Allow, nil
}

func TestCoBatchedWriteCompletesBeforeVerification(t *testing.T) {
	workspace := t.TempDir()
	args, err := json.Marshal(map[string]string{"path": "fixed.txt", "content": "fixed"})
	if err != nil {
		t.Fatal(err)
	}
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "write", Name: "write_file", Arguments: string(args)},
			{ID: "verify", Name: "verify", Arguments: `{"targets":["fixed.txt"]}`},
		}}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}}
	reg := tools.NewRegistry(tools.Context{
		Workspace: workspace,
		Verify: func(context.Context) (string, error) {
			return "verify PASS\n1 passed", nil
		},
	})
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	a := New(cfg, fake, reg, perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = taskstate.NewStore(t.TempDir())
	a.SetTaskSession("serial-write-verify")
	var explicitSawFile bool
	var callOrder []string
	a.runTool = func(ctx context.Context, call llm.ToolCall, tool tools.Tool, c tools.Context) (string, error) {
		callOrder = append(callOrder, call.Name)
		if call.Name == "write_file" {
			// The old fan-out implementation could let verify run during this
			// delay. A serial turn must finish this write first.
			time.Sleep(100 * time.Millisecond)
			return tool.Run(ctx, call.Arguments, c)
		}
		if call.Name == "verify" {
			_, readErr := os.ReadFile(filepath.Join(workspace, "fixed.txt"))
			if readErr != nil {
				return "verify FAIL\nwrite was not visible", nil
			}
			explicitSawFile = true
			return "verify PASS\n1 passed", nil
		}
		return tool.Run(ctx, call.Arguments, c)
	}

	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "fix the broken flow"}, serialAllowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if !explicitSawFile {
		t.Fatalf("co-batched verification ran before the preceding write completed; calls=%v task=%#v", callOrder, result.Task)
	}
	if result.Task == nil || result.Task.Status != taskstate.StatusDone || result.Task.NeedsVerification() {
		t.Fatalf("task=%#v", result.Task)
	}
	if result.Task.ChangeSeq != 1 || result.Task.VerifiedChangeSeq != 1 {
		t.Fatalf("task evidence=%#v", result.Task)
	}
}
