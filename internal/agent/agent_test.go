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

func TestNonDurableRunDoesNotInstallPublicationHook(t *testing.T) {
	dir := t.TempDir()
	args, _ := json.Marshal(map[string]string{"path": "hello.txt", "content": "picogent"})
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "1", Name: "write_file", Arguments: string(args)}}}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}}
	publishCalls := 0
	reg := tools.NewRegistry(tools.Context{
		Workspace: dir,
		BeforeWorkspacePublish: func(string, []byte, os.FileMode) error {
			publishCalls++
			return nil
		},
	})
	cfg := config.Default()
	cfg.Workspace = dir
	cfg.Mode = config.ModeFast
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, fake, reg, perm.New(config.ModeFast, dir, nil))
	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "create hello.txt"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if publishCalls != 0 {
		t.Fatalf("non-durable publication hook calls = %d, want 0", publishCalls)
	}
	if !result.UndoAvailable {
		t.Fatal("non-durable write lost process-local undo")
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

func TestScopedPromptKeepsDurableTaskGoalReadableAndPrioritizesTurnBoundary(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = dir
	cfg.Provider = config.ProviderOllama
	store := taskstate.NewStore(t.TempDir())
	fake := &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "done"}}}}
	a := agent.New(cfg, fake, tools.NewRegistry(tools.Context{Workspace: dir}), perm.New(config.ModeFast, dir, nil))
	a.TaskStore = store
	a.SetTaskSession("scoped-goal")
	mode := agent.TaskAgent
	internalPrompt := "fix all flaky tests and make CI green\n\nPicogent scope choice: A focused fix. Best default: keep the first pass focused."
	boundary := "For this turn, honor this scope boundary: A focused fix. This temporary boundary takes precedence over any broader active goal or durable task; do not expand beyond it unless the user explicitly asks."
	a.SetGoal("fix all flaky tests and make CI green")
	_, result, err := a.RunWithOptions(context.Background(), nil, llm.Message{Role: "user", Content: internalPrompt}, allowAll{}, agent.RunOptions{
		TaskMode:      &mode,
		TracePrompt:   "fix all flaky tests and make CI green",
		DurablePrompt: "fix all flaky tests and make CI green",
		ScopeBoundary: boundary,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Task == nil || result.Task.Goal != "Fix all flaky tests and make CI green" {
		t.Fatalf("durable task goal = %#v, want original prompt", result.Task)
	}
	if len(fake.Calls) != 1 {
		t.Fatal("scoped prompt did not reach the model")
	}
	var system string
	for _, msg := range fake.Calls[0].Messages {
		if msg.Role == "system" {
			system = msg.Content
			break
		}
	}
	goalAt := strings.Index(system, `task.goal: "Fix all flaky tests and make CI green"`)
	boundaryAt := strings.Index(system, boundary)
	if goalAt < 0 || boundaryAt <= goalAt {
		t.Fatalf("system prompt did not put turn boundary after durable goal: %q", system)
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

func TestProjectHealthAddsTransientOutcomeFocusToNextRound(t *testing.T) {
	dir := t.TempDir()
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "health", Name: "project_health", Arguments: `{}`}}}},
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "list", Name: "list_dir", Arguments: `{"path":"."}`}}}},
		{Message: llm.Message{Role: "assistant", Content: "I inspected the project."}},
	}}
	cfg := config.Default()
	cfg.Workspace = dir
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, fake, tools.NewRegistry(tools.Context{Workspace: dir}), perm.New(config.ModeFast, dir, nil))
	a.SetTaskStore(taskstate.NewStore(t.TempDir()))
	if err := a.SetTaskSession("health-focus"); err != nil {
		t.Fatal(err)
	}

	history, _, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "make this project ready"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.Calls) != 3 {
		t.Fatalf("model calls = %d, want 3", len(fake.Calls))
	}
	var focus string
	for _, message := range fake.Calls[1].Messages {
		if message.Role == "system" && strings.Contains(message.Content, "Internal outcome focus:") {
			focus = message.Content
			break
		}
	}
	for _, marker := range []string{
		"Internal outcome focus: bounded outcome contract",
		"Outcome state: DIAGNOSE",
		"Top obstacle categories: project-shape-unknown",
		"not user authorization",
	} {
		if !strings.Contains(focus, marker) {
			t.Fatalf("next-round focus missing %q: %q", marker, focus)
		}
	}
	if strings.Contains(focus, "transient advisory data") {
		t.Fatalf("next-round focus = %q", focus)
	}
	for _, message := range fake.Calls[2].Messages {
		if strings.Contains(message.Content, "Internal outcome focus:") {
			t.Fatalf("focus leaked beyond one model request: %q", message.Content)
		}
	}
	for _, message := range history {
		if strings.Contains(message.Content, "Internal outcome focus:") {
			t.Fatalf("transient focus leaked into returned history: %q", message.Content)
		}
	}
}

func TestProjectHealthFocusIsSkippedWhenCoBatchedWithWrite(t *testing.T) {
	dir := t.TempDir()
	writeArgs, _ := json.Marshal(map[string]string{"path": "created.txt", "content": "created"})
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "health", Name: "project_health", Arguments: `{}`},
			{ID: "write", Name: "write_file", Arguments: string(writeArgs)},
		}}},
		{Message: llm.Message{Role: "assistant", Content: "the file is written"}},
	}}
	cfg := config.Default()
	cfg.Workspace = dir
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, fake, tools.NewRegistry(tools.Context{Workspace: dir}), perm.New(config.ModeFast, dir, nil))
	a.SetTaskStore(taskstate.NewStore(t.TempDir()))
	if err := a.SetTaskSession("co-batched-focus"); err != nil {
		t.Fatal(err)
	}

	if _, _, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "create the file"}, allowAll{}); err != nil {
		t.Fatal(err)
	}
	if len(fake.Calls) != 2 {
		t.Fatalf("model calls = %d, want 2", len(fake.Calls))
	}
	var focus string
	for _, message := range fake.Calls[1].Messages {
		if strings.Contains(message.Content, "Internal outcome focus:") {
			focus = message.Content
		}
	}
	if strings.Contains(focus, "Outcome state: DIAGNOSE") || strings.Contains(focus, "Health observation: status=ATTENTION") {
		t.Fatalf("stale health focus was injected after a co-batched write: %q", focus)
	}
	for _, marker := range []string{
		"Outcome state: VERIFY",
		"Turn side effects data: changed_files=[\"created.txt\"] capped=false",
		"Health observation: status=UNKNOWN",
	} {
		if !strings.Contains(focus, marker) {
			t.Fatalf("post-write task-only focus missing %q: %q", marker, focus)
		}
	}
}

func TestDurableWriteAddsOutcomeFocusToNextRound(t *testing.T) {
	dir := t.TempDir()
	writeArgs, _ := json.Marshal(map[string]string{"path": "created.txt", "content": "created"})
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "write", Name: "write_file", Arguments: string(writeArgs)}}}},
		{Message: llm.Message{Role: "assistant", Content: "the file is written"}},
	}}
	cfg := config.Default()
	cfg.Workspace = dir
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, fake, tools.NewRegistry(tools.Context{Workspace: dir}), perm.New(config.ModeFast, dir, nil))
	a.SetTaskStore(taskstate.NewStore(t.TempDir()))
	if err := a.SetTaskSession("write-focus"); err != nil {
		t.Fatal(err)
	}

	history, _, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "create the file"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.Calls) != 2 {
		t.Fatalf("model calls = %d, want 2", len(fake.Calls))
	}
	var focus string
	for _, message := range fake.Calls[1].Messages {
		if message.Role == "system" && strings.Contains(message.Content, "Internal outcome focus:") {
			focus = message.Content
			break
		}
	}
	for _, marker := range []string{
		"Outcome state: VERIFY",
		"Completion proof ready: false",
		"Turn side effects data: changed_files=[\"created.txt\"] capped=false",
		"Health observation: status=UNKNOWN",
	} {
		if !strings.Contains(focus, marker) {
			t.Fatalf("post-write focus missing %q: %q", marker, focus)
		}
	}
	for _, message := range history {
		if strings.Contains(message.Content, "Internal outcome focus:") {
			t.Fatalf("durable focus leaked into returned history: %q", message.Content)
		}
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

func TestActiveGoalCompletionRejectsUnboundEvidenceWithoutChanges(t *testing.T) {
	dir := t.TempDir()
	checks := 0
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", Content: "Goal complete: the project is finished"}},
	}}
	cfg := config.Default()
	cfg.Workspace = dir
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{
		Workspace: dir,
		VerifyTargets: func(_ context.Context, targets []string) (string, error) {
			checks++
			if len(targets) != 0 {
				t.Fatalf("completion-only verification targets = %v, want none", targets)
			}
			return "verify PASS\nproject checks passed", nil
		},
	})
	a := agent.New(cfg, fake, reg, perm.New(config.ModeFast, dir, nil))
	a.SetGoal("finish this project")
	a.TaskStore = taskstate.NewStore(t.TempDir())
	a.SetTaskSession("active-goal-completion")

	_, res, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "finish this project"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if checks != 1 {
		t.Fatalf("completion verification calls = %d, want 1", checks)
	}
	if res.Task == nil || res.Task.Status != taskstate.StatusBlocked || !res.Task.NeedsVerification() {
		t.Fatalf("completion task = %#v, want blocked with inconclusive evidence", res.Task)
	}
	if len(res.Task.Verification) != 1 || res.Task.VerifiedChangeSeq != -1 || res.Task.Verification[0].Passed || !strings.HasPrefix(res.Task.Verification[0].Summary, "verify INCONCLUSIVE") {
		t.Fatalf("completion evidence = %#v", res.Task)
	}
	if res.GoalDone {
		t.Fatal("unbound completion evidence must not set GoalDone")
	}
}

func TestActiveGoalCompletionWithoutVerifierRemainsVerifying(t *testing.T) {
	dir := t.TempDir()
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", Content: "Goal complete: the project is finished"}},
	}}
	cfg := config.Default()
	cfg.Workspace = dir
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, fake, tools.NewRegistry(tools.Context{Workspace: dir}), perm.New(config.ModeFast, dir, nil))
	a.SetGoal("finish the project")
	a.TaskStore = taskstate.NewStore(t.TempDir())
	a.SetTaskSession("active-goal-no-verifier")

	_, res, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "finish the project"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Task == nil || res.Task.Status != taskstate.StatusVerifying || !res.Task.NeedsVerification() {
		t.Fatalf("completion task = %#v, want verifying/unverified", res.Task)
	}
	if res.GoalDone {
		t.Fatal("completion marker without evidence must not set GoalDone")
	}
}

func TestActiveGoalCompletionRequiresDurableTaskStart(t *testing.T) {
	workspace := t.TempDir()
	badRoot := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(badRoot, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", Content: "Goal complete: the project is finished"}},
	}}
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{
		Workspace: workspace,
		Verify: func(context.Context) (string, error) {
			return "verify PASS", nil
		},
	})
	a := agent.New(cfg, fake, reg, perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = taskstate.NewStore(badRoot)
	a.SetTaskSession("durable-start-failure")
	a.SetGoal("finish this project")

	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "finish this project"}, allowAll{})
	if err == nil {
		t.Fatal("run should fail closed when durable task initialization fails")
	}
	if result.GoalDone {
		t.Fatal("goal completed despite failed durable task initialization")
	}
	if len(fake.Calls) != 0 {
		t.Fatalf("provider calls = %d, want 0", len(fake.Calls))
	}
}

func TestActiveGoalCompletionRefreshesEvidenceAfterLaterWriteWithoutTaskStore(t *testing.T) {
	dir := t.TempDir()
	writeArgs, _ := json.Marshal(map[string]string{"path": "later.txt", "content": "updated"})
	checks := 0
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "verify-before", Name: "verify", Arguments: `{}`}}}},
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "write", Name: "write_file", Arguments: string(writeArgs)}}}},
		{Message: llm.Message{Role: "assistant", Content: "Goal complete: the project is finished"}},
	}}
	cfg := config.Default()
	cfg.Workspace = dir
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{
		Workspace: dir,
		VerifyTargets: func(_ context.Context, _ []string) (string, error) {
			checks++
			return "verify PASS\nchecks passed", nil
		},
	})
	a := agent.New(cfg, fake, reg, perm.New(config.ModeFast, dir, nil))
	a.SetGoal("finish this project")

	_, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "finish this project"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if checks != 2 {
		t.Fatalf("verification calls = %d, want explicit evidence plus a fresh completion check", checks)
	}
	if !result.GoalDone {
		t.Fatal("completion should require and receive fresh post-write evidence")
	}
}

func TestScopedCompletionCannotRetireBroaderActiveGoal(t *testing.T) {
	dir := t.TempDir()
	fake := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", Content: "Goal complete: the focused pass is finished"}},
	}}
	cfg := config.Default()
	cfg.Workspace = dir
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{
		Workspace: dir,
		Verify: func(context.Context) (string, error) {
			return "verify PASS\nfocused checks passed", nil
		},
	})
	a := agent.New(cfg, fake, reg, perm.New(config.ModeFast, dir, nil))
	a.SetGoal("finish this project")
	mode := agent.TaskPlan
	_, result, err := a.RunWithOptions(context.Background(), nil, llm.Message{Role: "user", Content: "finish this project"}, allowAll{}, agent.RunOptions{
		TaskMode:      &mode,
		ScopeBoundary: "For this turn, complete only the focused first pass.",
		DurablePrompt: "finish this project",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.GoalDone {
		t.Fatal("a scoped first pass must not retire the broader active goal")
	}
	if got := a.GoalSnapshot(); got != "finish this project" {
		t.Fatalf("active goal = %q, want retained broad goal", got)
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
