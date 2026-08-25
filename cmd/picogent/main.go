package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/app"
	"github.com/saiaathish/picogent/internal/codexauth"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/goal"
	"github.com/saiaathish/picogent/internal/gui"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/mcpbridge"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/scope"
	"github.com/saiaathish/picogent/internal/setup"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tui"
	"github.com/saiaathish/picogent/internal/verify"
)

var version = "1.0.0"

// errHeadlessPermissionDenied is intentionally distinguishable from provider
// and tool failures. CI callers can treat exit code 2 as "blocked pending
// approval" without mistaking it for an agent crash.
var errHeadlessPermissionDenied = errors.New("Problem: permission denied.\nCause:   headless Safe mode could not approve the requested action.\nFix:     rerun interactively and approve it, or use --yes for non-destructive in-workspace work.")

type headlessOutcome uint8

const (
	headlessOutcomeBlocked headlessOutcome = iota + 1
	headlessOutcomeCanceled
	headlessOutcomeUnverified
)

// headlessOutcomeError keeps the CLI's machine-visible exit classification
// attached to the human-readable error without making the agent package know
// about command-line policy.
type headlessOutcomeError struct {
	outcome headlessOutcome
	cause   error
	message string
}

func (e *headlessOutcomeError) Error() string {
	if e == nil {
		return ""
	}
	if e.message != "" {
		return e.message
	}
	if e.cause != nil {
		return e.cause.Error()
	}
	return "headless run did not complete"
}

func (e *headlessOutcomeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func main() {
	args := os.Args[1:]
	var err error
	if headlessInvocation(args) {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		err = runContext(ctx, args)
		stop()
	} else {
		err = run(args)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(exitCode(err))
	}
}

func headlessInvocation(args []string) bool {
	return len(args) > 0 && (args[0] == "run" || strings.HasPrefix(args[0], "-"))
}

func exitCode(err error) int {
	var outcome *headlessOutcomeError
	if errors.As(err, &outcome) {
		switch outcome.outcome {
		case headlessOutcomeBlocked:
			return 2
		case headlessOutcomeCanceled:
			return 130
		case headlessOutcomeUnverified:
			return 3
		}
	}
	if errors.Is(err, errHeadlessPermissionDenied) {
		return 2
	}
	return 1
}

func run(args []string) error {
	return runContext(context.Background(), args)
}

func runContext(ctx context.Context, args []string) error {
	if len(args) == 0 {
		if config.NeedsSetup() {
			return gui.Run()
		}
		return tui.Run()
	}
	switch args[0] {
	case "gui":
		return gui.Run()
	case "setup":
		return gui.RunSetup()
	case "tui":
		return tui.Run()
	case "run":
		return runOnceContext(ctx, args[1:])
	case "init":
		return runInitArgs(args[1:])
	case "login":
		return runLogin(args[1:])
	case "mcp":
		return runMCP(args[1:])
	case "version", "-v", "--version":
		fmt.Println("picogent", version)
		return nil
	case "help", "-h", "--help":
		printHelp()
		return nil
	default:
		if strings.HasPrefix(args[0], "-") {
			return runOnceContext(ctx, args)
		}
		printHelp()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printHelp() {
	fmt.Print(`picogent — tiny coding agent

Usage:
  picogent              first run: install cores + browser setup; later: TUI
  picogent gui          browser chat (setup first if needed)
  picogent setup        open the browser setup again
  picogent tui          terminal UI
  picogent run "..."    one-shot prompt (headless)
  picogent login        connect ChatGPT Codex (~/.codex/auth.json)
  picogent login claude connect Claude Code CLI (subscription, no API key)
  picogent login opencode connect OpenCode Zen/Go (opencode auth login)
  picogent login antigravity connect Antigravity CLI (agy)
  picogent mcp          list connected MCP tools
  picogent init         write ~/.picogent/config.yaml
  picogent version

No extra app. The GUI is a local page in your browser.

Default backend is your Codex subscription (same auth file as Codex CLI).
Claude Code / OpenCode / Antigravity reuse their CLI logins.

Init flags:
  --ollama              use local Ollama
  --model NAME          default model
  --base-url URL        OpenAI-compatible base URL

Run flags:
  --dir PATH            workspace (default: current directory)
  --yes                 Fast mode and auto-approve in-workspace writes/shell
  --model NAME          override model
  --clarify             ask a quick scope question for broad prompts
`)
}

func runLogin(args []string) error {
	target := "codex"
	if len(args) > 0 {
		target = strings.ToLower(args[0])
	}
	switch target {
	case "claude", "quadcode", "anthropic":
		return runClaudeLogin()
	case "opencode", "zen", "opencode-go", "go":
		return runOpenCodeLogin()
	case "antigravity", "agy", "gemini":
		return runAntigravityLogin()
	case "codex", "chatgpt", "":
		return runCodexLogin()
	default:
		return fmt.Errorf("unknown login target %q (try: picogent login | login claude | login opencode | login antigravity)", args[0])
	}
}

func runCodexLogin() error {
	codex, err := exec.LookPath("codex")
	if err != nil {
		return fmt.Errorf("Problem: Codex CLI is not installed.\nCause:   `codex` is not on PATH.\nFix:     install the Codex CLI, then run picogent login")
	}
	cmd := exec.Command(codex, "login")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	if !codexauth.LoggedIn() {
		return fmt.Errorf("login finished, but ~/.codex/auth.json is still empty")
	}
	fmt.Println("Codex connected. Run picogent or picogent gui.")
	return nil
}

func runClaudeLogin() error {
	if err := setup.StartClaudeLogin(); err != nil {
		return err
	}
	if setup.ClaudeLoggedIn() {
		fmt.Println("Claude Code already connected. In Settings pick provider Claude Code.")
		return nil
	}
	fmt.Println("Finish Claude login in the window that opened, then pick Claude Code in Settings.")
	return nil
}

func runOpenCodeLogin() error {
	if err := setup.StartOpenCodeLogin(); err != nil {
		return err
	}
	fmt.Println("Finish OpenCode auth (Zen and/or Go) in the window that opened, then pick OpenCode in Settings.")
	return nil
}

func runAntigravityLogin() error {
	if err := setup.StartAntigravityLogin(); err != nil {
		return err
	}
	fmt.Println("Finish Antigravity Google sign-in in the CLI window, then pick Antigravity in Settings.")
	return nil
}

func runMCP(args []string) error {
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}
	if sub != "list" {
		return fmt.Errorf("unknown mcp subcommand %q (try: picogent mcp list)", sub)
	}
	wd, _ := os.Getwd()
	servers, err := mcpbridge.LoadServers(wd)
	if err != nil {
		return err
	}
	if len(servers) == 0 {
		fmt.Println("No MCP servers configured.")
		fmt.Println("Add ~/.picogent/mcp.yaml or use ~/.cursor/mcp.json (same format as Cursor).")
		return nil
	}
	fmt.Printf("Configured servers (%d):\n", len(servers))
	for name := range servers {
		fmt.Println(" ", name)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	mgr, warns := mcpbridge.ConnectBestEffort(ctx, servers)
	defer mgr.Close()
	for _, w := range warns {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	lines := mgr.Report()
	if len(lines) == 0 {
		fmt.Println("No tools connected (servers may be offline).")
		return nil
	}
	fmt.Println("\nConnected tools:")
	for _, line := range lines {
		fmt.Println(line)
	}
	return nil
}

func runInitArgs(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	ollama := fs.Bool("ollama", false, "use local Ollama")
	model := fs.String("model", "", "model name")
	base := fs.String("base-url", "", "OpenAI-compatible base URL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg := config.Default()
	if existing, err := config.Load(); err == nil {
		cfg = existing
	}
	if *ollama {
		cfg.Provider = config.ProviderOllama
		if *model == "" {
			*model = "qwen2.5-coder:7b"
		}
	}
	if *model != "" {
		cfg.Model = *model
	}
	if *base != "" {
		cfg.BaseURL = *base
		cfg.Provider = config.ProviderOpenAI
	}
	if err := config.Save(cfg); err != nil {
		return err
	}
	path, _ := config.Path()
	fmt.Println("wrote", path)
	switch cfg.Provider {
	case config.ProviderOllama:
		fmt.Println("next: ollama serve && ollama pull", cfg.Model)
	case config.ProviderCodex:
		if codexauth.LoggedIn() {
			fmt.Println("Codex already connected via ~/.codex/auth.json")
			fmt.Println("next: picogent   or   picogent gui")
		} else {
			fmt.Println("next: picogent login")
		}
	default:
		fmt.Println("next: export PICOGENT_API_KEY=sk-...   (or set api_key in the yaml)")
	}
	return nil
}

func runOnceContext(ctx context.Context, args []string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return newHeadlessCanceledError(err)
	}
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dir := fs.String("dir", ".", "workspace")
	yes := fs.Bool("yes", false, "auto-approve in-workspace writes and shell")
	model := fs.String("model", "", "model override")
	clarify := fs.Bool("clarify", false, "ask a quick scope question for broad prompts")
	if err := fs.Parse(args); err != nil {
		return err
	}
	prompt := strings.Join(fs.Args(), " ")
	originalPrompt := prompt
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("Problem: missing prompt.\nCause:   picogent run needs text.\nFix:     picogent run \"create hello.txt\"")
	}
	cfg, a, err := app.LoadContext(ctx, *dir)
	if err != nil {
		return err
	}
	if *model != "" {
		cfg.Model = *model
		a.SetModel(*model)
	}
	if *yes {
		applyHeadlessYes(&cfg, a)
	}
	// Headless turns do not have a chat UI session, but they still need the
	// same durable execution checkpoint as TUI/GUI turns.  A stable, prompt-
	// keyed id lets an interrupted identical invocation resume without making
	// unrelated prompts share progress.
	a.SetTaskSession(headlessTaskSessionID(originalPrompt))
	input := bufio.NewReader(os.Stdin)
	interruptInput := func() { _ = os.Stdin.Close() }
	h := &stdioHandler{yes: *yes, in: input, out: os.Stdout, errOut: os.Stderr, interruptInput: interruptInput}
	if preflight, ok := scope.Analyze(prompt); ok {
		choice := scope.Recommended(preflight)
		if *clarify {
			var proceed bool
			var err error
			choice, proceed, err = chooseScopeContext(ctx, preflight, input, os.Stderr, interruptInput)
			if err != nil {
				return newHeadlessCanceledError(err)
			}
			if !proceed {
				return newHeadlessCanceledError(context.Canceled)
			}
		}
		if applied, ok := scope.Apply(prompt, preflight, choice.ID); ok {
			prompt = applied
		}
		mode := scopeModeForHeadlessTurn(a, cfg, originalPrompt, choice, *clarify)
		if err := applyHeadlessGoalInference(a, cfg, originalPrompt); err != nil {
			return err
		}
		runGoal, runGoalRevision := a.GoalStateSnapshot()
		runGoal = strings.TrimSpace(runGoal)
		scopeBoundary := scope.TurnBoundary(choice)
		result, err := runHeadlessAgent(ctx, a, h, prompt, agent.RunOptions{TaskMode: mode, TracePrompt: originalPrompt, DurablePrompt: originalPrompt, ScopeBoundary: scopeBoundary, SuppressUndo: true})
		if outcomeErr := classifyHeadlessOutcome(ctx, runGoal, result, err); outcomeErr != nil {
			return outcomeErr
		}
		return clearHeadlessGoalAfterCompletion(a, cfg, runGoal, runGoalRevision, result)
	}
	automaticMode := autoModeForHeadlessTurn(a, cfg, originalPrompt)
	if err := applyHeadlessGoalInference(a, cfg, originalPrompt); err != nil {
		return err
	}
	runGoal, runGoalRevision := a.GoalStateSnapshot()
	runGoal = strings.TrimSpace(runGoal)
	result, err := runHeadlessAgent(ctx, a, h, prompt, agent.RunOptions{
		TaskMode:      automaticMode,
		TracePrompt:   originalPrompt,
		DurablePrompt: originalPrompt,
		SuppressUndo:  true,
	})
	if outcomeErr := classifyHeadlessOutcome(ctx, runGoal, result, err); outcomeErr != nil {
		return outcomeErr
	}
	return clearHeadlessGoalAfterCompletion(a, cfg, runGoal, runGoalRevision, result)
}

func runHeadlessAgent(ctx context.Context, a *agent.Agent, h *stdioHandler, prompt string, opts agent.RunOptions) (agent.Result, error) {
	_, result, err := a.RunWithOptions(ctx, nil, llm.Message{Role: "user", Content: prompt}, h, opts)
	if err == nil {
		err = h.EventError()
	}
	return result, err
}

func classifyHeadlessOutcome(ctx context.Context, expectedGoal string, result agent.Result, runErr error) error {
	if ctx != nil && ctx.Err() != nil {
		return newHeadlessCanceledError(ctx.Err())
	}
	if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
		return newHeadlessCanceledError(runErr)
	}
	if result.Task != nil && result.Task.Status == taskstate.StatusBlocked {
		reason := strings.TrimSpace(result.Task.BlockedBy)
		if reason == "" {
			reason = "the agent could not take the next safe action"
		}
		return &headlessOutcomeError{
			outcome: headlessOutcomeBlocked,
			cause:   runErr,
			message: fmt.Sprintf("Problem: headless run is blocked.\nCause:   %s.\nFix:     resolve the blocker, then rerun the same command to resume the task.", reason),
		}
	}
	if errors.Is(runErr, errHeadlessPermissionDenied) {
		return &headlessOutcomeError{outcome: headlessOutcomeBlocked, cause: runErr}
	}
	if runErr != nil {
		return runErr
	}
	if result.Task != nil && result.Task.NeedsVerification() {
		return newHeadlessUnverifiedError(result.Verified)
	}
	if len(result.FilesChanged) > 0 && verify.StatusFromEvidence(result.Verified) != verify.StatusPass {
		return newHeadlessUnverifiedError(result.Verified)
	}
	if strings.TrimSpace(expectedGoal) != "" && !result.GoalDone {
		return newHeadlessUnverifiedError(result.Verified)
	}
	return nil
}

func newHeadlessCanceledError(cause error) error {
	if cause == nil {
		cause = context.Canceled
	}
	return &headlessOutcomeError{
		outcome: headlessOutcomeCanceled,
		cause:   cause,
		message: "Problem: headless run canceled.\nCause:   the current turn was interrupted before completion.\nFix:     rerun the same command to resume the durable task.",
	}
}

func newHeadlessUnverifiedError(evidence string) error {
	reason := "no passing verification evidence was recorded"
	if evidence = strings.TrimSpace(evidence); evidence != "" {
		reason = "the latest verification was not a passing result: " + short(evidence, 240)
	}
	return &headlessOutcomeError{
		outcome: headlessOutcomeUnverified,
		message: fmt.Sprintf("Problem: headless task is not verified.\nCause:   %s.\nFix:     rerun the same command to continue until the requested outcome has passing evidence.", reason),
	}
}

// applyHeadlessGoalInference mirrors the GUI/TUI automatic intent path. The
// inferred goal is persisted before the model runs so an interrupted one-shot
// invocation can resume with the same durable intent.
func applyHeadlessGoalInference(a *agent.Agent, cfg config.Config, prompt string) error {
	if a == nil || !cfg.AutoTaskModeOn() {
		return nil
	}
	current := a.GoalSnapshot()
	decision := agent.InferAuto(prompt, a.TaskModeSnapshot(), current)
	if !decision.GoalSet || decision.Goal == current {
		return nil
	}
	revision, err := goal.SetState(cfg.Workspace, decision.Goal)
	if err != nil {
		return fmt.Errorf("couldn't save inferred goal: %w", err)
	}
	a.SetGoalState(decision.Goal, revision)
	return nil
}

// clearHeadlessGoalAfterCompletion removes a persisted goal only after the
// agent reports a completion marker backed by its verification gate.
func clearHeadlessGoalAfterCompletion(a *agent.Agent, cfg config.Config, expectedGoal string, expectedRevision uint64, result agent.Result) error {
	expectedGoal = strings.TrimSpace(expectedGoal)
	if a == nil || !result.GoalDone || expectedGoal == "" {
		return nil
	}
	currentGoal, currentRevision := a.GoalStateSnapshot()
	if currentGoal != expectedGoal || currentRevision != expectedRevision {
		return nil
	}
	cleared, err := goal.ClearIfState(cfg.Workspace, expectedGoal, expectedRevision)
	if err != nil {
		return fmt.Errorf("couldn't clear completed goal: %w", err)
	}
	currentGoal, currentRevision = a.GoalStateSnapshot()
	if cleared && currentGoal == expectedGoal && currentRevision == expectedRevision {
		a.SetGoalState("", 0)
	}
	return nil
}

// autoModeForHeadlessTurn mirrors the lightweight GUI/TUI mode inference for
// prompts that do not trigger a scope preflight. It is per-turn only; the
// saved PICOGENT_MODE/task-mode preference remains untouched.
func autoModeForHeadlessTurn(a *agent.Agent, cfg config.Config, prompt string) *agent.TaskMode {
	if a == nil || !cfg.AutoTaskModeOn() {
		return nil
	}
	current := a.TaskModeSnapshot()
	decision := agent.InferAuto(prompt, current, a.GoalSnapshot())
	if decision.TaskMode == current {
		return nil
	}
	mode := decision.TaskMode
	return &mode
}

// scopeModeForHeadlessTurn keeps an automatic scope boundary separate from
// task intent. Only an explicit --clarify selection overrides the active mode;
// otherwise the existing lightweight intent inference remains free to honor
// wording such as "plan it first" or "inspect and report".
func scopeModeForHeadlessTurn(a *agent.Agent, cfg config.Config, prompt string, choice scope.Choice, explicit bool) *agent.TaskMode {
	if explicit {
		mode := agent.ScopeTaskMode(choice.ID)
		return &mode
	}
	if a == nil || !cfg.AutoTaskModeOn() {
		return nil
	}
	current := a.TaskModeSnapshot()
	decision := agent.InferAutomaticScope(prompt, current, a.GoalSnapshot())
	if decision.TaskMode == current {
		return nil
	}
	mode := decision.TaskMode
	return &mode
}

// applyHeadlessYes enables Fast mode for this invocation without changing the
// saved preference. Destructive actions still receive a separate hard deny.
func applyHeadlessYes(cfg *config.Config, a *agent.Agent) {
	if cfg == nil {
		return
	}
	cfg.SetRuntimeMode(config.ModeFast)
	if a != nil {
		a.UpdateConfig(func(current *config.Config) { current.SetRuntimeMode(config.ModeFast) })
	}
}

func headlessTaskSessionID(prompt string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(prompt)))
	return "headless-" + hex.EncodeToString(digest[:8])
}

type lineReadResult struct {
	line string
	err  error
}

func readLineContext(ctx context.Context, in *bufio.Reader, interrupt func()) (string, error) {
	if in == nil {
		return "", io.EOF
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	read := make(chan lineReadResult, 1)
	go func() {
		line, err := in.ReadString('\n')
		read <- lineReadResult{line: line, err: err}
	}()
	select {
	case result := <-read:
		return result.line, result.err
	case <-ctx.Done():
		if interrupt != nil {
			interrupt()
		}
		return "", ctx.Err()
	}
}

func chooseScopeContext(ctx context.Context, p scope.Prompt, in *bufio.Reader, out io.Writer, interrupt func()) (scope.Choice, bool, error) {
	choice := scope.Recommended(p)
	if out == nil {
		out = os.Stderr
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Quick question:", p.Question)
	for i, c := range p.Choices {
		recommended := ""
		if c.Recommended {
			recommended = " (recommended)"
		}
		fmt.Fprintf(out, "  %d. %s%s — %s\n", i+1, c.Label, recommended, c.Why)
	}
	fmt.Fprint(out, "Choose 1-", len(p.Choices), " (Enter for recommended, type esc to cancel): ")
	line, err := readLineContext(ctx, in, interrupt)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return choice, false, err
	}
	if err != nil {
		return choice, true, nil
	}
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "esc" || line == "cancel" {
		fmt.Fprintln(out, "Canceled.")
		return scope.Choice{}, false, nil
	}
	if line == "" {
		return choice, true, nil
	}
	for i, c := range p.Choices {
		if line == fmt.Sprint(i+1) {
			return c, true, nil
		}
	}
	fmt.Fprintln(out, "Using the recommended option.")
	return choice, true, nil
}

type stdioHandler struct {
	yes            bool
	in             *bufio.Reader
	out            io.Writer
	errOut         io.Writer
	interruptInput func()
	errMu          sync.Mutex
	eventErr       error
	streamMu       sync.Mutex
	stream         strings.Builder
}

func (h *stdioHandler) OnText(text string) {
	h.discardStream()
	out := h.stdout()
	if text == "" {
		fmt.Fprintln(out)
		return
	}
	fmt.Fprintln(out, text)
}
func (h *stdioHandler) OnTextDelta(delta string) {
	if delta == "" {
		return
	}
	h.streamMu.Lock()
	h.stream.WriteString(delta)
	h.streamMu.Unlock()
}
func (h *stdioHandler) OnTextFinal(text string) {
	h.OnText(text)
}
func (h *stdioHandler) OnToolStart(call llm.ToolCall) {
	h.discardStream()
	fmt.Fprintf(h.stderr(), "→ %s %s\n", call.Name, short(call.Arguments, 80))
}
func (h *stdioHandler) OnToolEnd(_ llm.ToolCall, result string, err error) {
	if err != nil {
		fmt.Fprintln(h.stderr(), "  error:", err)
		return
	}
	fmt.Fprintln(h.stderr(), " ", short(result, 120))
}
func (h *stdioHandler) OnNeedPermission(ctx context.Context, req perm.Request) (perm.Decision, error) {
	if h.yes && !req.Destructive && !req.OutsideWorkspace {
		return perm.Allow, nil
	}
	if h.yes && (req.Destructive || req.OutsideWorkspace) {
		return perm.Deny, errHeadlessPermissionDenied
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return perm.Deny, err
	}
	fmt.Fprintf(h.stderr(), "Allow %s? [y/n] ", req.Summary)
	if h.in == nil {
		return perm.Deny, errHeadlessPermissionDenied
	}
	line, err := readLineContext(ctx, h.in, h.interruptInput)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return perm.Deny, err
		}
		return perm.Deny, errHeadlessPermissionDenied
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "y") {
		return perm.Allow, nil
	}
	return perm.Deny, errHeadlessPermissionDenied
}
func (h *stdioHandler) OnError(err error) {
	h.discardStream()
	if err == nil {
		return
	}
	h.errMu.Lock()
	if h.eventErr == nil {
		h.eventErr = err
	}
	h.errMu.Unlock()
}

func (h *stdioHandler) discardStream() {
	if h == nil {
		return
	}
	h.streamMu.Lock()
	h.stream.Reset()
	h.streamMu.Unlock()
}

func (h *stdioHandler) EventError() error {
	if h == nil {
		return nil
	}
	h.errMu.Lock()
	defer h.errMu.Unlock()
	return h.eventErr
}

func (h *stdioHandler) stdout() io.Writer {
	if h != nil && h.out != nil {
		return h.out
	}
	return os.Stdout
}

func (h *stdioHandler) stderr() io.Writer {
	if h != nil && h.errOut != nil {
		return h.errOut
	}
	return os.Stderr
}

func short(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
