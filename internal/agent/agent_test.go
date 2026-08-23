package agent_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/evolve"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
)

func TestUpdateConfigKeepsToolWorkspaceInSync(t *testing.T) {
	oldWorkspace := t.TempDir()
	newWorkspace := t.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = oldWorkspace
	reg := tools.NewRegistry(tools.Context{Workspace: oldWorkspace})
	gate := perm.New(config.ModeSafe, oldWorkspace, nil)
	a := agent.New(cfg, &llm.Scripted{}, reg, gate)
	a.UpdateConfig(func(current *config.Config) {
		current.Workspace = newWorkspace
		current.BashTimeoutSec = 7
	})
	ctx := reg.ContextSnapshot()
	if ctx.Workspace != newWorkspace || ctx.BashTimeout != 7*time.Second {
		t.Fatalf("tool context = %+v, want workspace %q and 7s timeout", ctx, newWorkspace)
	}
	if got := gate.CloneForTurn().Workspace; got != newWorkspace {
		t.Fatalf("cloned gate workspace = %q, want %q", got, newWorkspace)
	}
}

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

func TestScopedTaskModeIsPerTurn(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = dir
	cfg.Provider = config.ProviderOllama
	cfg.TaskMode = string(agent.TaskAgent)
	fake := &llm.Scripted{}
	a := agent.New(cfg, fake, tools.NewRegistry(tools.Context{Workspace: dir}), perm.New(config.ModeFast, dir, nil))
	mode := agent.TaskPlan
	if _, _, err := a.RunWithOptions(context.Background(), nil, llm.Message{Role: "user", Content: "plan this"}, allowAll{}, agent.RunOptions{TaskMode: &mode, TracePrompt: "plan this"}); err != nil {
		t.Fatal(err)
	}
	if len(fake.Calls) != 1 || fake.Calls[0].TaskMode != string(agent.TaskPlan) || !fake.Calls[0].ReadOnly {
		t.Fatalf("scoped request = %#v", fake.Calls)
	}
	if got := a.TaskModeSnapshot(); got != agent.TaskAgent {
		t.Fatalf("scoped mode became sticky: %s", got)
	}
	if _, _, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "do it"}, allowAll{}); err != nil {
		t.Fatal(err)
	}
	if len(fake.Calls) != 2 || fake.Calls[1].TaskMode != string(agent.TaskAgent) || fake.Calls[1].ReadOnly {
		t.Fatalf("next request inherited scoped mode = %#v", fake.Calls)
	}
}

func TestScopedPromptKeepsDurableTaskGoalReadable(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = dir
	cfg.Provider = config.ProviderOllama
	store := taskstate.NewStore(t.TempDir())
	a := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "done"}}}}, tools.NewRegistry(tools.Context{Workspace: dir}), perm.New(config.ModeFast, dir, nil))
	a.TaskStore = store
	a.SetTaskSession("scoped-goal")
	mode := agent.TaskAgent
	internalPrompt := "build something\n\nPicogent scope choice: A small working version. Best default: keep the first pass focused."
	_, result, err := a.RunWithOptions(context.Background(), nil, llm.Message{Role: "user", Content: internalPrompt}, allowAll{}, agent.RunOptions{
		TaskMode:      &mode,
		TracePrompt:   "build something",
		DurablePrompt: "build something",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Task == nil || result.Task.Goal != "Build something" {
		t.Fatalf("durable task goal = %#v, want original prompt", result.Task)
	}
}

func TestTracePromptFallbackKeepsDurableTaskGoalReadable(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = dir
	cfg.Provider = config.ProviderOllama
	store := taskstate.NewStore(t.TempDir())
	a := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "done"}}}}, tools.NewRegistry(tools.Context{Workspace: dir}), perm.New(config.ModeFast, dir, nil))
	a.TaskStore = store
	a.SetTaskSession("trace-fallback")
	mode := agent.TaskPlan
	internalPrompt := "build something\n\nPicogent scope choice: A small working version. Best default: keep the first pass focused."
	_, result, err := a.RunWithOptions(context.Background(), nil, llm.Message{Role: "user", Content: internalPrompt}, allowAll{}, agent.RunOptions{
		TaskMode:    &mode,
		TracePrompt: "build something",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Task == nil || result.Task.Goal != "Build something" {
		t.Fatalf("durable task goal = %#v, want trace/display prompt", result.Task)
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
	if !strings.Contains(res.Text, "Changed:") || !strings.Contains(res.Text, "Undo:") {
		t.Fatalf("expected explain footer, got %q", res.Text)
	}
}

func TestFastAgentDoesNotAutoWriteThroughOutsideSymlink(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(target, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "escape")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("symlink creation requires privileges on Windows")
		}
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]string{"path": "escape/secret.txt", "content": "after"})
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "1", Name: "write_file", Arguments: string(args)}}}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}}
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Mode = config.ModeFast
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, fake, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	_, res, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update secret"}, denyAll{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "before" {
		t.Fatalf("outside target changed: %q", got)
	}
	if len(res.FilesChanged) != 0 {
		t.Fatalf("outside path recorded as changed: %+v", res.FilesChanged)
	}
}

func TestExplicitVerifyPopulatesResultEvidence(t *testing.T) {
	dir := t.TempDir()
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "1", Name: "verify", Arguments: `{}`}}}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}}
	reg := tools.NewRegistry(tools.Context{
		Workspace: dir,
		Verify:    func(context.Context) (string, error) { return "verify PASS\ngo test ./...", nil },
	})
	cfg := config.Default()
	cfg.Workspace = dir
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, fake, reg, perm.New(config.ModeFast, dir, nil))
	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "verify the auth flow"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Verified, "verify PASS") {
		t.Fatalf("explicit verify evidence missing from result: %q", result.Verified)
	}
}

func TestAutoVerifyAddsRelevantPlaybookTargets(t *testing.T) {
	dir := t.TempDir()
	args, _ := json.Marshal(map[string]string{"path": "internal/auth/auth.go", "content": "package auth\n"})
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "1", Name: "write_file", Arguments: string(args)}}}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}}
	var gotTargets []string
	reg := tools.NewRegistry(tools.Context{
		Workspace: dir,
		VerifyTargets: func(_ context.Context, targets []string) (string, error) {
			gotTargets = append([]string(nil), targets...)
			return "verify PASS\n12 passed", nil
		},
	})
	cfg := config.Default()
	cfg.Workspace = dir
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, fake, reg, perm.New(config.ModeFast, dir, nil))
	a.Memory = evolve.Store{Workspace: dir, Playbooks: []evolve.Playbook{{
		Title: "Auth changes", Class: "auth", Hits: 3,
		Body: "go test ./internal/auth then go test ./internal/session",
	}}}
	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "fix the auth token bug"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Verified, "verify PASS") {
		t.Fatalf("verification did not run: %q", result.Verified)
	}
	joined := strings.Join(gotTargets, "|")
	for _, want := range []string{"internal/auth/auth.go", "internal/auth", "internal/session"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("targets=%v missing learned target %q", gotTargets, want)
		}
	}
}

func TestExplainFooterInjectedWhenMissing(t *testing.T) {
	dir := t.TempDir()
	args, _ := json.Marshal(map[string]string{"path": "a.txt", "content": "x"})
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "1", Name: "write_file", Arguments: string(args)},
		}}},
		{Message: llm.Message{Role: "assistant", Content: "all set"}},
	}}
	cfg := config.Default()
	cfg.Workspace = dir
	cfg.Mode = config.ModeFast
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{Workspace: dir})
	gate := perm.New(config.ModeFast, dir, nil)
	a := agent.New(cfg, fake, reg, gate)
	_, res, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "write a.txt"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "Changed:") || !strings.Contains(res.Text, "Run:") || !strings.Contains(res.Text, "Undo:") {
		t.Fatalf("footer missing: %q", res.Text)
	}
	if !strings.Contains(res.Text, "a.txt") {
		t.Fatalf("path missing from footer: %q", res.Text)
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
	reg := tools.NewRegistry(tools.Context{
		Workspace: dir,
		Verify:    func(context.Context) (string, error) { return "verify PASS", nil },
	})
	gate := perm.New(config.ModeSafe, dir, nil)
	a := agent.New(cfg, fake, reg, gate)
	_, res, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "write x.txt"}, denyAll{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "x.txt")); !os.IsNotExist(err) {
		t.Fatal("file should not exist")
	}
	if len(res.FilesChanged) != 0 {
		t.Fatalf("denied write must not mark FilesChanged: %#v", res.FilesChanged)
	}
	if strings.Contains(res.Text, "Changed:") {
		t.Fatalf("denied write must not append Changed footer: %q", res.Text)
	}
	if res.Verified != "" {
		t.Fatalf("denied write must not auto-verify: %q", res.Verified)
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
