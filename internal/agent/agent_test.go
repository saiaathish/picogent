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
	"github.com/saiaathish/picogent/internal/tools"
)

func TestWritesFileThenStops(t *testing.T) {
	dir := t.TempDir()
	args, _ := json.Marshal(map[string]string{"path": "hello.txt", "content": "picogent"})
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "1", Name: "write_file", Arguments: string(args)},
		}}},
		{Message: llm.Message{Role: "assistant", Content: "Changed: hello.txt\nRun: cat hello.txt\nUndo: delete hello.txt"}},
	}}
	cfg := config.Default()
	cfg.Workspace = dir
	cfg.Mode = config.ModeFast
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{Workspace: dir})
	gate := perm.New(config.ModeFast, dir, nil)
	a := agent.New(cfg, fake, reg, gate)
	h := allowAll{}
	_, res, err := a.Run(context.Background(), nil, "create hello.txt that says picogent", h)
	if err != nil {
		t.Fatal(err)
	}
	if res.ToolRounds != 1 {
		t.Fatalf("rounds=%d", res.ToolRounds)
	}
	got, err := os.ReadFile(filepath.Join(dir, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "picogent" {
		t.Fatalf("got %q", got)
	}
}

func TestSafeModeDeniesWriteWithoutPromptAllow(t *testing.T) {
	dir := t.TempDir()
	args, _ := json.Marshal(map[string]string{"path": "x.txt", "content": "nope"})
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "1", Name: "write_file", Arguments: string(args)},
		}}},
		{Message: llm.Message{Role: "assistant", Content: "did not write"}},
	}}
	cfg := config.Default()
	cfg.Workspace = dir
	cfg.Mode = config.ModeSafe
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{Workspace: dir})
	gate := perm.New(config.ModeSafe, dir, nil)
	a := agent.New(cfg, fake, reg, gate)
	_, _, err := a.Run(context.Background(), nil, "write x.txt", denyAll{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "x.txt")); !os.IsNotExist(err) {
		t.Fatal("file should not exist")
	}
}

func TestMissingKeyStopsBeforeLLM(t *testing.T) {
	t.Setenv("PICOGENT_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("PICOGENT_CODEX_HOME", t.TempDir())
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = dir
	reg := tools.NewRegistry(tools.Context{Workspace: dir})
	gate := perm.New(config.ModeSafe, dir, nil)
	a := agent.New(cfg, &llm.Scripted{}, reg, gate)
	_, _, err := a.Run(context.Background(), nil, "hi", denyAll{})
	if err == nil || err.Error() == "" {
		t.Fatal("expected auth error")
	}
}

func (allowAll) OnNeedPermission(context.Context, perm.Request) (perm.Decision, error) {
	return perm.Allow, nil
}

type allowAll struct{ agent.NopHandler }

type denyAll struct{ agent.NopHandler }
