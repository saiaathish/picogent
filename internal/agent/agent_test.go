package agent_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	_, res, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "create hello.txt that says picogent"}, h)
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

func TestAutoVerifyAfterWrite(t *testing.T) {
	dir := t.TempDir()
	args, _ := json.Marshal(map[string]string{"path": "hello.txt", "content": "picogent"})
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "1", Name: "write_file", Arguments: string(args)},
		}}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}}
	cfg := config.Default()
	cfg.Workspace = dir
	cfg.Mode = config.ModeFast
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{
		Workspace: dir,
		Verify:    func(context.Context) (string, error) { return "verify PASS (go test ./...)", nil },
	})
	gate := perm.New(config.ModeFast, dir, nil)
	a := agent.New(cfg, fake, reg, gate)
	_, res, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "write hello"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Verified, "PASS") {
		t.Fatalf("verified=%q", res.Verified)
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
	_, _, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "write x.txt"}, denyAll{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "x.txt")); !os.IsNotExist(err) {
		t.Fatal("file should not exist")
	}
}

func TestGoalCompleteMarksResult(t *testing.T) {
	dir := t.TempDir()
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", Content: "Goal complete: all tests pass"}},
	}}
	cfg := config.Default()
	cfg.Workspace = dir
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{Workspace: dir})
	gate := perm.New(config.ModeFast, dir, nil)
	a := agent.New(cfg, fake, reg, gate)
	_, res, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "done?"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.GoalDone {
		t.Fatal("expected GoalDone")
	}
}

func TestSystemPromptRefreshesTaskMode(t *testing.T) {
	dir := t.TempDir()
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", Content: "ok"}},
		{Message: llm.Message{Role: "assistant", Content: "plan"}},
	}}
	cfg := config.Default()
	cfg.Workspace = dir
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{Workspace: dir})
	gate := perm.New(config.ModeFast, dir, nil)
	a := agent.New(cfg, fake, reg, gate)

	hist, _, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "hi"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.Calls) < 1 || !strings.Contains(fake.Calls[0].Messages[0].Content, "Picogent") {
		t.Fatalf("first system missing: %+v", fake.Calls)
	}
	if strings.Contains(fake.Calls[0].Messages[0].Content, "PLAN MODE") {
		t.Fatal("first turn should not be plan")
	}

	a.SetTaskMode(agent.TaskPlan)
	_, _, err = a.Run(context.Background(), hist, llm.Message{Role: "user", Content: "plan a REST API"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.Calls) < 2 {
		t.Fatal("expected second chat call")
	}
	sys := fake.Calls[1].Messages[0]
	if sys.Role != "system" || !strings.Contains(sys.Content, "PLAN MODE") {
		t.Fatalf("second system should include PLAN MODE, got role=%s content=%q", sys.Role, sys.Content)
	}
	// Stale system from hist must not appear twice.
	sysCount := 0
	for _, m := range fake.Calls[1].Messages {
		if m.Role == "system" {
			sysCount++
		}
	}
	if sysCount != 1 {
		t.Fatalf("expected one system message, got %d", sysCount)
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
	_, _, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "hi"}, denyAll{})
	if err == nil || err.Error() == "" {
		t.Fatal("expected auth error")
	}
}

func (allowAll) OnNeedPermission(context.Context, perm.Request) (perm.Decision, error) {
	return perm.Allow, nil
}

type allowAll struct{ agent.NopHandler }

type denyAll struct{ agent.NopHandler }
