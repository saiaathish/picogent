package main

import (
	"bufio"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
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
