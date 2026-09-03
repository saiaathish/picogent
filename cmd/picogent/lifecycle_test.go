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
	"github.com/saiaathish/picogent/internal/app"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/lifecycle"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/projects"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
)

func TestHeadlessFreshProcessKillRecoversInterruptedTurn(t *testing.T) {
	if os.Getenv("PICOGENT_HEADLESS_PROCESS_KILL_RESUME_HELPER") == "1" {
		headlessProcessKillResumeHelper(t)
		return
	}

	root := t.TempDir()
	home := filepath.Join(root, "home")
	codexHome := filepath.Join(root, "codex")
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("PICOGENT_HOME", home)
	t.Setenv("PICOGENT_CODEX_HOME", codexHome)
	t.Setenv("PICOGENT_PROVIDER", "")
	t.Setenv("PICOGENT_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("PICOGENT_BASE_URL", "")
	t.Setenv("PICOGENT_ROUTER", "0")
	t.Setenv("PICOGENT_MODE", "")

	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	var startedOnce sync.Once
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedOnce.Do(func() { close(started) })
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		provider.Close()
	})

	cfg := config.Default()
	cfg.SetupComplete = true
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOpenAI
	cfg.APIKey = "test-key"
	cfg.BaseURL = provider.URL
	cfg.Model = "headless-process-kill-test-model"
	cfg.Router.Enabled = false
	cfg.Router.UseLLMAdvisor = false
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	binary := filepath.Join(t.TempDir(), "picogent")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = mustWorkingDirectory(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build headless process-kill binary: %v\n%s", err, output)
	}

	const prompt = "fix the greeting after a process restart"
	cmd := exec.Command(binary, "run", "--yes", "--dir", workspace, prompt)
	cmd.Env = os.Environ()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})

	select {
	case <-started:
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("headless kill child did not reach provider barrier\nstdout=%q\nstderr=%q", stdout.String(), stderr.String())
	}

	store := taskstate.NewStore(filepath.Join(home, "tasks", projects.IDForPath(workspace)))
	active := waitForHeadlessTask(t, store, headlessTaskSessionID(prompt), func(task *taskstate.Task) bool {
		last := task.LastTurn()
		return task.Status == taskstate.StatusWorking && last != nil && last.State == taskstate.TurnActive
	})
	if active == nil {
		t.Fatal("headless child did not persist an active turn")
	}

	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("kill active headless child: %v", err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("killed headless child exited cleanly")
	}
	cmd.Process = nil

	resultPath := filepath.Join(root, "resume-result.json")
	resumer := exec.Command(os.Args[0], "-test.run", "^TestHeadlessFreshProcessKillRecoversInterruptedTurn$", "-test.count=1")
	resumer.Dir = workspace
	resumer.Env = append(os.Environ(),
		"PICOGENT_HEADLESS_PROCESS_KILL_RESUME_HELPER=1",
		"PICOGENT_HOME="+home,
		"PICOGENT_CODEX_HOME="+codexHome,
		"PICOGENT_HEADLESS_PROCESS_KILL_WORKSPACE="+workspace,
		"PICOGENT_HEADLESS_PROCESS_KILL_TASK_DIR="+filepath.Join(home, "tasks", projects.IDForPath(workspace)),
		"PICOGENT_HEADLESS_PROCESS_KILL_SESSION="+headlessTaskSessionID(prompt),
		"PICOGENT_HEADLESS_PROCESS_KILL_RESULT="+resultPath,
	)
	if output, err := resumer.CombinedOutput(); err != nil {
		t.Fatalf("fresh headless process recovery failed: %v\n%s", err, output)
	}
	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var result headlessProcessKillRecoveryResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.SessionID != headlessTaskSessionID(prompt) || result.TaskStatus != taskstate.StatusWorking || result.TurnState != taskstate.TurnInterrupted || result.TurnRoute != string(taskstate.TurnRouteRecover) || result.EvidenceState != "UNVERIFIED" || result.StopReason != taskstate.StopProcessRestart || strings.TrimSpace(result.Hypothesis) == "" {
		t.Fatalf("fresh headless process recovery = %#v, want process-restart recovery", result)
	}
	if result.Completion.Ready || strings.TrimSpace(result.Completion.Reason) == "" {
		t.Fatalf("fresh headless completion = %#v, want fail-closed proof", result.Completion)
	}

	recovered, err := store.Load(result.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	scenario := headlessLifecycleScenario(t, "headless-process-kill-active-turn")
	observation := lifecycle.Observe(
		scenario.ID, scenario.Surface, scenario.Trigger, recovered,
		lifecycle.CompletionProjection{Required: true, Ready: result.Completion.Ready}, nil,
	)
	if violations := scenario.Check(observation); len(violations) != 0 {
		t.Fatalf("fresh headless process-kill observation violations = %v", violations)
	}
}

type headlessProcessKillRecoveryResult struct {
	SessionID     string                    `json:"session_id"`
	TaskStatus    taskstate.Status          `json:"task_status"`
	TurnSequence  uint64                    `json:"turn_sequence"`
	TurnState     taskstate.TurnState       `json:"turn_state"`
	TurnRoute     string                    `json:"turn_route"`
	EvidenceState string                    `json:"evidence_state"`
	StopReason    taskstate.StopReason      `json:"stop_reason"`
	Hypothesis    string                    `json:"hypothesis"`
	Completion    taskstate.CompletionCheck `json:"completion"`
}

func headlessProcessKillResumeHelper(t *testing.T) {
	t.Helper()
	workspace := os.Getenv("PICOGENT_HEADLESS_PROCESS_KILL_WORKSPACE")
	_, ag, err := app.LoadContext(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer ag.Close()
	if err := ag.SetTaskSession(os.Getenv("PICOGENT_HEADLESS_PROCESS_KILL_SESSION")); err != nil {
		t.Fatal(err)
	}
	task := ag.TaskSnapshot()
	if task == nil {
		t.Fatal("fresh headless process did not load durable task")
	}
	last := task.LastTurn()
	if last == nil || last.State != taskstate.TurnInterrupted || last.Route != string(taskstate.TurnRouteRecover) || last.EvidenceState != "UNVERIFIED" || last.StopReason != taskstate.StopProcessRestart || strings.TrimSpace(last.Hypothesis) == "" {
		t.Fatalf("fresh headless process recovered turn = %#v, want process-restart recovery", last)
	}
	result := headlessProcessKillRecoveryResult{
		SessionID:     task.SessionID,
		TaskStatus:    task.Status,
		TurnSequence:  last.Sequence,
		TurnState:     last.State,
		TurnRoute:     last.Route,
		EvidenceState: last.EvidenceState,
		StopReason:    last.StopReason,
		Hypothesis:    last.Hypothesis,
		Completion:    agent.CompletionProof(task),
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("PICOGENT_HEADLESS_PROCESS_KILL_RESULT"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func waitForHeadlessTask(t *testing.T, store *taskstate.Store, sessionID string, want func(*taskstate.Task) bool) *taskstate.Task {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		task, err := store.Load(sessionID)
		if err == nil {
			if want(task) {
				return task
			}
		} else if !errors.Is(err, taskstate.ErrNotFound) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for task %q", sessionID)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestHeadlessFreshProcessSignalRetainsInterruptedTurn(t *testing.T) {
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
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
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
	if err := prepareSignalChild(cmd); err != nil {
		t.Fatalf("prepare headless signal child: %v", err)
	}
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
	if err := sendInterruptToChild(cmd); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("interrupt headless child with platform console signal: %v", err)
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
	if h.switched || task == nil {
		return
	}
	last := task.LastTurn()
	if last == nil || last.State != taskstate.TurnActive || len(task.ChangedFiles) == 0 {
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
