package gui

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
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

func TestGUIFreshProcessShutdownRetainsInterruptedTurn(t *testing.T) {
	if os.Getenv("PICOGENT_GUI_SHUTDOWN_HELPER") == "1" {
		if err := Run(); err != nil {
			t.Fatalf("GUI helper returned error: %v", err)
		}
		return
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	var err error
	workspace, err = filepath.EvalSymlinks(workspace)
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
	var releaseOnce sync.Once
	releaseProvider := func() {
		releaseOnce.Do(func() { close(release) })
	}
	var startedOnce sync.Once
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedOnce.Do(func() { close(started) })
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(func() {
		releaseProvider()
		provider.Close()
	})

	cfg := config.Default()
	cfg.SetupComplete = true
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOpenAI
	cfg.APIKey = "test-key"
	cfg.BaseURL = provider.URL
	cfg.Model = "gui-lifecycle-test-model"
	cfg.Router.Enabled = false
	cfg.Router.UseLLMAdvisor = false
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run", "^TestGUIFreshProcessShutdownRetainsInterruptedTurn$", "-test.count=1")
	cmd.Dir = workspace
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(),
		"PICOGENT_GUI_SHUTDOWN_HELPER=1",
		"PICOGENT_HOME="+home,
		"PICOGENT_NO_BROWSER=1",
		"PICOGENT_GUI_ADDR=127.0.0.1:0",
		"PICOGENT_PROVIDER=",
		"PICOGENT_API_KEY=",
		"OPENAI_API_KEY=",
		"PICOGENT_BASE_URL=",
		"PICOGENT_ROUTER=0",
		"PICOGENT_MODE=",
	)
	if err := prepareSignalChild(cmd); err != nil {
		t.Fatal(err)
	}
	var childOutput strings.Builder
	urlCh := make(chan string, 1)
	stdoutDone := make(chan struct{})
	go func() {
		defer close(stdoutDone)
		scanner := bufio.NewScanner(stdout)
		found := false
		for scanner.Scan() {
			line := scanner.Text()
			childOutput.WriteString(line)
			childOutput.WriteByte('\n')
			if !found && strings.HasPrefix(line, "picogent gui ") {
				found = true
				urlCh <- strings.TrimSpace(strings.TrimPrefix(line, "picogent gui "))
			}
		}
		if !found {
			urlCh <- ""
		}
	}()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	wait := make(chan error, 1)
	childDone := make(chan struct{})
	go func() {
		wait <- cmd.Wait()
		close(childDone)
	}()
	cleanupChild := func() {
		cleanupGUIChild(t, cmd, wait, childDone, stdoutDone, releaseProvider, "GUI shutdown child")
	}
	t.Cleanup(cleanupChild)

	var baseURL string
	select {
	case baseURL = <-urlCh:
		if baseURL == "" {
			cleanupChild()
			t.Fatalf("GUI child exited before announcing listener; stdout=%q stderr=%q", childOutput.String(), stderr.String())
		}
	case <-time.After(15 * time.Second):
		cleanupChild()
		t.Fatalf("GUI child did not announce listener; stdout=%q stderr=%q", childOutput.String(), stderr.String())
	}

	state := guiProcessState(t, baseURL)
	if state.SessionID == "" {
		t.Fatal("GUI state did not include a session ID")
	}
	response := guiJSONRequest(t, http.MethodPost, guiEndpoint(baseURL, "/api/chat"), `{"prompt":"fix the greeting"}`)
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("GUI chat status = %d, want %d", response.StatusCode, http.StatusAccepted)
	}
	response.Body.Close()

	select {
	case <-started:
	case <-time.After(15 * time.Second):
		cleanupChild()
		t.Fatalf("GUI child did not reach provider barrier; stdout=%q stderr=%q", childOutput.String(), stderr.String())
	}
	if err := sendInterruptToChild(cmd); err != nil {
		cleanupChild()
		t.Fatalf("interrupt GUI child: %v", err)
	}
	if err, ok := waitGUIChildFor(wait, 15*time.Second); !ok {
		cleanupChild()
		t.Fatalf("GUI child did not finish shutdown; stdout=%q stderr=%q", childOutput.String(), stderr.String())
	} else if err != nil {
		if !waitGUIChannelFor(stdoutDone, guiChildCleanupTimeout) {
			cleanupChild()
		}
		t.Fatalf("GUI child failed after shutdown: %v\nstdout=%q\nstderr=%q", err, childOutput.String(), stderr.String())
	}
	if !waitGUIChannelFor(stdoutDone, 15*time.Second) {
		cleanupChild()
		t.Fatalf("GUI child stdout reader did not finish; stdout=%q stderr=%q", childOutput.String(), stderr.String())
	}

	store := taskstate.NewStore(filepath.Join(home, "tasks", projects.IDForPath(workspace)))
	task, err := store.Load(state.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	scenario := guiLifecycleScenario(t, "gui-shutdown-active-turn")
	observation := lifecycle.Observe(
		scenario.ID, scenario.Surface, scenario.Trigger, task,
		lifecycle.CompletionProjection{Required: true}, nil,
	)
	if violations := scenario.Check(observation); len(violations) != 0 {
		t.Fatalf("fresh GUI shutdown observation violations = %v", violations)
	}
}

func TestGUIFreshProcessKillRecoversInterruptedTurn(t *testing.T) {
	if os.Getenv("PICOGENT_GUI_PROCESS_KILL_GUI_HELPER") == "1" {
		if err := Run(); err != nil {
			t.Fatalf("GUI kill helper returned error: %v", err)
		}
		return
	}
	if os.Getenv("PICOGENT_GUI_PROCESS_KILL_RESUME_HELPER") == "1" {
		guiProcessKillResumeHelper(t)
		return
	}

	root := t.TempDir()
	home := filepath.Join(root, "home")
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	var err error
	workspace, err = filepath.EvalSymlinks(workspace)
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
	var releaseOnce sync.Once
	releaseProvider := func() {
		releaseOnce.Do(func() { close(release) })
	}
	var startedOnce sync.Once
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedOnce.Do(func() { close(started) })
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(func() {
		releaseProvider()
		provider.Close()
	})

	cfg := config.Default()
	cfg.SetupComplete = true
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOpenAI
	cfg.APIKey = "test-key"
	cfg.BaseURL = provider.URL
	cfg.Model = "gui-process-kill-test-model"
	cfg.Router.Enabled = false
	cfg.Router.UseLLMAdvisor = false
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run", "^TestGUIFreshProcessKillRecoversInterruptedTurn$", "-test.count=1")
	cmd.Dir = workspace
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(),
		"PICOGENT_GUI_PROCESS_KILL_GUI_HELPER=1",
		"PICOGENT_HOME="+home,
		"PICOGENT_NO_BROWSER=1",
		"PICOGENT_GUI_ADDR=127.0.0.1:0",
		"PICOGENT_PROVIDER=",
		"PICOGENT_API_KEY=",
		"OPENAI_API_KEY=",
		"PICOGENT_BASE_URL=",
		"PICOGENT_ROUTER=0",
		"PICOGENT_MODE=",
	)
	var childOutput strings.Builder
	urlCh := make(chan string, 1)
	stdoutDone := make(chan struct{})
	go func() {
		defer close(stdoutDone)
		scanner := bufio.NewScanner(stdout)
		found := false
		for scanner.Scan() {
			line := scanner.Text()
			childOutput.WriteString(line)
			childOutput.WriteByte('\n')
			if !found && strings.HasPrefix(line, "picogent gui ") {
				found = true
				urlCh <- strings.TrimSpace(strings.TrimPrefix(line, "picogent gui "))
			}
		}
		if !found {
			urlCh <- ""
		}
	}()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	wait := make(chan error, 1)
	childDone := make(chan struct{})
	go func() {
		wait <- cmd.Wait()
		close(childDone)
	}()
	cleanupChild := func() {
		cleanupGUIChild(t, cmd, wait, childDone, stdoutDone, releaseProvider, "GUI process-kill child")
	}
	t.Cleanup(cleanupChild)

	var baseURL string
	select {
	case baseURL = <-urlCh:
		if baseURL == "" {
			cleanupChild()
			t.Fatalf("GUI kill child exited before announcing listener; stdout=%q stderr=%q", childOutput.String(), stderr.String())
		}
	case <-time.After(15 * time.Second):
		cleanupChild()
		t.Fatalf("GUI kill child did not announce listener; stdout=%q stderr=%q", childOutput.String(), stderr.String())
	}

	initial := guiProcessState(t, baseURL)
	if initial.SessionID == "" {
		t.Fatal("GUI state did not include a session ID")
	}
	response := guiJSONRequest(t, http.MethodPost, guiEndpoint(baseURL, "/api/chat"), `{"prompt":"fix the greeting"}`)
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("GUI chat status = %d, want %d", response.StatusCode, http.StatusAccepted)
	}
	response.Body.Close()

	select {
	case <-started:
	case <-time.After(15 * time.Second):
		cleanupChild()
		t.Fatalf("GUI kill child did not reach provider barrier; stdout=%q stderr=%q", childOutput.String(), stderr.String())
	}
	active := guiProcessStateUntilActive(t, baseURL)
	if active.SessionID != initial.SessionID {
		t.Fatalf("GUI active state session = %q, want %q", active.SessionID, initial.SessionID)
	}
	if active.Completion == nil || active.Completion.Ready {
		t.Fatalf("GUI active completion = %#v, want fail-closed projection", active.Completion)
	}

	if err := cmd.Process.Kill(); err != nil {
		cleanupChild()
		t.Fatalf("kill active GUI child: %v", err)
	}
	if err, ok := waitGUIChildFor(wait, guiChildCleanupTimeout); !ok {
		cleanupChild()
		t.Fatalf("killed GUI child did not exit within %s", guiChildCleanupTimeout)
	} else if err == nil {
		t.Fatal("killed GUI child exited cleanly")
	}
	if !waitGUIChannelFor(stdoutDone, 15*time.Second) {
		cleanupChild()
		t.Fatal("killed GUI child stdout reader did not finish")
	}

	taskDir := filepath.Join(home, "tasks", projects.IDForPath(workspace))
	resultPath := filepath.Join(root, "resume-result.json")
	resumer := exec.Command(os.Args[0], "-test.run", "^TestGUIFreshProcessKillRecoversInterruptedTurn$", "-test.count=1")
	resumer.Dir = workspace
	resumer.Env = append(os.Environ(),
		"PICOGENT_GUI_PROCESS_KILL_RESUME_HELPER=1",
		"PICOGENT_HOME="+home,
		"PICOGENT_GUI_PROCESS_KILL_WORKSPACE="+workspace,
		"PICOGENT_GUI_PROCESS_KILL_TASK_DIR="+taskDir,
		"PICOGENT_GUI_PROCESS_KILL_SESSION="+initial.SessionID,
		"PICOGENT_GUI_PROCESS_KILL_RESULT="+resultPath,
	)
	if output, err := resumer.CombinedOutput(); err != nil {
		t.Fatalf("fresh GUI process recovery failed: %v\n%s", err, output)
	}
	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var result guiProcessKillRecoveryResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.SessionID != initial.SessionID || result.TaskStatus != taskstate.StatusWorking || result.TurnState != taskstate.TurnInterrupted || result.TurnRoute != string(taskstate.TurnRouteRecover) || result.EvidenceState != "UNVERIFIED" || result.StopReason != taskstate.StopProcessRestart || strings.TrimSpace(result.Hypothesis) == "" {
		t.Fatalf("fresh GUI process recovery = %#v, want recoverable interrupted turn", result)
	}
	if result.Completion.Ready || strings.TrimSpace(result.Completion.Reason) == "" {
		t.Fatalf("fresh GUI completion = %#v, want fail-closed proof", result.Completion)
	}

	store := taskstate.NewStore(taskDir)
	task, err := store.Load(initial.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	last := task.LastTurn()
	if last == nil || last.State != taskstate.TurnInterrupted || last.Route != string(taskstate.TurnRouteRecover) || last.StopReason != taskstate.StopProcessRestart {
		t.Fatalf("persisted GUI recovery = %#v, want process-restart interruption", last)
	}
	scenario := guiLifecycleScenario(t, "gui-process-kill-active-turn")
	observation := lifecycle.Observe(
		scenario.ID, scenario.Surface, scenario.Trigger, task,
		lifecycle.CompletionProjection{Required: true, Ready: result.Completion.Ready}, nil,
	)
	if violations := scenario.Check(observation); len(violations) != 0 {
		t.Fatalf("fresh GUI process-kill observation violations = %v", violations)
	}
}

type guiProcessKillRecoveryResult struct {
	SessionID     string                    `json:"session_id"`
	TaskStatus    taskstate.Status          `json:"task_status"`
	TaskRevision  uint64                    `json:"task_revision"`
	TurnSequence  uint64                    `json:"turn_sequence"`
	TurnState     taskstate.TurnState       `json:"turn_state"`
	TurnRoute     string                    `json:"turn_route"`
	EvidenceState string                    `json:"evidence_state"`
	StopReason    taskstate.StopReason      `json:"stop_reason"`
	Hypothesis    string                    `json:"hypothesis"`
	Completion    taskstate.CompletionCheck `json:"completion"`
}

func guiProcessKillResumeHelper(t *testing.T) {
	t.Helper()
	workspace := os.Getenv("PICOGENT_GUI_PROCESS_KILL_WORKSPACE")
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	ag := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	defer ag.Close()
	ag.SetTaskStore(taskstate.NewStore(os.Getenv("PICOGENT_GUI_PROCESS_KILL_TASK_DIR")))
	sessionID := os.Getenv("PICOGENT_GUI_PROCESS_KILL_SESSION")
	if err := ag.SetTaskSession(sessionID); err != nil {
		t.Fatal(err)
	}
	task := ag.TaskSnapshot()
	if task == nil {
		t.Fatal("fresh GUI process did not load durable task")
	}
	last := task.LastTurn()
	if last == nil || last.State != taskstate.TurnInterrupted || last.Route != string(taskstate.TurnRouteRecover) || last.EvidenceState != "UNVERIFIED" || last.StopReason != taskstate.StopProcessRestart || strings.TrimSpace(last.Hypothesis) == "" {
		t.Fatalf("fresh GUI process recovered turn = %#v, want process-restart recovery", last)
	}
	proof := agent.CompletionProof(task)
	result := guiProcessKillRecoveryResult{
		SessionID:     task.SessionID,
		TaskStatus:    task.Status,
		TaskRevision:  task.Revision,
		TurnSequence:  last.Sequence,
		TurnState:     last.State,
		TurnRoute:     last.Route,
		EvidenceState: last.EvidenceState,
		StopReason:    last.StopReason,
		Hypothesis:    last.Hypothesis,
		Completion:    proof,
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("PICOGENT_GUI_PROCESS_KILL_RESULT"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestGUIReconnectKeepsCurrentTaskProjection(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	store := taskstate.NewStore(t.TempDir())
	const sessionID = "gui-reconnect-active"
	task, err := taskstate.New(sessionID, "finish the requested change", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := task.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	if _, ok := task.BeginTurn(taskstate.TurnRouteImplement); !ok {
		t.Fatal("GUI reconnect fixture did not start an active turn")
	}
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	ag := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	defer ag.Close()
	ag.SetTaskStore(store)
	if err := ag.SetTaskSession(sessionID); err != nil {
		t.Fatal(err)
	}
	events := make(chan event, 4)
	s := &server{cfg: cfg, ag: ag, sessionID: sessionID, turnGen: 2, subs: []chan event{events}}

	stateResponse := httptest.NewRecorder()
	s.state(stateResponse, httptest.NewRequest(http.MethodGet, "/api/state", nil))
	if stateResponse.Code != http.StatusOK {
		t.Fatalf("GUI reconnect state status = %d", stateResponse.Code)
	}
	var state struct {
		SessionID  string                     `json:"session_id"`
		Task       *taskstate.Task            `json:"task"`
		Completion *taskstate.CompletionCheck `json:"completion"`
	}
	if err := json.Unmarshal(stateResponse.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.SessionID != sessionID || state.Task == nil || state.Task.ID != task.ID {
		t.Fatalf("reconnect state = %#v, want current task %q", state, task.ID)
	}
	if state.Completion == nil || state.Completion.Ready {
		t.Fatalf("reconnect completion = %#v, want fail-closed active projection", state.Completion)
	}

	stale := &guiHandler{s: s, sessionID: sessionID, turnGen: 1}
	stale.OnTaskState(task)
	select {
	case got := <-events:
		t.Fatalf("stale reconnect callback emitted %#v", got)
	default:
	}
	live := &guiHandler{s: s, sessionID: sessionID, turnGen: 2}
	live.OnTaskState(task)
	select {
	case got := <-events:
		if got.Type != "task_progress" || got.Task == nil || got.Task.ID != task.ID || got.turnGen != 2 {
			t.Fatalf("live reconnect callback = %#v", got)
		}
	default:
		t.Fatal("live reconnect callback did not emit")
	}

	scenario := guiLifecycleScenario(t, "gui-reconnect-active-turn")
	observation := lifecycle.Observe(
		scenario.ID, scenario.Surface, scenario.Trigger, state.Task,
		lifecycle.CompletionProjection{Required: true, Ready: state.Completion.Ready}, nil,
	)
	if violations := scenario.Check(observation); len(violations) != 0 {
		t.Fatalf("GUI reconnect observation violations = %v", violations)
	}
}

func TestGUITaskSaveFailureMatchesLifecycleScenario(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
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
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
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
	gate := perm.New(config.ModeFast, workspace, nil)
	// The direct integration fixture has no browser permission responder. Keep
	// the auto-verification step deterministic without weakening production
	// permission policy (verify still requires explicit approval by default).
	gate.AddAlwaysAllowed("verify")
	ag := agent.New(cfg, client, reg, gate)
	defer ag.Close()
	ag.SetTaskStore(goodStore)
	const sessionID = "gui-task-save-failure"
	if err := ag.SetTaskSession(sessionID); err != nil {
		t.Fatal(err)
	}
	events := make(chan event, 32)
	s := &server{cfg: cfg, ag: ag, sessionID: sessionID, turnGen: 1, permCh: make(chan perm.Decision, 1), subs: []chan event{events}}
	h := &guiTaskSaveFailureHandler{
		guiHandler: newGUIHandlerAtWithPerm(s, sessionID, 1, s.permCh),
		ag:         ag,
		badStore:   taskstate.NewStore(badRoot),
	}
	_, result, runErr := ag.RunWithOptions(context.Background(), nil, llm.Message{Role: "user", Content: "finish the requested change"}, h, agent.RunOptions{})
	if runErr == nil || !strings.Contains(strings.ToLower(runErr.Error()), "durable task state") {
		t.Fatalf("GUI task save failure = %v, want durable-state error", runErr)
	}
	if !h.switched || result.GoalDone || result.Completion.Ready {
		t.Fatalf("GUI save failure switched=%v goalDone=%v completion=%#v", h.switched, result.GoalDone, result.Completion)
	}
	task, err := goodStore.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	sawError := false
	for len(events) > 0 {
		if e := <-events; e.Type == "error" && strings.Contains(e.Text, "durable task state") {
			sawError = true
		}
	}
	if !sawError {
		t.Fatal("GUI task save failure did not emit a persistence error")
	}
	scenario := guiLifecycleScenario(t, "gui-task-save-failure")
	observation := lifecycle.Observe(
		scenario.ID, scenario.Surface, scenario.Trigger, task,
		lifecycle.CompletionProjection{Required: true}, runErr,
	)
	if violations := scenario.Check(observation); len(violations) != 0 {
		t.Fatalf("GUI task-save observation violations = %v", violations)
	}
}

func TestGUISessionSaveFailureMatchesLifecycleScenario(t *testing.T) {
	home := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(home, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PICOGENT_HOME", home)
	workspace := t.TempDir()
	store := taskstate.NewStore(t.TempDir())
	const sessionID = "gui-session-save-failure"
	task, err := taskstate.New(sessionID, "finish the requested change", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := task.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	if _, ok := task.BeginTurn(taskstate.TurnRouteImplement); !ok {
		t.Fatal("GUI session-save fixture did not start an active turn")
	}
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	ag := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	defer ag.Close()
	ag.SetTaskStore(store)
	if err := ag.SetTaskSession(sessionID); err != nil {
		t.Fatal(err)
	}
	events := make(chan event, 16)
	s := &server{
		cfg:       cfg,
		ag:        ag,
		sessionID: sessionID,
		hist:      []llm.Message{{Role: "user", Content: "request"}},
		permCh:    make(chan perm.Decision, 1),
		subs:      []chan event{events},
	}
	res := httptest.NewRecorder()
	s.reset(res, httptest.NewRequest(http.MethodPost, "/api/reset", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("GUI reset status = %d, want %d", res.Code, http.StatusOK)
	}
	saveError := ""
	for len(events) > 0 {
		if e := <-events; e.Type == "error" && strings.Contains(e.Text, "couldn't save session") {
			saveError = e.Text
			break
		}
	}
	if saveError == "" {
		t.Fatal("GUI session save failure did not emit a persistence error")
	}
	retained, err := store.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	scenario := guiLifecycleScenario(t, "gui-session-save-failure")
	observation := lifecycle.Observe(
		scenario.ID, scenario.Surface, scenario.Trigger, retained,
		lifecycle.CompletionProjection{Required: true}, errors.New(saveError),
	)
	if violations := scenario.Check(observation); len(violations) != 0 {
		t.Fatalf("GUI session-save observation violations = %v", violations)
	}
}

type guiTaskSaveFailureHandler struct {
	*guiHandler
	ag       *agent.Agent
	badStore *taskstate.Store
	switched bool
}

func (h *guiTaskSaveFailureHandler) OnTaskState(task *taskstate.Task) {
	if h.switched || task == nil {
		h.guiHandler.OnTaskState(task)
		return
	}
	last := task.LastTurn()
	if last == nil || last.State != taskstate.TurnActive || len(task.ChangedFiles) == 0 {
		h.guiHandler.OnTaskState(task)
		return
	}
	h.switched = true
	h.ag.SetTaskStore(h.badStore)
}

type guiProcessStateSnapshot struct {
	SessionID  string                     `json:"session_id"`
	Busy       bool                       `json:"busy"`
	Task       *taskstate.Task            `json:"task"`
	Completion *taskstate.CompletionCheck `json:"completion"`
}

const guiChildCleanupTimeout = 5 * time.Second

func waitGUIChildFor(wait <-chan error, timeout time.Duration) (error, bool) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-wait:
		return err, true
	case <-timer.C:
		return nil, false
	}
}

func waitGUIChannelFor(done <-chan struct{}, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

func guiChildExited(done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	default:
		return false
	}
}

// cleanupGUIChild is deliberately bounded. The lifecycle fixtures exercise a
// real child process and an intentionally blocked provider; an unbounded
// cleanup wait can otherwise turn one runner-sensitive teardown into the
// workflow's global ten-minute timeout and hide the useful failure.
func cleanupGUIChild(t *testing.T, cmd *exec.Cmd, wait <-chan error, childDone, stdoutDone <-chan struct{}, release func(), label string) {
	t.Helper()
	if release != nil {
		release()
	}
	if !guiChildExited(childDone) && cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	if !guiChildExited(childDone) {
		if _, ok := waitGUIChildFor(wait, guiChildCleanupTimeout); !ok {
			t.Errorf("%s did not exit within %s after forced cleanup", label, guiChildCleanupTimeout)
			return
		}
	}
	if !waitGUIChannelFor(stdoutDone, guiChildCleanupTimeout) {
		t.Errorf("%s stdout reader did not finish within %s", label, guiChildCleanupTimeout)
	}
}

func TestGUIChildWaitIsBounded(t *testing.T) {
	wait := make(chan error)
	started := time.Now()
	if _, ok := waitGUIChildFor(wait, 20*time.Millisecond); ok {
		t.Fatal("GUI child wait unexpectedly completed")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("GUI child wait took %s, want a bounded timeout", elapsed)
	}
}

func guiProcessState(t *testing.T, baseURL string) guiProcessStateSnapshot {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(5 * time.Second)
	for {
		res, err := client.Get(guiEndpoint(baseURL, "/api/state"))
		if err == nil {
			var state guiProcessStateSnapshot
			decodeErr := json.NewDecoder(res.Body).Decode(&state)
			res.Body.Close()
			if res.StatusCode == http.StatusOK && decodeErr == nil {
				return state
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("GUI state did not become available: err=%v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func guiProcessStateUntilActive(t *testing.T, baseURL string) guiProcessStateSnapshot {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(5 * time.Second)
	var latest guiProcessStateSnapshot
	for {
		res, err := client.Get(guiEndpoint(baseURL, "/api/state"))
		if err == nil {
			decodeErr := json.NewDecoder(res.Body).Decode(&latest)
			res.Body.Close()
			if res.StatusCode == http.StatusOK && decodeErr == nil && latest.Task != nil {
				last := latest.Task.LastTurn()
				if latest.Task.Status == taskstate.StatusWorking && last != nil && last.State == taskstate.TurnActive {
					return latest
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("GUI state did not expose a durable active turn: err=%v state=%#v", err, latest)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func guiEndpoint(baseURL, path string) string {
	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(path, "/")
}

func guiJSONRequest(t *testing.T, method, target, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, target, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if parsed, err := url.Parse(target); err == nil {
		req.Header.Set("Origin", parsed.Scheme+"://"+parsed.Host)
	}
	res, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func guiLifecycleScenario(t *testing.T, id string) lifecycle.Scenario {
	t.Helper()
	for _, scenario := range lifecycle.Scenarios() {
		if scenario.ID == id {
			return scenario
		}
	}
	t.Fatalf("lifecycle scenario %q not found", id)
	return lifecycle.Scenario{}
}
