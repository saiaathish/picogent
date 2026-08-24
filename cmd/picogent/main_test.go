package main

import (
	"bufio"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/goal"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/scope"
	"github.com/saiaathish/picogent/internal/tools"
)

func TestVersion(t *testing.T) {
	if err := run([]string{"version"}); err != nil {
		t.Fatal(err)
	}
}

func TestChooseScopeUsesNumberedChoiceAndDefault(t *testing.T) {
	p, ok := scope.Analyze("build something")
	if !ok {
		t.Fatal("expected scope prompt")
	}
	choice, proceed := chooseScope(p, bufio.NewReader(strings.NewReader("2\n")))
	if !proceed || choice.ID != "full" {
		t.Fatalf("choice = %#v, proceed=%v", choice, proceed)
	}
	choice, proceed = chooseScope(p, bufio.NewReader(strings.NewReader("\n")))
	if !proceed || choice.ID != "small" {
		t.Fatalf("default choice = %#v, proceed=%v", choice, proceed)
	}
}

func TestAutomaticRecommendedScopePreservesTaskIntent(t *testing.T) {
	cfg := config.Default()
	cfg.Workspace = t.TempDir()
	a := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: cfg.Workspace}), perm.New(config.ModeFast, cfg.Workspace, nil))
	p, ok := scope.Analyze("build something")
	if !ok {
		t.Fatal("expected scope prompt")
	}

	a.SetTaskMode(agent.TaskPlan)
	if got := scopeModeForHeadlessTurn(a, cfg, "build something", scope.Recommended(p), false); got != nil {
		t.Fatalf("automatic scope overrode existing task mode: %q", *got)
	}
	if got := scopeModeForHeadlessTurn(a, cfg, "create something", scope.Recommended(p), false); got != nil {
		t.Fatalf("automatic scope escaped plan mode: %q", *got)
	}

	a.SetTaskMode(agent.TaskAgent)
	if got := scopeModeForHeadlessTurn(a, cfg, "build something, but plan it first", scope.Recommended(p), false); got == nil || *got != agent.TaskPlan {
		t.Fatalf("automatic scope mode = %v, want temporary plan", got)
	}
	if got := scopeModeForHeadlessTurn(a, cfg, "build something, but inspect and report first", scope.Recommended(p), false); got == nil || *got != agent.TaskAsk {
		t.Fatalf("automatic scope mode = %v, want temporary ask", got)
	}

	plan := scope.Choice{ID: "plan"}
	if got := scopeModeForHeadlessTurn(a, cfg, "build something", plan, true); got == nil || *got != agent.TaskPlan {
		t.Fatalf("explicit plan scope mode = %v, want temporary plan", got)
	}
}

func TestHeadlessCompletionInferenceExitsSavedPlanModeForThisTurn(t *testing.T) {
	cfg := config.Default()
	cfg.Workspace = t.TempDir()
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: cfg.Workspace}), perm.New(config.ModeFast, cfg.Workspace, nil))
	a.SetTaskMode(agent.TaskPlan)
	mode := autoModeForHeadlessTurn(a, cfg, "finish this project")
	if mode == nil || *mode != agent.TaskAgent {
		t.Fatalf("automatic headless mode = %v, want agent", mode)
	}
	if got := a.TaskModeSnapshot(); got != agent.TaskPlan {
		t.Fatalf("headless inference changed saved live mode to %q", got)
	}
}

func TestRunRequiresPrompt(t *testing.T) {
	err := run([]string{"run"})
	if err == nil || !strings.Contains(err.Error(), "missing prompt") {
		t.Fatalf("%v", err)
	}
}

func TestRunLoginUsesProvidedTarget(t *testing.T) {
	err := run([]string{"login", "not-a-provider"})
	if err == nil || !strings.Contains(err.Error(), `unknown login target "not-a-provider"`) {
		t.Fatalf("login error = %v", err)
	}
}

func TestHeadlessTaskSessionIDIsStableAndSafe(t *testing.T) {
	a := headlessTaskSessionID("  fix the login flow  ")
	b := headlessTaskSessionID("fix the login flow")
	if a != b {
		t.Fatalf("equivalent prompts got different session ids: %q vs %q", a, b)
	}
	if len(a) != len("headless-")+16 || strings.ContainsAny(a, "/\\. ") {
		t.Fatalf("session id is not safe for task storage: %q", a)
	}
	if a == headlessTaskSessionID("fix the logout flow") {
		t.Fatal("different prompts shared a headless task session")
	}
}

func TestHeadlessPermissionDenialUsesExitCodeTwo(t *testing.T) {
	if got := exitCode(errHeadlessPermissionDenied); got != 2 {
		t.Fatalf("permission exit code = %d, want 2", got)
	}
	if got := exitCode(errors.New("provider failed")); got != 1 {
		t.Fatalf("provider exit code = %d, want 1", got)
	}
}

func TestStdioPermissionDenialFailsClosedOnEOF(t *testing.T) {
	h := &stdioHandler{in: bufio.NewReader(strings.NewReader(""))}
	decision, err := h.OnNeedPermission(context.Background(), perm.Request{Summary: "write file"})
	if decision != perm.Deny || !errors.Is(err, errHeadlessPermissionDenied) {
		t.Fatalf("EOF permission result = %s, %v; want deny/exit-2 error", decision, err)
	}
}

func TestStdioYesStillBlocksRiskyActions(t *testing.T) {
	h := &stdioHandler{yes: true, in: bufio.NewReader(strings.NewReader(""))}
	decision, err := h.OnNeedPermission(context.Background(), perm.Request{Summary: "delete file", Destructive: true})
	if decision != perm.Deny || !errors.Is(err, errHeadlessPermissionDenied) {
		t.Fatalf("risky --yes result = %s, %v; want deny/exit-2 error", decision, err)
	}
}

func TestHeadlessYesOverridesOnlyThisProcess(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)
	t.Setenv("PICOGENT_CODEX_HOME", t.TempDir())
	t.Setenv("PICOGENT_MODE", "")
	t.Chdir(t.TempDir())

	user := config.Default()
	user.Mode = config.ModeSafe
	user.Provider = config.ProviderOllama
	user.Workspace = workspace
	if err := config.Save(user); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PICOGENT_MODE", "safe")
	effective, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	gate := perm.New(effective.Mode, workspace, nil)
	a := agent.New(effective, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), gate)
	applyHeadlessYes(&effective, a)
	if effective.Mode != config.ModeFast || effective.PersistentMode() != config.ModeSafe {
		t.Fatalf("headless config effective=%q persistent=%q, want fast/safe", effective.Mode, effective.PersistentMode())
	}
	if got := a.ConfigSnapshot().Mode; got != config.ModeFast {
		t.Fatalf("agent mode = %q, want fast", got)
	}
	if gate.Mode != config.ModeFast {
		t.Fatalf("gate mode = %q, want fast", gate.Mode)
	}
	if err := config.Save(effective); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PICOGENT_MODE", "")
	reloaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Mode != config.ModeSafe {
		t.Fatalf("saved mode = %q, want original safe mode", reloaded.Mode)
	}
}

func TestHeadlessPersistsExplicitCompletionGoalBeforeRun(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	a := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))

	if err := applyHeadlessGoalInference(a, cfg, "finish this project"); err != nil {
		t.Fatal(err)
	}
	if got := a.GoalSnapshot(); got != "finish this project" {
		t.Fatalf("agent goal = %q, want persisted completion intent", got)
	}
	if got, err := goal.Load(workspace); err != nil || got != "finish this project" {
		t.Fatalf("stored goal = %q, err=%v", got, err)
	}
}

func TestHeadlessClarifyCancelDoesNotPersistInferredGoal(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)
	t.Setenv("PICOGENT_CODEX_HOME", t.TempDir())
	t.Setenv("PICOGENT_MODE", "")
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	oldStdin := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.WriteString("esc\n"); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	os.Stdin = reader
	defer func() {
		_ = reader.Close()
		os.Stdin = oldStdin
	}()

	if err := run([]string{"run", "--clarify", "--dir", workspace, "fix all flaky tests and make CI green"}); err != nil {
		t.Fatal(err)
	}
	if got, err := goal.Load(workspace); err != nil || got != "" {
		t.Fatalf("canceled headless scope persisted goal %q, err=%v", got, err)
	}
}

func TestHeadlessClearsGoalOnlyAfterVerifiedCompletion(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	a := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	if err := goal.Set(workspace, "finish the project"); err != nil {
		t.Fatal(err)
	}
	state, _ := goal.LoadState(workspace)
	a.SetGoalState(state.Text, state.Revision)

	if err := clearHeadlessGoalAfterCompletion(a, cfg, "finish the project", state.Revision, agent.Result{}); err != nil {
		t.Fatal(err)
	}
	if got, _ := goal.Load(workspace); got != "finish the project" {
		t.Fatalf("unverified goal was cleared: %q", got)
	}
	if err := goal.Set(workspace, "newer project goal"); err != nil {
		t.Fatal(err)
	}
	newer, _ := goal.LoadState(workspace)
	a.SetGoalState(newer.Text, newer.Revision)
	if err := clearHeadlessGoalAfterCompletion(a, cfg, "finish the project", state.Revision, agent.Result{GoalDone: true}); err != nil {
		t.Fatal(err)
	}
	if got, _ := goal.Load(workspace); got != "newer project goal" || a.GoalSnapshot() != "newer project goal" {
		t.Fatalf("stale completion erased newer goal: stored=%q agent=%q", got, a.GoalSnapshot())
	}
	if err := goal.Set(workspace, "finish the project"); err != nil {
		t.Fatal(err)
	}
	state, _ = goal.LoadState(workspace)
	a.SetGoalState(state.Text, state.Revision)

	if err := clearHeadlessGoalAfterCompletion(a, cfg, "finish the project", state.Revision, agent.Result{GoalDone: true}); err != nil {
		t.Fatal(err)
	}
	if got, _ := goal.Load(workspace); got != "" || a.GoalSnapshot() != "" {
		t.Fatalf("verified goal was not cleared: stored=%q agent=%q", got, a.GoalSnapshot())
	}
}
