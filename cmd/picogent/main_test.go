package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/goal"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/projects"
	"github.com/saiaathish/picogent/internal/scope"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
)

func TestVersion(t *testing.T) {
	if err := run([]string{"version"}); err != nil {
		t.Fatal(err)
	}
}

func TestHeadlessInvocationDoesNotInterceptGUIShutdown(t *testing.T) {
	if headlessInvocation([]string{"gui"}) || headlessInvocation([]string{"tui"}) || headlessInvocation(nil) {
		t.Fatal("GUI/TUI/default invocations must retain normal signal shutdown")
	}
	if !headlessInvocation([]string{"run", "say hello"}) || !headlessInvocation([]string{"--yes", "say hello"}) {
		t.Fatal("headless command forms must receive signal cancellation")
	}
}

func TestChooseScopeUsesNumberedChoiceAndDefault(t *testing.T) {
	p, ok := scope.Analyze("build something")
	if !ok {
		t.Fatal("expected scope prompt")
	}
	var output bytes.Buffer
	choice, proceed, err := chooseScopeContext(context.Background(), p, bufio.NewReader(strings.NewReader("2\n")), &output, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !proceed || choice.ID != "full" {
		t.Fatalf("choice = %#v, proceed=%v", choice, proceed)
	}
	choice, proceed, err = chooseScopeContext(context.Background(), p, bufio.NewReader(strings.NewReader("\n")), &output, nil)
	if err != nil {
		t.Fatal(err)
	}
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

func TestHeadlessDurableLoadFailureStopsBeforeRun(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)
	t.Setenv("PICOGENT_CODEX_HOME", t.TempDir())
	t.Setenv("PICOGENT_PROVIDER", "")
	t.Setenv("PICOGENT_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("PICOGENT_BASE_URL", "")
	t.Setenv("PICOGENT_ROUTER", "0")
	t.Setenv("PICOGENT_MODE", "")
	cfg := config.Default()
	cfg.SetupComplete = true
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	root, err := config.Dir()
	if err != nil {
		t.Fatal(err)
	}
	store := taskstate.NewStore(filepath.Join(root, "tasks", projects.IDForPath(workspace)))
	path, err := store.Path(headlessTaskSessionID("say hello"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	err = run([]string{"run", "--dir", workspace, "say hello"})
	if err == nil || !strings.Contains(err.Error(), "load durable task state") {
		t.Fatalf("headless run error = %v, want durable-load failure", err)
	}
	if exitCode(err) != 1 {
		t.Fatalf("headless durable-load exit code = %d, want 1", exitCode(err))
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

func TestHeadlessOutcomeClassificationUsesTaskEvidence(t *testing.T) {
	blocked := &taskstate.Task{Status: taskstate.StatusBlocked, BlockedBy: "permission needed"}
	if err := classifyHeadlessOutcome(context.Background(), "", agent.Result{Task: blocked}, nil); exitCode(err) != 2 {
		t.Fatalf("blocked outcome = %v, exit=%d; want exit 2", err, exitCode(err))
	}
	unverified := &taskstate.Task{
		Status:            taskstate.StatusVerifying,
		ChangedFiles:      []string{"note.txt"},
		ChangeSeq:         1,
		VerifiedChangeSeq: 0,
	}
	if err := classifyHeadlessOutcome(context.Background(), "", agent.Result{Task: unverified}, nil); exitCode(err) != 3 {
		t.Fatalf("unverified outcome = %v, exit=%d; want exit 3", err, exitCode(err))
	}
	if err := classifyHeadlessOutcome(context.Background(), "finish the project", agent.Result{}, nil); exitCode(err) != 3 {
		t.Fatalf("missing goal evidence = %v, exit=%d; want exit 3", err, exitCode(err))
	}
	if err := classifyHeadlessOutcome(context.Background(), "", agent.Result{}, errors.New("provider failed")); exitCode(err) != 1 {
		t.Fatalf("failed outcome = %v, exit=%d; want exit 1", err, exitCode(err))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := classifyHeadlessOutcome(ctx, "", agent.Result{}, nil); exitCode(err) != 130 {
		t.Fatalf("canceled outcome = %v, exit=%d; want exit 130", err, exitCode(err))
	}
}

func TestStdioSeparatesAnswerPromptsAndDiagnostics(t *testing.T) {
	var stdout, stderr bytes.Buffer
	h := &stdioHandler{
		in:     bufio.NewReader(strings.NewReader("y\n")),
		out:    &stdout,
		errOut: &stderr,
	}
	h.OnToolStart(llm.ToolCall{Name: "read_file", Arguments: `{"path":"note.txt"}`})
	h.OnToolEnd(llm.ToolCall{Name: "read_file"}, "contents", nil)
	decision, err := h.OnNeedPermission(context.Background(), perm.Request{Summary: "write note.txt"})
	if err != nil || decision != perm.Allow {
		t.Fatalf("permission = %s, %v; want allow", decision, err)
	}
	h.OnText("answer")
	if got := stdout.String(); got != "answer\n" {
		t.Fatalf("stdout = %q, want only final answer", got)
	}
	for _, want := range []string{"read_file", "contents", "Allow write note.txt?"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr = %q, missing %q", stderr.String(), want)
		}
	}
	h.OnError(errors.New("deferred failure"))
	if got := stderr.String(); strings.Contains(got, "deferred failure") {
		t.Fatalf("event errors must be emitted once by the command boundary, got %q", got)
	}
	if !strings.Contains(h.EventError().Error(), "deferred failure") {
		t.Fatalf("event error was not retained: %v", h.EventError())
	}
	var streamed bytes.Buffer
	streamHandler := &stdioHandler{out: &streamed, errOut: &stderr}
	streamHandler.OnTextDelta("partial answer")
	if streamed.Len() != 0 {
		t.Fatalf("partial streamed answer leaked before completion: %q", streamed.String())
	}
	streamHandler.OnTextFinal("complete answer")
	if got := streamed.String(); got != "complete answer\n" {
		t.Fatalf("completed stream = %q", got)
	}
	streamHandler.OnTextDelta("failed answer")
	streamHandler.OnError(errors.New("stream failed"))
	if strings.Contains(streamed.String(), "failed answer") {
		t.Fatalf("failed streamed answer leaked to stdout: %q", streamed.String())
	}
}

func TestStdioDiagnosticsRedactSecretsAndControlBytes(t *testing.T) {
	const (
		argumentSecret   = "headless-argument-secret"
		resultSecret     = "headless-result-secret"
		errorSecret      = "headless-error-secret"
		permissionSecret = "headless-permission-secret"
	)
	var stderr bytes.Buffer
	h := &stdioHandler{
		in:     bufio.NewReader(strings.NewReader("n\n")),
		errOut: &stderr,
	}
	h.OnToolStart(llm.ToolCall{
		Name:      "mcp\nforged",
		Arguments: "\x1b[31m{\"access_token\":\"" + argumentSecret + "\"}",
	})
	h.OnToolEnd(llm.ToolCall{Name: "mcp"}, `{"api_key":"`+resultSecret+`"}`, nil)
	h.OnToolEnd(llm.ToolCall{Name: "mcp"}, "", errors.New("tool failed\npassword="+errorSecret))
	_, _ = h.OnNeedPermission(context.Background(), perm.Request{Summary: "write token=" + permissionSecret})

	got := stderr.String()
	for _, secret := range []string{argumentSecret, resultSecret, errorSecret, permissionSecret} {
		if strings.Contains(got, secret) {
			t.Fatalf("diagnostic leaked secret %q: %q", secret, got)
		}
	}
	if strings.Contains(got, "\x1b") || strings.Contains(got, "mcp\nforged") {
		t.Fatalf("diagnostic retained terminal control or a forged line: %q", got)
	}
	if !strings.Contains(got, "mcp forged") || !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("diagnostic lost flattened source or redaction marker: %q", got)
	}
}

func TestDisplayErrorRedactsCredentialShapedCause(t *testing.T) {
	const secret = "headless-cause-secret"
	got := displayError(errors.New("provider failed: access_token=" + secret))
	if strings.Contains(got, secret) {
		t.Fatalf("display error leaked secret: %q", got)
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("display error lost redaction marker: %q", got)
	}
}

func TestDisplayErrorFlattensTerminalControl(t *testing.T) {
	const secret = "headless-terminal-secret"
	got := displayError(errors.New("provider failed\n\x1b[31mapi_key=" + secret))
	if strings.Contains(got, secret) {
		t.Fatalf("display error leaked secret: %q", got)
	}
	if strings.Contains(got, "\x1b") || strings.Contains(got, "\n") || strings.Contains(got, "[31m") {
		t.Fatalf("display error retained terminal control: %q", got)
	}
	if !strings.Contains(got, "provider failed") || !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("display error lost source or redaction marker: %q", got)
	}
}

func TestHeadlessUnverifiedErrorRedactsEvidence(t *testing.T) {
	const secret = "headless-verification-secret"
	err := newHeadlessUnverifiedError("verify output\napi_key=" + secret)
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("unverified diagnostic leaked secret: %q", err)
	}
	if !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("unverified diagnostic lost redaction marker: %q", err)
	}
}

func TestStdioPermissionReadHonorsCancellation(t *testing.T) {
	reader := &blockingPermissionReader{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	var stderr bytes.Buffer
	h := &stdioHandler{
		in:             bufio.NewReader(reader),
		errOut:         &stderr,
		interruptInput: reader.interrupt,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := h.OnNeedPermission(ctx, perm.Request{Summary: "write note.txt"})
		done <- err
	}()
	select {
	case <-reader.started:
	case <-time.After(time.Second):
		t.Fatal("permission reader did not reach its blocking barrier")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("permission cancellation = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("permission prompt did not stop after cancellation")
	}
}

type blockingPermissionReader struct {
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
	releaseOnce sync.Once
}

func (r *blockingPermissionReader) Read([]byte) (int, error) {
	r.startedOnce.Do(func() { close(r.started) })
	<-r.release
	return 0, io.EOF
}

func (r *blockingPermissionReader) interrupt() {
	r.releaseOnce.Do(func() { close(r.release) })
}

func TestHeadlessSubprocessStreamsAndExitCodes(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	codexHome := t.TempDir()
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, `{"error":{"message":"fake provider failure"}}`, http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"fake provider answer"}}]}`)
	}))
	defer server.Close()

	t.Setenv("PICOGENT_HOME", home)
	t.Setenv("PICOGENT_CODEX_HOME", codexHome)
	t.Setenv("PICOGENT_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("PICOGENT_PROVIDER", "")
	t.Setenv("PICOGENT_BASE_URL", "")
	t.Setenv("PICOGENT_ROUTER", "0")
	t.Setenv("PICOGENT_MODE", "")
	cfg := config.Default()
	cfg.SetupComplete = true
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOpenAI
	cfg.APIKey = "test-key"
	cfg.BaseURL = server.URL
	cfg.Model = "fake-model"
	cfg.Router.Enabled = false
	cfg.Router.UseLLMAdvisor = false
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "picogent")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = packageDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build headless binary: %v\n%s", err, out)
	}

	childEnv := os.Environ()
	runChild := func() (stdout, stderr bytes.Buffer, err error) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, "run", "--dir", workspace, "say hello")
		cmd.Env = childEnv
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err = cmd.Run()
		if err == nil && ctx.Err() != nil {
			err = ctx.Err()
		}
		return stdout, stderr, err
	}
	stdout, stderr, err := runChild()
	if err != nil {
		t.Fatalf("successful subprocess: %v\nstdout=%q\nstderr=%q", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "fake provider answer") {
		t.Fatalf("stdout = %q, missing provider answer", stdout.String())
	}
	if strings.Contains(stdout.String(), "Allow ") || strings.Contains(stdout.String(), "Problem:") {
		t.Fatalf("stdout contains prompt/diagnostic: %q", stdout.String())
	}
	if strings.Contains(stderr.String(), "Problem:") {
		t.Fatalf("successful stderr contains failure diagnostic: %q", stderr.String())
	}

	fail.Store(true)
	stdout, stderr, err = runChild()
	if err == nil {
		t.Fatalf("failing subprocess unexpectedly succeeded: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("failing subprocess error = %v, want exit 1", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("failed run wrote answer to stdout: %q", stdout.String())
	}
	if got := strings.Count(stderr.String(), "Problem: the model call failed."); got != 1 {
		t.Fatalf("stderr = %q, expected one model diagnostic, count=%d", stderr.String(), got)
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

func TestHeadlessClarifyCancelReturnsCanceledAndDoesNotPersistInferredGoal(t *testing.T) {
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

	err = run([]string{"run", "--clarify", "--dir", workspace, "fix all flaky tests and make CI green"})
	if err == nil || exitCode(err) != 130 {
		t.Fatalf("clarify cancellation = %v, exit=%d; want canceled/130", err, exitCode(err))
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
