package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/app"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/lifecycle"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/projects"
	"github.com/saiaathish/picogent/internal/taskstate"
)

type tuiSignalHelperResult struct {
	SessionID string `json:"session_id"`
	Error     string `json:"error"`
}

func TestTUIFreshProcessSignalRetainsInterruptedTurn(t *testing.T) {
	if os.Getenv("PICOGENT_TUI_SIGNAL_HELPER") == "1" {
		tuiSignalHelper(t)
		return
	}
	if runtime.GOOS == "windows" {
		t.Skip("Windows hosted runners do not provide a stable child SIGINT boundary; the limitation is recorded in the lifecycle contract")
	}

	root := t.TempDir()
	home := filepath.Join(root, "home")
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PICOGENT_HOME", home)
	t.Setenv("PICOGENT_PROVIDER", "")
	t.Setenv("PICOGENT_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("PICOGENT_BASE_URL", "")
	t.Setenv("PICOGENT_ROUTER", "0")
	t.Setenv("PICOGENT_MODE", "")

	started := make(chan struct{})
	release := make(chan struct{})
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-started:
		default:
			close(started)
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(func() {
		close(release)
		provider.Close()
	})

	cfg := config.Default()
	cfg.SetupComplete = true
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOpenAI
	cfg.APIKey = "test-key"
	cfg.BaseURL = provider.URL
	cfg.Model = "tui-lifecycle-test-model"
	cfg.Router.Enabled = false
	cfg.Router.UseLLMAdvisor = false
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	resultPath := filepath.Join(root, "helper-result.json")
	cmd := exec.Command(os.Args[0], "-test.run", "^TestTUIFreshProcessSignalRetainsInterruptedTurn$", "-test.count=1")
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(),
		"PICOGENT_TUI_SIGNAL_HELPER=1",
		"PICOGENT_HOME="+home,
		"PICOGENT_TUI_RESULT="+resultPath,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	select {
	case <-started:
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		<-wait
		t.Fatalf("TUI child did not reach provider barrier\nstdout=%q\nstderr=%q", stdout.String(), stderr.String())
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		_ = cmd.Process.Kill()
		<-wait
		t.Fatalf("interrupt TUI child: %v", err)
	}
	select {
	case err := <-wait:
		if err != nil {
			t.Fatalf("TUI child failed: %v\nstdout=%q\nstderr=%q", err, stdout.String(), stderr.String())
		}
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		<-wait
		t.Fatal("TUI child did not finish its interrupted turn")
	}

	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var helper tuiSignalHelperResult
	if err := json.Unmarshal(data, &helper); err != nil {
		t.Fatal(err)
	}
	if helper.SessionID == "" || helper.Error == "" {
		t.Fatalf("TUI helper result = %#v, want session and cancellation error", helper)
	}
	store := taskstate.NewStore(filepath.Join(home, "tasks", projects.IDForPath(workspace)))
	task, err := store.Load(helper.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	scenario := tuiLifecycleScenario(t, "tui-signal-active-turn")
	observation := lifecycle.Observe(
		scenario.ID, scenario.Surface, scenario.Trigger, task,
		lifecycle.CompletionProjection{Required: true}, errors.New(helper.Error),
	)
	if violations := scenario.Check(observation); len(violations) != 0 {
		t.Fatalf("fresh TUI observation violations = %v", violations)
	}
}

func tuiSignalHelper(t *testing.T) {
	t.Helper()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg, ag, err := app.LoadContext(ctx, ".")
	if err != nil {
		t.Fatal(err)
	}
	m, err := newModel(cfg, ag)
	if err != nil {
		ag.Close()
		t.Fatal(err)
	}
	defer ag.Close()

	cmd := m.runAgent("fix the greeting")
	if cmd == nil {
		t.Fatal("TUI run command was nil")
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	<-ctx.Done()
	m.stop()

	msg := <-done
	finished, ok := msg.(doneMsg)
	if !ok {
		t.Fatalf("TUI interrupted command result = %T, want doneMsg", msg)
	}
	if finished.err == nil || !strings.Contains(strings.ToLower(finished.err.Error()), "context canceled") {
		t.Fatalf("TUI interrupted command error = %v, want context cancellation", finished.err)
	}
	_, _ = m.Update(finished)
	data, err := json.Marshal(tuiSignalHelperResult{SessionID: m.sessionID, Error: finished.err.Error()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("PICOGENT_TUI_RESULT"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestTUISessionSaveFailureMatchesLifecycleScenario(t *testing.T) {
	home := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(home, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PICOGENT_HOME", home)

	workspace := t.TempDir()
	store := taskstate.NewStore(t.TempDir())
	const sessionID = "tui-session-save-failure"
	task, err := taskstate.New(sessionID, "finish the requested change", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := task.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	if _, ok := task.BeginTurn(taskstate.TurnRouteImplement); !ok {
		t.Fatal("TUI session-save fixture did not start an active turn")
	}
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	m := &model{
		cfg:       cfg,
		sessionID: sessionID,
		lines:     []logLine{{Kind: "user", Text: "request"}},
		vp:        viewport.New(80, 20),
	}
	_, _ = m.Update(doneMsg{
		history: []llm.Message{{Role: "user", Content: "request"}},
		result:  agent.Result{Task: task},
	})
	if len(m.lines) == 0 || m.lines[len(m.lines)-1].Kind != "error" || !strings.Contains(m.lines[len(m.lines)-1].Text, "couldn't save session") {
		t.Fatalf("TUI session-save result = %#v, want visible session-save error", m.lines)
	}
	scenario := tuiLifecycleScenario(t, "tui-session-save-failure")
	observation := lifecycle.Observe(
		scenario.ID, scenario.Surface, scenario.Trigger, m.task,
		lifecycle.CompletionProjection{Required: true, Ready: m.completion.Ready},
		errors.New(m.lines[len(m.lines)-1].Text),
	)
	if violations := scenario.Check(observation); len(violations) != 0 {
		t.Fatalf("TUI session-save observation violations = %v", violations)
	}
	if m.completion.Ready {
		t.Fatal("TUI session-save failure projected completion")
	}
	if got, err := store.Load(sessionID); err != nil || got.Status == taskstate.StatusDone {
		t.Fatalf("TUI session-save durable task = %#v, err=%v; want resumable task", got, err)
	}
}

func tuiLifecycleScenario(t *testing.T, id string) lifecycle.Scenario {
	t.Helper()
	for _, scenario := range lifecycle.Scenarios() {
		if scenario.ID == id {
			return scenario
		}
	}
	t.Fatalf("lifecycle scenario %q not found", id)
	return lifecycle.Scenario{}
}
