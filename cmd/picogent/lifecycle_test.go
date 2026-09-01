package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/lifecycle"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/projects"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
)

func TestHeadlessFreshProcessSignalRetainsInterruptedTurn(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows hosted runners do not provide a stable child SIGINT boundary; the limitation is recorded in the lifecycle contract")
	}

	home := t.TempDir()
	codexHome := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)
	t.Setenv("PICOGENT_CODEX_HOME", codexHome)
	t.Setenv("PICOGENT_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("PICOGENT_PROVIDER", "")
	t.Setenv("PICOGENT_BASE_URL", "")
	t.Setenv("PICOGENT_ROUTER", "0")
	t.Setenv("PICOGENT_MODE", "")

	started := make(chan struct{})
	release := make(chan struct{})
	var startedOnce sync.Once
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedOnce.Do(func() { close(started) })
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
	cfg.Model = "lifecycle-test-model"
	cfg.Router.Enabled = false
	cfg.Router.UseLLMAdvisor = false
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	binary := filepath.Join(t.TempDir(), "picogent")
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = mustWorkingDirectory(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build headless lifecycle binary: %v\n%s", err, output)
	}

	const prompt = "fix the greeting"
	cmd := exec.Command(binary, "run", "--yes", "--dir", workspace, prompt)
	cmd.Env = os.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("headless child did not reach provider barrier\nstdout=%q\nstderr=%q", stdout.String(), stderr.String())
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("interrupt headless child: %v", err)
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	select {
	case err := <-wait:
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 130 {
			t.Fatalf("headless child exit = %v, want exit 130\nstdout=%q\nstderr=%q", err, stdout.String(), stderr.String())
		}
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		<-wait
		t.Fatal("headless child did not exit after SIGINT")
	}
	if !strings.Contains(strings.ToLower(stderr.String()), "headless run canceled") {
		t.Fatalf("headless stderr = %q, want canceled diagnostic", stderr.String())
	}

	store := taskstate.NewStore(filepath.Join(home, "tasks", projects.IDForPath(workspace)))
	task, err := store.Load(headlessTaskSessionID(prompt))
	if err != nil {
		t.Fatal(err)
	}
	scenario := headlessLifecycleScenario(t, "headless-signal-active-turn")
	observation := lifecycle.Observe(
		scenario.ID, scenario.Surface, scenario.Trigger, task,
		lifecycle.CompletionProjection{Required: true},
		errors.New(stderr.String()),
	)
	if violations := scenario.Check(observation); len(violations) != 0 {
		t.Fatalf("fresh headless observation violations = %v", violations)
	}
}

func TestHeadlessTaskSaveFailureMatchesLifecycleScenario(t *testing.T) {
	workspace := t.TempDir()
	goodStore := taskstate.NewStore(t.TempDir())
	badRoot := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(badRoot, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	args, err := json.Marshal(map[string]string{"path": "done.txt", "content": "completed"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	client := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "write", Name: "write_file", Arguments: string(args)}}}},
		{Message: llm.Message{Role: "assistant", Content: "Goal complete: done"}},
	}}
	reg := tools.NewRegistry(tools.Context{
		Workspace: workspace,
		VerifyTargets: func(context.Context, []string) (string, error) {
			return "verify PASS\nrequested checks passed", nil
		},
	})
	a := agent.New(cfg, client, reg, perm.New(config.ModeFast, workspace, nil))
	a.SetTaskStore(goodStore)
	const sessionID = "headless-task-save-failure"
	if err := a.SetTaskSession(sessionID); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	h := &headlessTaskSaveFailureHandler{
		stdioHandler: &stdioHandler{yes: true, in: bufio.NewReader(strings.NewReader("")), out: &stdout, errOut: &stderr},
		ag:           a,
		badStore:     taskstate.NewStore(badRoot),
	}
	_, result, runErr := a.RunWithOptions(context.Background(), nil, llm.Message{Role: "user", Content: "finish the requested change"}, h, agent.RunOptions{SuppressUndo: true})
	if runErr == nil || !strings.Contains(strings.ToLower(runErr.Error()), "durable task state") {
		t.Fatalf("headless task save failure = %v, want durable-state error", runErr)
	}
	if !h.switched || result.GoalDone {
		t.Fatalf("headless save failure switched=%v goalDone=%v result=%#v", h.switched, result.GoalDone, result)
	}
	task, err := goodStore.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	scenario := headlessLifecycleScenario(t, "headless-task-save-failure")
	observation := lifecycle.Observe(
		scenario.ID, scenario.Surface, scenario.Trigger, task,
		lifecycle.CompletionProjection{Required: true}, runErr,
	)
	if violations := scenario.Check(observation); len(violations) != 0 {
		t.Fatalf("headless save-failure observation violations = %v", violations)
	}
}

type headlessTaskSaveFailureHandler struct {
	*stdioHandler
	ag       *agent.Agent
	badStore *taskstate.Store
	switched bool
}

func (h *headlessTaskSaveFailureHandler) OnTaskState(task *taskstate.Task) {
	if h.switched || task == nil || task.Status != taskstate.StatusDone {
		return
	}
	last := task.LastTurn()
	if last == nil || last.State != taskstate.TurnActive {
		return
	}
	h.switched = true
	h.ag.SetTaskStore(h.badStore)
}

func headlessLifecycleScenario(t *testing.T, id string) lifecycle.Scenario {
	t.Helper()
	for _, scenario := range lifecycle.Scenarios() {
		if scenario.ID == id {
			return scenario
		}
	}
	t.Fatalf("lifecycle scenario %q not found", id)
	return lifecycle.Scenario{}
}

func mustWorkingDirectory(t *testing.T) string {
	t.Helper()
	directory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return directory
}
