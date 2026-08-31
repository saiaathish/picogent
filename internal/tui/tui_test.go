package tui

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/goal"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/mcpbridge"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/session"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
)

func TestInitialSessionResumesLatestSavedSession(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	if err := session.SaveMessages(workspace, "resume-me", []llm.Message{{Role: "user", Content: "continue"}}); err != nil {
		t.Fatal(err)
	}
	id, history, resumed := initialSession(workspace)
	if !resumed || id != "resume-me" || len(history) != 1 || history[0].Content != "continue" {
		t.Fatalf("initial session=(%q, %#v, %v)", id, history, resumed)
	}
}

func TestNewModelSurfacesDurableTaskFailure(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	const sessionID = "corrupt-session"
	if err := session.SaveMessages(workspace, sessionID, []llm.Message{{Role: "user", Content: "resume me"}}); err != nil {
		t.Fatal(err)
	}
	store := taskstate.NewStore(t.TempDir())
	path, err := store.Path(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	a := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = store

	m, err := newModel(cfg, a)
	if err == nil || !strings.Contains(err.Error(), "load durable task state") {
		t.Fatalf("newModel error = %v, want durable-load failure", err)
	}
	if m != nil {
		t.Fatalf("newModel returned a model after durable-load failure: %#v", m)
	}
}

func TestResumeDoesNotClaimSuccessWhenDurableTaskLoadFails(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	const targetSession = "target-session"
	if err := session.SaveMessages(workspace, targetSession, []llm.Message{{Role: "user", Content: "target"}}); err != nil {
		t.Fatal(err)
	}
	store := taskstate.NewStore(t.TempDir())
	path, err := store.Path(targetSession)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	a := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = store
	const currentSession = "current-session"
	if err := a.SetTaskSession(currentSession); err != nil {
		t.Fatal(err)
	}
	m := &model{
		cfg:       cfg,
		ag:        a,
		sessionID: currentSession,
		history:   []llm.Message{{Role: "user", Content: "current"}},
		lines:     []logLine{{Kind: "system", Text: "ready"}},
		vp:        viewport.New(80, 20),
	}

	m.slashLocal("resume")
	last := m.lines[len(m.lines)-1]
	if last.Kind != "error" || !strings.Contains(last.Text, "couldn't resume session") {
		t.Fatalf("resume result = %#v, want visible durable-load error", last)
	}
	for _, line := range m.lines {
		if strings.Contains(line.Text, "resumed ") {
			t.Fatalf("resume success message was published: %#v", m.lines)
		}
	}
	if m.sessionID != currentSession || len(m.history) != 1 || m.history[0].Content != "current" || a.TaskSession != currentSession {
		t.Fatalf("failed resume changed current session: model=%q history=%#v agent=%q", m.sessionID, m.history, a.TaskSession)
	}
}

func TestAssistantFinalReplacesStreamedText(t *testing.T) {
	m := &model{lines: []logLine{{Kind: "assistant", Text: "Undo: git checkout -- note.txt"}}}
	_, _ = m.Update(logMsg{Kind: "assistant_final", Text: "Undo: /undo"})
	if len(m.lines) != 1 || m.lines[0].Text != "Undo: /undo" {
		t.Fatalf("lines = %#v", m.lines)
	}
}

func TestUndoRefreshesCurrentTaskSnapshot(t *testing.T) {
	workspace := t.TempDir()
	store := taskstate.NewStore(t.TempDir())
	const sessionID = "session-undo-tui"
	cfg := config.Default()
	cfg.Mode = config.ModeFast
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	args, err := json.Marshal(map[string]string{"path": "note.txt", "content": "after"})
	if err != nil {
		t.Fatal(err)
	}
	gate := perm.New(config.ModeFast, workspace, nil)
	gate.AddAlwaysAllowed("verify")
	ag := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "write", Name: "write_file", Arguments: string(args)}}}},
		{Message: llm.Message{Role: "assistant", Content: "Goal complete: note updated"}},
	}}, tools.NewRegistry(tools.Context{
		Workspace: workspace,
		Verify:    func(context.Context) (string, error) { return "verify PASS\n1 passed", nil },
	}), gate)
	ag.SetTaskStore(store)
	if err := ag.SetTaskSession(sessionID); err != nil {
		t.Fatal(err)
	}
	_, result, err := ag.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update note.txt"}, agent.NopHandler{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Task == nil || result.Task.Status != taskstate.StatusDone {
		t.Fatalf("pre-undo task = %#v, want done", result.Task)
	}

	m := &model{
		cfg:       cfg,
		ag:        ag,
		sessionID: sessionID,
		task:      result.Task,
		lines:     []logLine{{Kind: "system", Text: "ready"}},
		vp:        viewport.New(80, 20),
	}
	m.slashLocal("undo")
	if m.task == nil || m.task.SessionID != sessionID {
		t.Fatalf("undo task = %#v, want current session task", m.task)
	}
	if m.task.Status != taskstate.StatusVerifying {
		t.Fatalf("undo task status = %q, want %q", m.task.Status, taskstate.StatusVerifying)
	}
	if _, err := os.Stat(filepath.Join(workspace, "note.txt")); !os.IsNotExist(err) {
		t.Fatalf("undo did not remove note.txt: %v", err)
	}
}

func TestDoneReportsSessionSaveFailure(t *testing.T) {
	home := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(home, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PICOGENT_HOME", home)

	workspace := t.TempDir()
	m := &model{
		cfg:       config.Config{Workspace: workspace},
		sessionID: "save-failure",
		lines:     []logLine{{Kind: "user", Text: "request"}},
		vp:        viewport.New(80, 20),
	}
	_, _ = m.Update(doneMsg{history: []llm.Message{{Role: "user", Content: "request"}}})
	if len(m.lines) == 0 || m.lines[len(m.lines)-1].Kind != "error" || !strings.Contains(m.lines[len(m.lines)-1].Text, "couldn't save session") {
		t.Fatalf("done result = %#v, want visible session-save error", m.lines)
	}
}

func TestBroadPromptStartsWithRecommendedScope(t *testing.T) {
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = workspace
	a := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	m := &model{
		cfg:       cfg,
		ag:        a,
		lines:     []logLine{{Kind: "system", Text: "ready"}},
		vp:        viewport.New(80, 20),
		h:         &handler{permCh: make(chan perm.Decision, 1)},
		sessionID: "scope-session",
	}
	if cmd := m.submit("build something"); cmd == nil {
		t.Fatal("broad prompt did not start")
	}
	if !m.busy {
		t.Fatal("automatic scope did not mark the model busy")
	}
	if m.turnMode != nil {
		t.Fatalf("automatic scope installed temporary task mode %q", *m.turnMode)
	}
	if len(m.lines) < 3 || m.lines[len(m.lines)-2].Kind != "system" || m.lines[len(m.lines)-2].Text != "Starting with a small working version by default." {
		t.Fatalf("scope notice = %#v", m.lines)
	}
	if got := m.lines[len(m.lines)-1]; got.Kind != "user" || got.Text != "build something" {
		t.Fatalf("user line = %#v", got)
	}
}

func TestAutomaticScopePrioritizesFocusedTurnOverDurableGoal(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	fake := &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "done"}}}}
	a := agent.New(cfg, fake, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	m := &model{cfg: cfg, ag: a, vp: viewport.New(80, 20), h: &handler{permCh: make(chan perm.Decision, 1)}, sessionID: "scope-boundary"}
	const broadGoal = "fix all flaky tests and make CI green"
	cmd := m.submit(broadGoal)
	if cmd == nil {
		t.Fatal("automatic scope did not start")
	}
	msg := cmd()
	done, ok := msg.(doneMsg)
	if !ok {
		t.Fatalf("automatic scoped turn result = %T, want doneMsg", msg)
	}
	if done.err != nil {
		t.Fatalf("automatic scoped turn failed: %v", done.err)
	}
	if got := a.GoalSnapshot(); got != broadGoal {
		t.Fatalf("automatic scope goal = %q, want %q", got, broadGoal)
	}
	if got, _ := goal.Load(workspace); got != broadGoal {
		t.Fatalf("automatic scope saved goal = %q, want %q", got, broadGoal)
	}
	if len(fake.Calls) == 0 {
		t.Fatal("automatic scoped turn did not reach the model")
	}
	var system string
	for _, message := range fake.Calls[0].Messages {
		if message.Role == "system" {
			system = message.Content
			break
		}
	}
	goalAt := strings.Index(system, broadGoal)
	boundaryAt := strings.Index(system, "Current turn scope (takes precedence over active and durable goals):")
	if goalAt < 0 || boundaryAt <= goalAt {
		t.Fatalf("system prompt did not prioritize turn boundary: %q", system)
	}
}

func TestCompletionPromptReachesAgentWithoutAutomaticScope(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	fake := &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "done"}}}}
	a := agent.New(cfg, fake, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	m := &model{cfg: cfg, ag: a, vp: viewport.New(80, 20), h: &handler{permCh: make(chan perm.Decision, 1)}, sessionID: "completion-without-scope"}
	const prompt = "finish this project"

	cmd := m.submit(prompt)
	if cmd == nil {
		t.Fatal("completion prompt did not start")
	}
	if len(m.lines) != 1 || m.lines[0].Kind != "user" || m.lines[0].Text != prompt {
		t.Fatalf("completion prompt lines = %#v, want only the unchanged user prompt", m.lines)
	}
	msg := cmd()
	done, ok := msg.(doneMsg)
	if !ok {
		t.Fatalf("completion prompt result = %T, want doneMsg", msg)
	}
	if done.err != nil {
		t.Fatalf("completion prompt failed: %v", done.err)
	}
	if done.result.GoalDone {
		t.Fatal("unmarked completion must not clear the active goal")
	}
	if len(fake.Calls) != 1 {
		t.Fatalf("completion prompt calls = %d, want 1", len(fake.Calls))
	}
	if got := a.GoalSnapshot(); got != prompt {
		t.Fatalf("completion prompt goal = %q, want persisted %q", got, prompt)
	}
	if got, err := goal.Load(workspace); err != nil || got != prompt {
		t.Fatalf("completion prompt stored goal = %q, err=%v", got, err)
	}

	var system, user string
	for _, message := range fake.Calls[0].Messages {
		switch message.Role {
		case "system":
			system = message.Content
		case "user":
			user = message.Content
		}
	}
	if user != prompt {
		t.Fatalf("completion prompt sent to model = %q, want unchanged %q", user, prompt)
	}
	if strings.Contains(system, "Current turn scope (takes precedence over active and durable goals):") {
		t.Fatalf("completion prompt installed an automatic scope boundary: %q", system)
	}
}

func TestStaleCompletedTurnCannotClearNewerGoal(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	ag := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	ag.SetGoal("newer project goal")
	if err := goal.Set(workspace, "newer project goal"); err != nil {
		t.Fatal(err)
	}
	m := &model{cfg: cfg, ag: ag, turnID: 2, sessionID: "new-session", vp: viewport.New(80, 20)}
	_, _ = m.Update(doneMsg{result: agent.Result{GoalDone: true}, goal: "finish this project", turnID: 1, sessionID: "old-session"})
	if got, _ := goal.Load(workspace); got != "newer project goal" || ag.GoalSnapshot() != "newer project goal" {
		t.Fatalf("stale completion erased newer goal: stored=%q agent=%q", got, ag.GoalSnapshot())
	}
}

func TestUnprovenCompletionUsesSharedProjection(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	ag := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	revision, err := goal.SetState(workspace, "finish this project")
	if err != nil {
		t.Fatal(err)
	}
	ag.SetGoalState("finish this project", revision)
	m := &model{cfg: cfg, ag: ag, turnID: 1, sessionID: "session-1", vp: viewport.New(80, 20)}
	const reason = "required criterion evidence is incomplete"
	_, _ = m.Update(doneMsg{
		result: agent.Result{Completion: agent.CompletionProjection{
			Required: true,
			Marker:   true,
			Reason:   reason,
		}},
		goal:         "finish this project",
		goalRevision: revision,
		turnID:       1,
		sessionID:    "session-1",
	})
	if got, _ := goal.Load(workspace); got != "finish this project" {
		t.Fatalf("unproven completion cleared goal: %q", got)
	}
	if len(m.lines) == 0 || m.lines[len(m.lines)-1].Kind != "system" || !strings.Contains(m.lines[len(m.lines)-1].Text, reason) {
		t.Fatalf("shared completion explanation was not visible: %#v", m.lines)
	}
}

func TestGoalSlashFailsClosedWhenDurableWriteFails(t *testing.T) {
	home := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(home, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PICOGENT_HOME", home)
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = workspace
	ag := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	ag.SetGoalState("old durable goal", 1)
	m := &model{cfg: cfg, ag: ag, lines: []logLine{}, vp: viewport.New(80, 20)}
	m.slashLocal("goal:set:new durable goal")
	if got, _ := ag.GoalStateSnapshot(); got != "old durable goal" {
		t.Fatalf("failed goal write changed memory to %q", got)
	}
	if len(m.lines) == 0 || m.lines[len(m.lines)-1].Kind != "error" {
		t.Fatalf("failed goal write did not report an error: %#v", m.lines)
	}
	m.slashLocal("goal:clear")
	if got, _ := ag.GoalStateSnapshot(); got != "old durable goal" {
		t.Fatalf("failed goal clear changed memory to %q", got)
	}
}

func TestCompletedGoalClearFailureIsVisibleAndKeepsGoal(t *testing.T) {
	home := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(home, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PICOGENT_HOME", home)
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	ag := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	ag.SetGoalState("finish this project", 1)
	m := &model{cfg: cfg, ag: ag, sessionID: "session-1"}
	_, _ = m.Update(doneMsg{result: agent.Result{GoalDone: true}, goal: "finish this project", goalRevision: 1})
	if got, _ := ag.GoalStateSnapshot(); got != "finish this project" {
		t.Fatalf("failed clear changed in-memory goal to %q", got)
	}
	if len(m.lines) == 0 || m.lines[len(m.lines)-1].Kind != "error" || !strings.Contains(m.lines[len(m.lines)-1].Text, "couldn't clear completed goal") {
		t.Fatalf("clear failure was not visible: %#v", m.lines)
	}
}

func TestAutomaticScopeKeepsPlanAndReportIntent(t *testing.T) {
	for _, tt := range []struct {
		prompt string
		want   agent.TaskMode
	}{
		{"build something, but plan it first", agent.TaskPlan},
		{"build something, but inspect and report first", agent.TaskAsk},
	} {
		t.Run(tt.want.Label(), func(t *testing.T) {
			workspace := t.TempDir()
			cfg := config.Default()
			cfg.Workspace = workspace
			a := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
			m := &model{cfg: cfg, ag: a, vp: viewport.New(80, 20), h: &handler{permCh: make(chan perm.Decision, 1)}, sessionID: "scope-intent"}
			if cmd := m.submit(tt.prompt); cmd == nil {
				t.Fatal("automatic scope did not start")
			}
			if m.turnMode != nil {
				t.Fatalf("automatic scope installed temporary task mode %q", *m.turnMode)
			}
			if got := a.TaskModeSnapshot(); got != tt.want {
				t.Fatalf("task mode = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAutomaticScopePreservesSelectedTaskBoundary(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	for _, tt := range []struct {
		prompt string
		mode   agent.TaskMode
	}{
		{"create something", agent.TaskPlan},
		{"fix everything", agent.TaskAsk},
		{"remove everything", agent.TaskDebug},
	} {
		t.Run(tt.mode.Label(), func(t *testing.T) {
			workspace := t.TempDir()
			cfg := config.Default()
			cfg.Workspace = workspace
			a := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
			a.SetTaskMode(tt.mode)
			m := &model{cfg: cfg, ag: a, vp: viewport.New(80, 20), h: &handler{permCh: make(chan perm.Decision, 1)}, sessionID: "scope-boundary"}
			if cmd := m.submit(tt.prompt); cmd == nil {
				t.Fatal("automatic scope did not start")
			}
			if got := a.TaskModeSnapshot(); got != tt.mode {
				t.Fatalf("task mode = %q, want preserved %q", got, tt.mode)
			}
		})
	}
}

func TestModeSlashPersistsPreferenceWithoutOverridingRuntimeMode(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)
	t.Setenv("PICOGENT_CODEX_HOME", t.TempDir())
	t.Setenv("PICOGENT_MODE", "")

	user := config.Default()
	user.Mode = config.ModeFast
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
	m := &model{cfg: effective, ag: a}

	m.slash("/fast")
	if got := m.lines[len(m.lines)-1].Text; !strings.Contains(got, "current run stays safe (PICOGENT_MODE)") {
		t.Fatalf("mode confirmation = %q, want active environment override disclosure", got)
	}
	if m.cfg.Mode != config.ModeSafe {
		t.Fatalf("model effective mode = %q, want environment mode %q", m.cfg.Mode, config.ModeSafe)
	}
	if got := a.ConfigSnapshot().Mode; got != config.ModeSafe {
		t.Fatalf("agent effective mode = %q, want environment mode %q", got, config.ModeSafe)
	}
	if gate.Mode != config.ModeSafe {
		t.Fatalf("gate mode = %q, want environment mode %q", gate.Mode, config.ModeSafe)
	}

	m.slash("/safe")
	m.cfg.Model = "saved-after-mode-choice"
	if err := config.Save(m.cfg); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PICOGENT_MODE", "")
	reloaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Mode != config.ModeSafe {
		t.Fatalf("saved mode = %q, want deliberate user choice %q", reloaded.Mode, config.ModeSafe)
	}
	if reloaded.Model != "saved-after-mode-choice" {
		t.Fatalf("saved model = %q, want unrelated setting to persist", reloaded.Model)
	}
}

func TestModeSlashDoesNotReportOrApplyUnsavedChange(t *testing.T) {
	for _, tc := range []struct {
		name       string
		persistent config.Mode
		runtime    config.Mode
		command    string
	}{
		{name: "fast saved beneath safe runtime", persistent: config.ModeFast, runtime: config.ModeSafe, command: "/safe"},
		{name: "safe saved beneath fast runtime", persistent: config.ModeSafe, runtime: config.ModeFast, command: "/fast"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := filepath.Join(t.TempDir(), "not-a-directory")
			if err := os.WriteFile(home, []byte("not a directory"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PICOGENT_HOME", home)

			workspace := t.TempDir()
			cfg := config.Default()
			cfg.Mode = tc.persistent
			cfg.Workspace = workspace
			cfg.Provider = config.ProviderOllama
			cfg.SetRuntimeMode(tc.runtime)
			gate := perm.New(cfg.Mode, workspace, nil)
			ag := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), gate)
			m := &model{cfg: cfg, ag: ag}

			m.slash(tc.command)
			if got := m.lines[len(m.lines)-1]; got.Kind != "error" || !strings.Contains(got.Text, "couldn't save mode") {
				t.Fatalf("mode result = %#v, want save failure", got)
			}
			if m.cfg.Mode != tc.runtime || m.cfg.PersistentMode() != tc.persistent {
				t.Fatalf("model config changed after failed save: %#v", m.cfg)
			}
			if got := ag.ConfigSnapshot(); got.Mode != tc.runtime || got.PersistentMode() != tc.persistent {
				t.Fatalf("agent config changed after failed save: %#v", got)
			}
			if gate.Mode != tc.runtime {
				t.Fatalf("gate mode changed after failed save: %q", gate.Mode)
			}
		})
	}
}

func TestFormatTaskProgress(t *testing.T) {
	tests := []struct {
		name string
		task *taskstate.Task
		want string
	}{
		{name: "none", want: ""},
		{
			name: "working",
			task: &taskstate.Task{
				Status:       taskstate.StatusWorking,
				Steps:        []taskstate.Step{{Description: "Map repository", Done: true}, {Description: "Implement UI"}},
				CurrentStep:  1,
				ChangedFiles: []string{"one.go"},
			},
			want: "task · working · 1/2 · Implement UI · proof pending: required criterion evidence is incomplete · 1 file",
		},
		{
			name: "blocked",
			task: &taskstate.Task{
				Status:       taskstate.StatusBlocked,
				Steps:        []taskstate.Step{{Description: "Implement UI"}},
				BlockedBy:    "verification repeatedly failed",
				ChangedFiles: []string{"one.go", "two.go"},
			},
			want: "task · blocked · 0/1 · blocked: verification repeatedly failed · proof pending: durable task is blocked · 2 files",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatTaskProgress(tt.task); got != tt.want {
				t.Fatalf("formatTaskProgress() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatTaskProgressDistinguishesReadyProof(t *testing.T) {
	task := &taskstate.Task{
		Status:           taskstate.StatusDone,
		DefinitionOfDone: []taskstate.Criterion{{Description: "required proof", Required: true}},
	}
	task.RecordCriterionVerification(0, "PASS", "criterion passed", "verify")
	if got := formatTaskProgress(task); !strings.Contains(got, "proof ready") {
		t.Fatalf("formatTaskProgress() = %q, want ready proof", got)
	}
}

func TestTaskProgressMessageCarriesSharedCompletionProof(t *testing.T) {
	task, err := taskstate.New("session-proof", "finish the loop", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.DefinitionOfDone = []taskstate.Criterion{{Description: "required proof", Required: true}}
	var got tea.Msg
	h := &handler{send: func(msg tea.Msg) { got = msg }}
	h.OnTaskState(task)
	msg, ok := got.(taskProgressMsg)
	if !ok {
		t.Fatalf("message = %T, want taskProgressMsg", got)
	}
	want := agent.CompletionProof(task)
	if !reflect.DeepEqual(msg.completion, want) {
		t.Fatalf("message proof = %#v, want %#v", msg.completion, want)
	}
}

func TestAcceptedTaskProgressStoresCompletionProof(t *testing.T) {
	task := &taskstate.Task{SessionID: "session-current", Status: taskstate.StatusWorking, Goal: "finish the loop"}
	proof := agent.CompletionProof(task)
	m := &model{
		sessionID: "session-current",
		turnID:    4,
		task:      &taskstate.Task{SessionID: "session-current", Goal: "old task"},
		vp:        viewport.New(80, 20),
		lines:     []logLine{{Kind: "system", Text: "current"}},
	}

	_, _ = m.Update(taskProgressMsg{
		task:       task,
		completion: proof,
		turnID:     4,
		sessionID:  "session-current",
	})
	if m.task != task || !reflect.DeepEqual(m.completion, proof) {
		t.Fatalf("accepted progress = task %#v proof %#v, want task %#v proof %#v", m.task, m.completion, task, proof)
	}
}

func TestTemporaryScopeModeVisibleInHeader(t *testing.T) {
	mode := agent.TaskPlan
	m, err := newModel(config.Default(), nil)
	if err != nil {
		t.Fatal(err)
	}
	m.turnMode = &mode
	m.width, m.height = 100, 30
	view := m.View()
	if !strings.Contains(view, "Plan") || !strings.Contains(view, "this turn") {
		t.Fatalf("temporary mode missing from header: %q", view)
	}
}

func TestStaleCanceledTurnEventsCannotMutateCurrentTurn(t *testing.T) {
	currentTask := &taskstate.Task{
		SessionID: "session-new",
		Status:    taskstate.StatusWorking,
		Goal:      "current task",
	}
	m := &model{
		sessionID: "session-new",
		turnID:    2,
		busy:      true,
		task:      currentTask,
		lines:     []logLine{{Kind: "system", Text: "current"}},
		h:         &handler{permCh: make(chan perm.Decision, 1)},
		vp:        viewport.New(80, 20),
	}

	_, _ = m.Update(logMsg{Kind: "assistant", Text: "stale text", turnID: 1, sessionID: "session-new"})
	_, _ = m.Update(taskProgressMsg{
		task:      &taskstate.Task{SessionID: "session-new", Goal: "stale task"},
		turnID:    1,
		sessionID: "session-new",
	})
	_, _ = m.Update(permAskMsg{
		Request:   perm.Request{Tool: "stale-tool", Summary: "stale permission"},
		turnID:    1,
		sessionID: "session-new",
	})
	_, _ = m.Update(doneMsg{
		history:   []llm.Message{{Role: "user", Content: "stale"}},
		turnID:    1,
		sessionID: "session-new",
	})
	_, _ = m.Update(evolveMsg{text: "stale reflection", turnID: 1, sessionID: "session-new"})

	if len(m.lines) != 1 || m.lines[0].Text != "current" {
		t.Fatalf("stale event changed lines: %#v", m.lines)
	}
	if m.task != currentTask {
		t.Fatalf("stale event replaced task: %#v", m.task)
	}
	if m.perm != nil {
		t.Fatalf("stale event opened permission prompt: %#v", m.perm)
	}
	if !m.busy {
		t.Fatal("stale done event cleared the active turn")
	}
	if len(m.history) != 0 {
		t.Fatalf("stale done event changed history: %#v", m.history)
	}
}

func TestStopInvalidatesActiveTurnID(t *testing.T) {
	stopCalled := false
	m := &model{
		turnID: 2,
		busy:   true,
		cancel: func() { stopCalled = true },
		h:      &handler{permCh: make(chan perm.Decision, 1)},
	}

	m.stop()

	if !stopCalled {
		t.Fatal("stop did not cancel the active turn")
	}
	if m.turnID != 3 {
		t.Fatalf("turn ID after stop = %d, want 3", m.turnID)
	}
	if m.busy {
		t.Fatal("stop left the model busy")
	}
}

func TestRunProgramStopsModelAfterProgramReturns(t *testing.T) {
	programErr := errors.New("program ended")
	stopped := false
	m := &model{
		turnID: 2,
		busy:   true,
		cancel: func() { stopped = true },
		h:      &handler{permCh: make(chan perm.Decision, 1)},
	}

	err := runProgram(m, func() (tea.Model, error) { return m, programErr })
	if !errors.Is(err, programErr) {
		t.Fatalf("runProgram error = %v, want %v", err, programErr)
	}
	if !stopped {
		t.Fatal("runProgram did not stop the model")
	}
	if m.busy || m.cancel != nil {
		t.Fatalf("model remained active after program return: busy=%v cancel=%v", m.busy, m.cancel != nil)
	}
}

func TestRunProgramClosesAgentRegistryAfterProgramReturns(t *testing.T) {
	workspace := t.TempDir()
	reg := tools.NewRegistry(tools.Context{Workspace: workspace})
	if err := reg.AttachMCP(&mcpbridge.Manager{}); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	a := agent.New(cfg, &llm.Scripted{}, reg, perm.New(config.ModeFast, workspace, nil))
	m := &model{ag: a}

	if err := runProgram(m, func() (tea.Model, error) { return m, nil }); err != nil {
		t.Fatal(err)
	}
	if err := reg.AttachMCP(&mcpbridge.Manager{}); !errors.Is(err, tools.ErrRegistryClosed) {
		t.Fatalf("attach after TUI return = %v, want tools.ErrRegistryClosed", err)
	}
}

func TestQuitKeysStopBeforeQuitting(t *testing.T) {
	for _, key := range []struct {
		name string
		key  tea.KeyType
	}{
		{name: "ctrl-c", key: tea.KeyCtrlC},
		{name: "ctrl-d", key: tea.KeyCtrlD},
	} {
		t.Run(key.name, func(t *testing.T) {
			canceled := false
			m := &model{
				turnID: 2,
				cancel: func() { canceled = true },
				h:      &handler{permCh: make(chan perm.Decision, 1)},
			}

			_, cmd := m.Update(tea.KeyMsg{Type: key.key})
			if cmd == nil {
				t.Fatal("quit key returned no quit command")
			}
			if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatalf("quit key command = %T, want tea.QuitMsg", cmd())
			}
			if !canceled {
				t.Fatal("quit key did not cancel the active turn")
			}
			if m.turnID != 3 {
				t.Fatalf("turn ID after quit key = %d, want 3", m.turnID)
			}
		})
	}
}

func TestQuitSlashStopsBeforeQuitting(t *testing.T) {
	canceled := false
	m := &model{
		turnID: 2,
		cancel: func() { canceled = true },
		h:      &handler{permCh: make(chan perm.Decision, 1)},
	}

	cmd := m.slash("/quit")
	if cmd == nil {
		t.Fatal("/quit returned no quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("/quit command = %T, want tea.QuitMsg", cmd())
	}
	if !canceled {
		t.Fatal("/quit did not cancel the active turn")
	}
	if m.turnID != 3 {
		t.Fatalf("turn ID after /quit = %d, want 3", m.turnID)
	}
}

func TestHandlerTagsEventsWithTurnAndSession(t *testing.T) {
	var got []tea.Msg
	h := &handler{
		send:      func(msg tea.Msg) { got = append(got, msg) },
		turnID:    9,
		sessionID: "session-9",
	}
	h.OnText("hello")
	h.OnTaskState(&taskstate.Task{SessionID: "session-9", Goal: "goal"})

	if len(got) != 2 {
		t.Fatalf("got %d events, want 2", len(got))
	}
	log, ok := got[0].(logMsg)
	if !ok || log.turnID != 9 || log.sessionID != "session-9" {
		t.Fatalf("log event metadata = %#v", got[0])
	}
	task, ok := got[1].(taskProgressMsg)
	if !ok || task.turnID != 9 || task.sessionID != "session-9" {
		t.Fatalf("task event metadata = %#v", got[1])
	}
}

func TestHandlerRedactsDiagnosticEvents(t *testing.T) {
	const (
		argumentSecret = "tui-argument-secret"
		resultSecret   = "tui-result-secret"
		errorSecret    = "tui-error-secret"
	)
	var got []tea.Msg
	h := &handler{
		send: func(msg tea.Msg) { got = append(got, msg) },
	}
	h.OnToolStart(llm.ToolCall{
		Name:      "mcp\nforged",
		Arguments: "\x1b[31m{\"access_token\":\"" + argumentSecret + "\"}",
	})
	h.OnToolEnd(llm.ToolCall{Name: "mcp"}, `{"api_key":"`+resultSecret+`"}`, nil)
	h.OnToolEnd(llm.ToolCall{Name: "mcp"}, "", errors.New("tool failed\npassword="+errorSecret))
	h.OnError(errors.New("token=" + errorSecret))

	if len(got) != 4 {
		t.Fatalf("events = %d, want 4", len(got))
	}
	var text strings.Builder
	for _, msg := range got {
		line, ok := msg.(logMsg)
		if !ok {
			t.Fatalf("event = %#v, want logMsg", msg)
		}
		text.WriteString(line.Text)
		text.WriteByte('\n')
	}
	joined := text.String()
	for _, secret := range []string{argumentSecret, resultSecret, errorSecret} {
		if strings.Contains(joined, secret) {
			t.Fatalf("TUI diagnostic leaked secret %q: %q", secret, joined)
		}
	}
	if strings.Contains(joined, "\x1b") || strings.Contains(joined, "mcp\nforged") || !strings.Contains(joined, "mcp forged") {
		t.Fatalf("TUI diagnostic retained control/newline injection: %q", joined)
	}
	if !strings.Contains(joined, "[REDACTED]") {
		t.Fatalf("TUI diagnostic lost redaction marker: %q", joined)
	}
}

func TestPermissionDisplayRedactsDiagnosticFields(t *testing.T) {
	const secret = "tui-permission-secret"
	m := &model{
		lines:     []logLine{{Kind: "system", Text: "ready"}},
		sessionID: "session-1",
		vp:        viewport.New(80, 20),
	}
	_, _ = m.Update(permAskMsg{Request: perm.Request{
		Hint:    "token=" + secret,
		Summary: "write access_token=" + secret,
	}})
	if m.perm == nil {
		t.Fatal("permission request was not retained")
	}
	if strings.Contains(m.perm.Hint+m.perm.Summary, secret) || strings.Contains(m.lines[len(m.lines)-1].Text, secret) {
		t.Fatalf("permission display leaked secret: perm=%#v lines=%#v", m.perm, m.lines)
	}
	if !strings.Contains(m.lines[len(m.lines)-1].Text, "[REDACTED]") {
		t.Fatalf("permission display lost redaction marker: %#v", m.lines[len(m.lines)-1])
	}
}

func TestClearStartsNewSessionAndClearsDurableTask(t *testing.T) {
	testSessionReset(t, "clear")
}

func TestResetStartsNewSessionAndClearsDurableTask(t *testing.T) {
	testSessionReset(t, "reset")
}

func TestNewSessionRestoresConfiguredTaskModeAfterAutomaticInference(t *testing.T) {
	for _, command := range []string{"clear", "reset"} {
		for _, tt := range []struct {
			name       string
			configured agent.TaskMode
			prompt     string
			inferred   agent.TaskMode
		}{
			{"default agent after plan", agent.TaskAgent, "build something, but plan it first", agent.TaskPlan},
			{"manual plan after debug", agent.TaskPlan, "debug this broken build", agent.TaskDebug},
			{"manual ask after plan", agent.TaskAsk, "build something, but plan it first", agent.TaskPlan},
			{"manual debug after report", agent.TaskDebug, "build something, but inspect and report first", agent.TaskAsk},
		} {
			t.Run(command+"/"+tt.name, func(t *testing.T) {
				workspace := t.TempDir()
				cfg := config.Default()
				cfg.Workspace = workspace
				cfg.TaskMode = string(tt.configured)
				a := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
				m := &model{
					cfg:       cfg,
					ag:        a,
					sessionID: "old-session",
					lines:     []logLine{{Kind: "system", Text: "ready"}},
					h:         &handler{permCh: make(chan perm.Decision, 1)},
					vp:        viewport.New(80, 20),
				}

				m.autoApplyScopedPrompt(tt.prompt)
				if got := a.TaskModeSnapshot(); got != tt.inferred {
					t.Fatalf("inferred task mode = %q, want %q", got, tt.inferred)
				}
				if got := agent.ParseTaskMode(m.cfg.TaskMode); got != tt.configured {
					t.Fatalf("saved task mode changed during inference: %q, want %q", got, tt.configured)
				}

				if command == "clear" {
					m.slashLocal("clear")
				} else {
					m.slash("/reset")
				}
				if got := a.TaskModeSnapshot(); got != tt.configured {
					t.Fatalf("task mode after %s = %q, want configured %q", command, got, tt.configured)
				}
			})
		}
	}
}

func testSessionReset(t *testing.T, command string) {
	t.Helper()
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Mode = config.ModeFast
	a := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = taskstate.NewStore(t.TempDir())
	oldSession := "old-session"
	task, err := taskstate.New(oldSession, "finish the old task", []string{"work"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.TaskStore.Save(task); err != nil {
		t.Fatal(err)
	}
	a.SetTaskSession(oldSession)
	if a.TaskSnapshot() == nil {
		t.Fatal("test setup did not load the old durable task")
	}
	m := &model{
		cfg:       cfg,
		ag:        a,
		history:   []llm.Message{{Role: "user", Content: "old"}},
		sessionID: oldSession,
		lines:     []logLine{{Kind: "user", Text: "old"}},
		task:      a.TaskSnapshot(),
		h:         &handler{permCh: make(chan perm.Decision, 1)},
		vp:        viewport.New(80, 20),
	}

	if command == "clear" {
		m.slashLocal("clear")
	} else {
		m.slash("/reset")
	}

	if m.sessionID == oldSession {
		t.Fatalf("%s reused the old session ID %q", command, m.sessionID)
	}
	if m.history != nil {
		t.Fatalf("history after %s = %#v, want nil", command, m.history)
	}
	if a.TaskSession != m.sessionID {
		t.Fatalf("agent task session = %q, want %q", a.TaskSession, m.sessionID)
	}
	if a.TaskSnapshot() != nil {
		t.Fatalf("agent retained durable task after %s: %#v", command, a.TaskSnapshot())
	}
	if m.task != nil {
		t.Fatalf("model retained durable task after %s: %#v", command, m.task)
	}
	if len(m.lines) != 1 || m.lines[0].Text == "old" {
		t.Fatalf("lines after %s = %#v", command, m.lines)
	}
}
