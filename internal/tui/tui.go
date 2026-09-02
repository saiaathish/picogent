package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/app"
	"github.com/saiaathish/picogent/internal/codexauth"
	"github.com/saiaathish/picogent/internal/commands"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/evolve"
	"github.com/saiaathish/picogent/internal/goal"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/redact"
	"github.com/saiaathish/picogent/internal/scope"
	"github.com/saiaathish/picogent/internal/session"
	"github.com/saiaathish/picogent/internal/slash"
	"github.com/saiaathish/picogent/internal/taskstate"
)

var (
	ink    = lipgloss.Color("230")
	paper  = lipgloss.Color("94")
	warn   = lipgloss.Color("214")
	okCol  = lipgloss.Color("114")
	errCol = lipgloss.Color("203")
	muted  = lipgloss.Color("245")
	accent = lipgloss.Color("216")

	brandStyle = lipgloss.NewStyle().Bold(true).Foreground(ink).Background(paper).Padding(0, 2)
	chipSafe   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(warn).Padding(0, 1)
	chipFast   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(okCol).Padding(0, 1)
	chipOn     = lipgloss.NewStyle().Foreground(okCol).Bold(true)
	chipOff    = lipgloss.NewStyle().Foreground(errCol).Bold(true)
	metaStyle  = lipgloss.NewStyle().Foreground(muted)
	userStyle  = lipgloss.NewStyle().Bold(true).Foreground(accent)
	botStyle   = lipgloss.NewStyle().Foreground(ink)
	toolStyle  = lipgloss.NewStyle().Foreground(muted)
	errStyle   = lipgloss.NewStyle().Foreground(errCol)
	permStyle  = lipgloss.NewStyle().Border(lipgloss.ThickBorder()).BorderForeground(warn).Padding(1, 2).Bold(true)
	helpStyle  = lipgloss.NewStyle().Foreground(muted)
	inputBox   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(paper).Padding(0, 1)
)

const tuiRecoveryHelp = "/undo last turn · /resume recover"

type logLine struct{ Kind, Text string }

type permAskMsg struct {
	perm.Request
	turnID    uint64
	sessionID string
}
type logMsg struct {
	Kind      string
	Text      string
	turnID    uint64
	sessionID string
}
type taskProgressMsg struct {
	task       *taskstate.Task
	completion taskstate.CompletionCheck
	turnID     uint64
	sessionID  string
}
type doneMsg struct {
	history      []llm.Message
	result       agent.Result
	prompt       string
	goal         string
	goalRevision uint64
	err          error
	turnID       uint64
	sessionID    string
}

type evolveMsg struct {
	text      string
	turnID    uint64
	sessionID string
}

type handler struct {
	send      func(tea.Msg)
	permCh    chan perm.Decision
	turnID    uint64
	sessionID string
}

// sendMsg tags every event emitted by a turn. A canceled turn can still
// finish unwinding after the user starts another turn, so the model must be
// able to discard those late events without changing current UI state.
func (h *handler) sendMsg(msg tea.Msg) {
	if h == nil || h.send == nil {
		return
	}
	switch msg := msg.(type) {
	case logMsg:
		msg.turnID = h.turnID
		msg.sessionID = h.sessionID
		h.send(msg)
	case permAskMsg:
		msg.turnID = h.turnID
		msg.sessionID = h.sessionID
		h.send(msg)
	case taskProgressMsg:
		msg.turnID = h.turnID
		msg.sessionID = h.sessionID
		h.send(msg)
	default:
		h.send(msg)
	}
}

func (h *handler) OnText(text string) {
	if text == "" {
		return
	}
	h.sendMsg(logMsg{Kind: "assistant", Text: text})
}
func (h *handler) OnTextDelta(delta string) {
	h.sendMsg(logMsg{Kind: "assistant_delta", Text: delta})
}
func (h *handler) OnTextFinal(text string) {
	h.sendMsg(logMsg{Kind: "assistant_final", Text: text})
}
func (h *handler) OnToolStart(call llm.ToolCall) {
	h.sendMsg(logMsg{Kind: "tool", Text: "→  " + redact.Diagnostic(call.Name, 100) + "  " + redact.Diagnostic(call.Arguments, 100)})
}
func (h *handler) OnToolEnd(_ llm.ToolCall, result string, err error) {
	if err != nil {
		h.sendMsg(logMsg{Kind: "error", Text: "   " + redact.Diagnostic(err.Error(), 180)})
		return
	}
	h.sendMsg(logMsg{Kind: "tool", Text: "   " + redact.Diagnostic(result, 180)})
}
func (h *handler) OnNeedPermission(ctx context.Context, req perm.Request) (perm.Decision, error) {
	h.sendMsg(permAskMsg{Request: req})
	select {
	case <-ctx.Done():
		return perm.Deny, ctx.Err()
	case d := <-h.permCh:
		return d, nil
	}
}
func (h *handler) OnError(err error) {
	if err == nil {
		return
	}
	h.sendMsg(logMsg{Kind: "error", Text: redact.Diagnostic(err.Error(), 180)})
}
func (h *handler) OnTaskState(task *taskstate.Task) {
	h.sendMsg(taskProgressMsg{task: task, completion: agent.CompletionProof(task)})
}

type model struct {
	cfg        config.Config
	ag         *agent.Agent
	history    []llm.Message
	sessionID  string
	lines      []logLine
	vp         viewport.Model
	ta         textarea.Model
	busy       bool
	perm       *perm.Request
	h          *handler
	turnID     uint64
	width      int
	height     int
	cancel     context.CancelFunc
	task       *taskstate.Task
	completion taskstate.CompletionCheck
	turnMode   *agent.TaskMode
}

func Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return RunContext(ctx)
}

// RunContext lets an owning command or test control the lifetime of the TUI.
// The signal-backed Run wrapper keeps the standalone CLI behavior equivalent.
func RunContext(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg, a, err := app.LoadContext(ctx, ".")
	if err != nil {
		return err
	}
	m, err := newModel(cfg, a)
	if err != nil {
		a.Close()
		return err
	}
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))
	m.h.send = func(msg tea.Msg) { p.Send(msg) }
	err = runProgram(m, p.Run)
	if err != nil && ctx.Err() != nil && errors.Is(err, tea.ErrProgramKilled) {
		return nil
	}
	return err
}

// runProgram owns the final TUI cleanup boundary. Bubble Tea can return from
// an input quit, terminal error, or another program-level exit after the model
// has started work; cleanup must not depend on one particular key path.
func runProgram(m *model, run func() (tea.Model, error)) (err error) {
	defer func() {
		stopModelIfActive(m)
		if m != nil && m.ag != nil {
			m.ag.Close()
		}
	}()
	_, err = run()
	return err
}

func stopModelIfActive(m *model) {
	if m == nil || (!m.busy && m.cancel == nil && m.perm == nil) {
		return
	}
	m.stop()
}

func newModel(cfg config.Config, a *agent.Agent) (*model, error) {
	ta := textarea.New()
	ta.Placeholder = "What should Picogent do?"
	ta.Focus()
	ta.Prompt = ""
	ta.CharLimit = 12000
	ta.ShowLineNumbers = false
	ta.SetHeight(3)
	ta.KeyMap.InsertNewline.SetEnabled(false)
	vp := viewport.New(80, 20)
	h := &handler{permCh: make(chan perm.Decision, 1), send: func(tea.Msg) {}}
	sessionID, history, resumed := initialSession(cfg.Workspace)
	m := &model{cfg: cfg, ag: a, ta: ta, vp: vp, h: h, sessionID: sessionID, history: history}
	if a != nil {
		if err := a.SetTaskSession(m.sessionID); err != nil {
			return nil, fmt.Errorf("load durable task state: %w", err)
		}
		m.task = a.TaskSnapshot()
		m.completion = agent.CompletionProof(m.task)
	}
	m.lines = []logLine{{Kind: "system", Text: greeting(cfg, a)}}
	if resumed {
		m.lines = append(m.lines, logLine{Kind: "system", Text: "resumed " + m.sessionID})
	}
	if err := cfg.MissingAuth(); err != nil {
		m.lines = append(m.lines, logLine{Kind: "error", Text: err.Error()})
	}
	m.refresh()
	return m, nil
}

func initialSession(workspace string) (id string, history []llm.Message, resumed bool) {
	if prev, err := session.Latest(workspace); err == nil {
		return prev.ID, prev.Messages, true
	}
	return session.New(workspace).ID, nil, false
}

func greeting(cfg config.Config, a *agent.Agent) string {
	base := "picogent · " + string(cfg.Provider) + " · " + cfg.Model
	if cfg.Provider == config.ProviderCodex && codexauth.LoggedIn() {
		base = "Codex connected · " + cfg.Model
	}
	if a != nil && a.Tools != nil && a.Tools.HasMCP() {
		if mcp := a.Tools.MCPManagerSnapshot(); mcp != nil {
			base += fmt.Sprintf(" · %d MCP tools", len(mcp.Tools()))
		}
	}
	return base + " · type a task, or /help"
}

func (m *model) Init() tea.Cmd { return textarea.Blink }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ta.SetWidth(max(20, msg.Width-4))
		m.layout()
		return m, nil
	case tea.KeyMsg:
		if m.perm != nil {
			switch msg.String() {
			case "y", "Y", "enter":
				m.decide(perm.Allow)
			case "n", "N", "esc":
				m.decide(perm.Deny)
			case "a", "A":
				m.decide(perm.AllowTurn)
			case "l", "L":
				m.decideAlways()
			case "ctrl+c":
				m.decide(perm.Deny)
				m.stop()
				return m, tea.Quit
			}
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c":
			if m.busy {
				m.stop()
				m.lines = append(m.lines, logLine{Kind: "system", Text: "stopped. ctrl-c again to quit."})
				m.refresh()
				return m, nil
			}
			m.stop()
			return m, tea.Quit
		case "ctrl+d":
			if strings.TrimSpace(m.ta.Value()) == "" {
				m.stop()
				return m, tea.Quit
			}
		case "enter":
			line := strings.TrimSpace(m.ta.Value())
			m.ta.Reset()
			if line == "" || m.busy {
				return m, nil
			}
			return m, m.submit(line)
		}
	case permAskMsg:
		if !m.accepts(msg.turnID, msg.sessionID) {
			return m, nil
		}
		req := msg.Request
		req.Hint = redact.Diagnostic(req.Hint, 240)
		req.Summary = redact.Diagnostic(req.Summary, 240)
		m.perm = &req
		body := req.Summary
		if req.Hint != "" {
			body = req.Hint + "\n\n" + req.Summary
		}
		m.lines = append(m.lines, logLine{Kind: "perm", Text: "Allow " + body + "?"})
		m.layout()
		return m, nil
	case logMsg:
		if !m.accepts(msg.turnID, msg.sessionID) {
			return m, nil
		}
		if msg.Kind == "assistant_delta" {
			n := len(m.lines)
			if n > 0 && m.lines[n-1].Kind == "assistant" {
				m.lines[n-1].Text += msg.Text
			} else {
				m.lines = append(m.lines, logLine{Kind: "assistant", Text: msg.Text})
			}
		} else if msg.Kind == "assistant_final" {
			n := len(m.lines)
			if n > 0 && m.lines[n-1].Kind == "assistant" {
				m.lines[n-1].Text = msg.Text
			} else if msg.Text != "" {
				m.lines = append(m.lines, logLine{Kind: "assistant", Text: msg.Text})
			}
		} else {
			m.lines = append(m.lines, logLine{Kind: msg.Kind, Text: msg.Text})
		}
		m.refresh()
		return m, nil
	case taskProgressMsg:
		if !m.accepts(msg.turnID, msg.sessionID) {
			return m, nil
		}
		if msg.task != nil && msg.task.SessionID == m.sessionID {
			m.task = msg.task
			m.completion = msg.completion
			m.layout()
		}
		return m, nil
	case doneMsg:
		if !m.accepts(msg.turnID, msg.sessionID) {
			return m, nil
		}
		m.busy = false
		m.cancel = nil
		m.turnMode = nil
		if msg.history != nil {
			m.history = msg.history
			if err := session.SaveMessages(m.cfg.Workspace, m.sessionID, m.history); err != nil {
				m.lines = append(m.lines, logLine{Kind: "error", Text: fmt.Sprintf("couldn't save session: %v", err)})
			}
		}
		if msg.result.Task != nil && msg.result.Task.SessionID == m.sessionID {
			m.task = msg.result.Task
			m.completion = agent.CompletionProof(msg.result.Task)
		} else if msg.result.Task == nil {
			m.completion = agent.CompletionProof(nil)
		}
		completion := msg.result.CompletionGate(msg.goal)
		if strings.TrimSpace(msg.goal) != "" && completion.Marker && !completion.Ready {
			m.lines = append(m.lines, logLine{Kind: "system", Text: "Completion not proven: " + completion.Explanation()})
		}
		if completion.Ready && strings.TrimSpace(msg.goal) != "" && m.ag != nil {
			workspace := m.ag.ConfigSnapshot().Workspace
			currentGoal, currentRevision := m.ag.GoalStateSnapshot()
			if currentGoal == msg.goal && currentRevision == msg.goalRevision {
				if cleared, err := goal.ClearIfState(workspace, msg.goal, msg.goalRevision); err != nil {
					m.lines = append(m.lines, logLine{Kind: "error", Text: fmt.Sprintf("couldn't clear completed goal: %v", err)})
				} else if cleared {
					m.ag.SetGoalState("", 0)
				}
			}
		}
		if msg.err != nil && !strings.Contains(strings.ToLower(msg.err.Error()), "context canceled") {
			m.lines = append(m.lines, logLine{Kind: "error", Text: redact.Diagnostic(msg.err.Error(), 240)})
		}
		m.refresh()
		var cmds []tea.Cmd
		if msg.err == nil {
			cmds = append(cmds, m.reflectCmd(msg.prompt, msg.result, msg.turnID, msg.sessionID))
		}
		return m, tea.Batch(cmds...)
	case evolveMsg:
		if !m.accepts(msg.turnID, msg.sessionID) {
			return m, nil
		}
		if msg.text != "" {
			m.lines = append(m.lines, logLine{Kind: "system", Text: msg.text})
			m.refresh()
		}
		return m, nil
	}
	var cmds []tea.Cmd
	var cmd tea.Cmd
	m.ta, cmd = m.ta.Update(msg)
	cmds = append(cmds, cmd)
	m.vp, cmd = m.vp.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

// accepts reports whether an event belongs to the currently visible turn and
// session. Zero IDs are accepted for direct, untagged model messages used by
// local callers and backwards-compatible tests; all real agent events are
// tagged by handler.sendMsg.
func (m *model) accepts(turnID uint64, sessionID string) bool {
	if turnID == 0 && sessionID == "" {
		return true
	}
	return turnID == m.turnID && sessionID == m.sessionID
}

func (m *model) decide(d perm.Decision) {
	if m.h == nil {
		return
	}
	select {
	case m.h.permCh <- d:
	default:
	}
	m.perm = nil
	m.layout()
}

func (m *model) decideAlways() {
	tool := ""
	if m.perm != nil {
		tool = m.perm.Tool
	}
	if tool != "" && m.ag != nil && m.ag.Gate != nil {
		m.cfg.Extensions.AlwaysAllowTools = appendUniqueStr(m.cfg.Extensions.AlwaysAllowTools, tool)
		m.ag.Gate.SetAlwaysAllowed(m.cfg.Extensions.AlwaysAllowTools)
		_ = config.Save(m.cfg)
	}
	m.decide(perm.AllowAlways)
}

func appendUniqueStr(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

func (m *model) stop() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	if m.h != nil {
		permCh := m.h.permCh
		m.h.permCh = nil
		select {
		case permCh <- perm.Deny:
		default:
		}
	}
	m.perm = nil
	m.busy = false
	m.turnMode = nil
	// Invalidate events already queued by the canceled turn. The next turn
	// receives a later ID, so a late done/log/task event cannot clear or
	// overwrite its state.
	m.turnID++
}

func (m *model) autoApplyPrompt(prompt string) error {
	return m.autoApplyPromptWith(prompt, false)
}

func (m *model) autoApplyScopedPrompt(prompt string) error {
	return m.autoApplyPromptWith(prompt, true)
}

func (m *model) autoApplyPromptWith(prompt string, automaticScope bool) error {
	if m.ag == nil || !m.cfg.AutoTaskModeOn() || strings.HasPrefix(strings.TrimSpace(prompt), "/") {
		return nil
	}
	currentMode := m.ag.TaskModeSnapshot()
	currentGoal := m.ag.GoalSnapshot()
	dec := agent.InferAuto(prompt, currentMode, currentGoal)
	if automaticScope {
		dec = agent.InferAutomaticScope(prompt, currentMode, currentGoal)
	}
	if dec.GoalSet && dec.Goal != currentGoal {
		if revision, err := goal.SetState(m.cfg.Workspace, dec.Goal); err == nil {
			m.ag.SetGoalState(dec.Goal, revision)
		} else {
			return fmt.Errorf("couldn't save inferred goal: %w", err)
		}
	}
	if dec.TaskMode != currentMode {
		m.ag.SetTaskMode(dec.TaskMode)
	}
	return nil
}

func (m *model) autoApplyGoal(prompt string) error {
	if m.ag == nil || !m.cfg.AutoTaskModeOn() || strings.HasPrefix(strings.TrimSpace(prompt), "/") {
		return nil
	}
	currentGoal := m.ag.GoalSnapshot()
	dec := agent.InferAuto(prompt, m.ag.TaskModeSnapshot(), currentGoal)
	if dec.GoalSet && dec.Goal != currentGoal {
		if revision, err := goal.SetState(m.cfg.Workspace, dec.Goal); err == nil {
			m.ag.SetGoalState(dec.Goal, revision)
		} else {
			return fmt.Errorf("couldn't save inferred goal: %w", err)
		}
	}
	return nil
}

func (m *model) submit(line string) tea.Cmd {
	if strings.HasPrefix(line, "/") {
		kind, payload := slash.Resolve(m.cfg.Workspace, line)
		switch kind {
		case slash.Prompt:
			m.lines = append(m.lines, logLine{Kind: "user", Text: line})
			m.refresh()
			return m.runAgent(payload)
		case slash.Local:
			return m.slashLocal(payload)
		case slash.Unknown:
			return m.slash(line)
		}
	}
	if p, ok := scope.Analyze(line); ok {
		choice := scope.Recommended(p)
		prompt, applied := scope.Apply(line, p, choice.ID)
		if !applied {
			m.lines = append(m.lines, logLine{Kind: "error", Text: "couldn't apply the recommended scope"})
			m.refresh()
			return nil
		}
		m.lines = append(m.lines, logLine{Kind: "system", Text: scope.DefaultMessage(choice)})
		m.refresh()
		// The automatic boundary does not override the current or inferred task
		// mode. runAgentAs keeps plan/report/debug wording intact.
		return m.runAgentAs(prompt, line, scope.TurnBoundary(choice))
	}
	return m.runAgent(line)
}

func (m *model) runAgent(prompt string) tea.Cmd {
	return m.runAgentAsMode(prompt, prompt, nil, false, "")
}

func (m *model) runAgentAs(prompt, displayPrompt, scopeBoundary string) tea.Cmd {
	return m.runAgentAsMode(prompt, displayPrompt, nil, true, scopeBoundary)
}

func (m *model) runAgentAsMode(prompt, displayPrompt string, mode *agent.TaskMode, automaticScope bool, scopeBoundary string) tea.Cmd {
	m.busy = true
	if mode != nil && mode.Valid() {
		copyMode := *mode
		m.turnMode = &copyMode
	} else {
		m.turnMode = nil
	}
	// Infer persistent state only after this turn is admitted. A temporary scope
	// never overwrites the saved task-mode preference. Automatic inference may
	// update the live session mode; a new session restores m.cfg.TaskMode.
	var intentErr error
	if mode != nil {
		intentErr = m.autoApplyGoal(displayPrompt)
	} else if automaticScope {
		intentErr = m.autoApplyScopedPrompt(displayPrompt)
	} else {
		intentErr = m.autoApplyPrompt(displayPrompt)
	}
	if intentErr != nil {
		m.busy = false
		m.turnMode = nil
		m.lines = append(m.lines, logLine{Kind: "error", Text: intentErr.Error()})
		m.refresh()
		return nil
	}
	m.turnID++
	turnID := m.turnID
	sessionID := m.sessionID
	if !strings.HasPrefix(displayPrompt, "/") {
		if displayPrompt != prompt || len(m.lines) == 0 || m.lines[len(m.lines)-1].Text != displayPrompt {
			m.lines = append(m.lines, logLine{Kind: "user", Text: displayPrompt})
		}
		m.refresh()
	}
	permCh := make(chan perm.Decision, 1)
	if m.h != nil {
		m.h.permCh = permCh
	}
	var send func(tea.Msg)
	if m.h != nil {
		send = m.h.send
	}
	h := &handler{send: send, permCh: permCh, turnID: turnID, sessionID: sessionID}
	ag := m.ag
	hist := m.history
	runGoal := ""
	var runGoalRevision uint64
	if ag != nil {
		runGoal, runGoalRevision = ag.GoalStateSnapshot()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	return func() tea.Msg {
		hist, res, err := ag.RunWithOptions(ctx, hist, llm.Message{Role: "user", Content: prompt}, h, agent.RunOptions{TaskMode: mode, TracePrompt: displayPrompt, DurablePrompt: displayPrompt, ScopeBoundary: scopeBoundary})
		if prompt != displayPrompt {
			for i := len(hist) - 1; i >= 0; i-- {
				if hist[i].Role == "user" && hist[i].Content == prompt {
					hist[i].Content = displayPrompt
					break
				}
			}
		}
		return doneMsg{history: hist, result: res, prompt: displayPrompt, goal: runGoal, goalRevision: runGoalRevision, err: err, turnID: turnID, sessionID: sessionID}
	}
}

func (m *model) reflectCmd(prompt string, result agent.Result, turnID uint64, sessionID string) tea.Cmd {
	ws := m.cfg.Workspace
	ag := m.ag
	cfg := m.cfg
	return func() tea.Msg {
		sig := evolve.Signal{
			Workspace:     ws,
			UserPrompt:    prompt,
			AssistantText: result.Text,
			FilesChanged:  result.FilesChanged,
			ToolRounds:    result.ToolRounds,
			GoalDone:      result.GoalDone,
			Verified:      result.Verified,
		}
		if !evolve.WorthReflecting(sig) {
			return evolveMsg{turnID: turnID, sessionID: sessionID}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		client := ag.ClientSnapshot()
		if r, ok := client.(*llm.Router); ok && r.Backend != nil {
			client = r.Backend
		}
		model := evolve.LightModel(cfg.Model, cfg.RouterEcosystem(), cfg.FableAllowed())
		delta, err := evolve.Reflect(ctx, client, model, sig)
		if err != nil || delta.Message == "" {
			return evolveMsg{turnID: turnID, sessionID: sessionID}
		}
		app.RefreshMemory(ag, ws)
		return evolveMsg{text: delta.Message, turnID: turnID, sessionID: sessionID}
	}
}

func (m *model) slashLocal(payload string) tea.Cmd {
	switch {
	case payload == "clear":
		if err := m.startNewSession("cleared"); err != nil {
			m.lines = append(m.lines, logLine{Kind: "error", Text: err.Error()})
		}
	case payload == "compact":
		if len(m.history) > 16 {
			head := m.history[0]
			if head.Role != "system" {
				head = llm.Message{}
			}
			m.history = append([]llm.Message{head}, m.history[len(m.history)-15:]...)
		}
		m.lines = append(m.lines, logLine{Kind: "system", Text: "context compacted"})
	case payload == "undo":
		text, err := m.ag.UndoLastTurn()
		kind := "system"
		if err != nil {
			kind = "error"
			text = err.Error()
		} else if m.ag != nil {
			m.task = m.ag.TaskSnapshot()
			m.completion = agent.CompletionProof(m.task)
		}
		m.lines = append(m.lines, logLine{Kind: kind, Text: text})
	case payload == "status":
		st := fmt.Sprintf("safe/fast=%s task=%s provider=%s model=%s\n%s", m.cfg.Mode, agent.ParseTaskMode(m.cfg.TaskMode).Label(), m.cfg.Provider, m.cfg.Model, m.cfg.Workspace)
		if g := m.ag.GoalSnapshot(); g != "" {
			st += "\ngoal: " + g
		}
		if m.ag.Tools != nil && m.ag.Tools.HasMCP() {
			if mcp := m.ag.Tools.MCPManagerSnapshot(); mcp != nil {
				st += fmt.Sprintf("\n%d MCP tools", len(mcp.Tools()))
			}
		}
		m.lines = append(m.lines, logLine{Kind: "system", Text: st})
	case payload == "diff":
		m.lines = append(m.lines, logLine{Kind: "system", Text: slash.GitDiff(m.cfg.Workspace)})
	case strings.HasPrefix(payload, "memory:"):
		text := strings.TrimPrefix(payload, "memory:")
		if text == "" {
			text = "(no AGENTS.md / CLAUDE.md / .picogent/rules.md)"
		}
		m.lines = append(m.lines, logLine{Kind: "system", Text: text})
	case payload == "resume":
		m.stop()
		if prev, err := session.Latest(m.cfg.Workspace); err == nil {
			previous := m.sessionID
			if m.ag != nil {
				if err := m.ag.SetTaskSession(prev.ID); err != nil {
					_ = m.ag.SetTaskSession(previous)
					m.lines = append(m.lines, logLine{Kind: "error", Text: "couldn't resume session: " + err.Error()})
					break
				}
			}
			m.history = prev.Messages
			m.sessionID = prev.ID
			if m.ag != nil {
				m.task = m.ag.TaskSnapshot()
				m.completion = agent.CompletionProof(m.task)
			} else {
				m.task = nil
				m.completion = agent.CompletionProof(nil)
			}
			m.lines = append(m.lines, logLine{Kind: "system", Text: "resumed " + prev.ID})
		} else {
			m.lines = append(m.lines, logLine{Kind: "error", Text: "no saved session"})
		}
	case payload == "commands":
		cmds := commands.List(m.cfg.Workspace)
		if len(cmds) == 0 {
			m.lines = append(m.lines, logLine{Kind: "system", Text: "no custom commands (.claude/commands/*.md)"})
		} else {
			m.lines = append(m.lines, logLine{Kind: "system", Text: "custom: /" + strings.Join(cmds, "  /")})
		}
	case strings.HasPrefix(payload, "task:"):
		tm := agent.ParseTaskMode(strings.TrimPrefix(payload, "task:"))
		m.cfg.TaskMode = string(tm)
		m.ag.SetTaskMode(tm)
		_ = config.Save(m.cfg)
		m.lines = append(m.lines, logLine{Kind: "system", Text: "task mode: " + strings.ToLower(tm.Label())})
	case strings.HasPrefix(payload, "goal:set:"):
		text := strings.TrimPrefix(payload, "goal:set:")
		revision, err := goal.SetState(m.cfg.Workspace, text)
		if err != nil {
			m.lines = append(m.lines, logLine{Kind: "error", Text: fmt.Sprintf("couldn't save goal: %v", err)})
			m.refresh()
			return nil
		}
		m.ag.SetGoalState(text, revision)
		m.lines = append(m.lines, logLine{Kind: "user", Text: "/goal " + text})
		m.lines = append(m.lines, logLine{Kind: "system", Text: "goal set"})
		m.refresh()
		return m.runAgent(goal.WorkPrompt(text))
	case payload == "goal:show":
		g, _ := goal.Load(m.cfg.Workspace)
		if g == "" {
			m.lines = append(m.lines, logLine{Kind: "system", Text: "no active goal"})
		} else {
			m.lines = append(m.lines, logLine{Kind: "system", Text: "goal: " + g})
		}
	case payload == "goal:clear":
		if err := goal.Clear(m.cfg.Workspace); err != nil {
			m.lines = append(m.lines, logLine{Kind: "error", Text: fmt.Sprintf("couldn't clear goal: %v", err)})
			m.refresh()
			return nil
		}
		m.ag.SetGoalState("", 0)
		m.lines = append(m.lines, logLine{Kind: "system", Text: "goal cleared"})
	}
	m.refresh()
	return nil
}

func (m *model) slash(line string) tea.Cmd {
	parts := strings.Fields(line)
	cmd := strings.ToLower(parts[0])
	switch cmd {
	case "/quit", "/exit", "/q":
		m.stop()
		return tea.Quit
	case "/help":
		help := "Type what you want. Safe asks before edits.\nOptional: " + tuiRecoveryHelp + " · /commit /review /clear /quit"
		if m.ag.Tools != nil && m.ag.Tools.HasMCP() {
			if mcp := m.ag.Tools.MCPManagerSnapshot(); mcp != nil {
				help += fmt.Sprintf("\nConnected: %d MCP tools.", len(mcp.Tools()))
			}
		}
		m.lines = append(m.lines, logLine{Kind: "system", Text: help})
	case "/mcp":
		if m.ag.Tools == nil || !m.ag.Tools.HasMCP() {
			m.lines = append(m.lines, logLine{Kind: "system", Text: "no MCP tools connected — ask in chat to add one"})
			break
		}
		if mcp := m.ag.Tools.MCPManagerSnapshot(); mcp != nil {
			for _, line := range mcp.Report() {
				m.lines = append(m.lines, logLine{Kind: "system", Text: line})
			}
		}
	case "/safe":
		m.saveMode(config.ModeSafe)
	case "/fast":
		m.saveMode(config.ModeFast)
	case "/model":
		if len(parts) < 2 {
			m.lines = append(m.lines, logLine{Kind: "system", Text: "current model: " + m.cfg.Model})
			break
		}
		m.cfg.Model = parts[1]
		m.ag.SetModel(parts[1])
		m.lines = append(m.lines, logLine{Kind: "system", Text: "model: " + parts[1]})
	case "/provider":
		if len(parts) < 2 {
			m.lines = append(m.lines, logLine{Kind: "system", Text: "current provider: " + string(m.cfg.Provider)})
			break
		}
		switch strings.ToLower(parts[1]) {
		case "codex":
			m.cfg.Provider = config.ProviderCodex
			if m.cfg.Model == "gpt-4.1-mini" || m.cfg.Model == "qwen2.5-coder:7b" {
				m.cfg.Model = codexauth.DefaultModel()
			}
		case "ollama":
			m.cfg.Provider = config.ProviderOllama
			if m.cfg.Model == "gpt-4.1-mini" || strings.HasPrefix(m.cfg.Model, "gpt-5") {
				m.cfg.Model = "qwen2.5-coder:7b"
			}
		case "openai":
			m.cfg.Provider = config.ProviderOpenAI
		case "quadcode", "claude", "claude-code":
			m.cfg.Provider = config.ProviderQuadCode
			if strings.HasPrefix(m.cfg.Model, "gpt-") {
				m.cfg.Model = "claude-sonnet-5"
			}
		case "opencode", "zen", "opencode-go":
			m.cfg.Provider = config.ProviderOpenCode
			m.cfg.Model = ""
			m.cfg.Model = m.cfg.BackendModel()
		case "antigravity", "agy", "gemini":
			m.cfg.Provider = config.ProviderAntigravity
			m.cfg.Model = ""
			m.cfg.Model = m.cfg.BackendModel()
		default:
			m.lines = append(m.lines, logLine{Kind: "error", Text: "provider must be codex, claude-code, opencode, antigravity, ollama, or openai"})
			m.refresh()
			return nil
		}
		client, err := app.NewClient(m.cfg)
		if err != nil {
			m.lines = append(m.lines, logLine{Kind: "error", Text: err.Error()})
			break
		}
		m.ag.SetClient(client)
		m.ag.UpdateConfig(func(cfg *config.Config) { *cfg = m.cfg })
		_ = config.Save(m.cfg)
		m.lines = append(m.lines, logLine{Kind: "system", Text: "provider: " + string(m.cfg.Provider) + "  model: " + m.cfg.Model})
	case "/reset":
		if err := m.startNewSession("new session"); err != nil {
			m.lines = append(m.lines, logLine{Kind: "error", Text: err.Error()})
		}
	default:
		m.lines = append(m.lines, logLine{Kind: "error", Text: "unknown command " + cmd + "  (try /help)"})
	}
	m.refresh()
	return nil
}

func (m *model) saveMode(mode config.Mode) {
	next := m.cfg
	next.SetUserMode(mode)
	if err := config.Save(next); err != nil {
		m.lines = append(m.lines, logLine{Kind: "error", Text: "couldn't save mode: " + err.Error()})
		return
	}
	m.cfg = next
	if m.ag != nil {
		m.ag.UpdateConfig(func(cfg *config.Config) { cfg.SetUserMode(mode) })
	}
	message := "saved mode: " + string(mode)
	if m.cfg.ModeOverridden() {
		message += "; current run stays " + string(m.cfg.Mode) + " (PICOGENT_MODE)"
	}
	m.lines = append(m.lines, logLine{Kind: "system", Text: message})
}

// startNewSession drops the current chat and durable execution state before
// assigning a fresh session ID. A canceled turn may still emit late events,
// so stop invalidates its turn ID before the session switch.
func (m *model) startNewSession(message string) error {
	m.stop()
	previous := m.sessionID
	next := session.New(m.cfg.Workspace).ID
	if next == previous {
		// Session.New intentionally uses second precision. Ensure a rapid
		// /clear followed by /reset cannot reopen the current task state.
		next = fmt.Sprintf("%s-%d", next, time.Now().UnixNano())
	}
	if m.ag != nil {
		if err := m.ag.SetTaskSession(next); err != nil {
			_ = m.ag.SetTaskSession(previous)
			return fmt.Errorf("couldn't start new session: %w", err)
		}
	}
	m.history = nil
	m.sessionID = next
	if m.ag != nil {
		// Inferred task modes belong to the old conversation. Restore the
		// configured/manual baseline for this fresh session instead.
		m.ag.SetTaskMode(agent.ParseTaskMode(m.cfg.TaskMode))
		m.task = m.ag.TaskSnapshot()
		m.completion = agent.CompletionProof(m.task)
	} else {
		m.task = nil
		m.completion = agent.CompletionProof(nil)
	}
	m.lines = []logLine{{Kind: "system", Text: message}}
	return nil
}

func (m *model) layout() {
	headerH, permH, inputH := 2, 0, 6
	if formatTaskProgressWithProof(m.task, m.completion) != "" {
		headerH++
	}
	if m.perm != nil {
		permH = 6
	}
	m.vp.Width = max(m.width, 20)
	m.vp.Height = max(5, m.height-headerH-permH-inputH)
	m.refresh()
}

func (m *model) refresh() {
	var b strings.Builder
	for _, ln := range m.lines {
		switch ln.Kind {
		case "user":
			b.WriteString(userStyle.Render("YOU") + "\n" + ln.Text)
		case "assistant":
			b.WriteString(botStyle.Render("PICOGENT") + "\n" + ln.Text)
		case "error":
			b.WriteString(errStyle.Render(ln.Text))
		case "tool":
			b.WriteString(toolStyle.Render(ln.Text))
		default:
			b.WriteString(metaStyle.Render(ln.Text))
		}
		b.WriteString("\n\n")
	}
	m.vp.SetContent(b.String())
	m.vp.GotoBottom()
}

func (m *model) View() string {
	if m.width == 0 {
		return "picogent"
	}
	mode := chipSafe.Render(" SAFE ")
	if m.cfg.Mode == config.ModeFast {
		mode = chipFast.Render(" FAST ")
	}
	conn := chipOff.Render("not logged in")
	if m.cfg.Provider == config.ProviderCodex && codexauth.LoggedIn() {
		conn = chipOn.Render("Codex connected")
	} else if m.cfg.Provider == config.ProviderOllama {
		conn = chipOn.Render("Ollama")
	} else if m.cfg.APIKeyResolved() != "" {
		conn = chipOn.Render("API key")
	}
	left := brandStyle.Render("PICOGENT")
	taskChip := ""
	if m.turnMode != nil && m.turnMode.Valid() && *m.turnMode != agent.TaskAgent {
		taskChip = "  " + chipOn.Render(m.turnMode.Label()+" · this turn")
	} else if m.ag != nil && m.ag.TaskModeSnapshot().Valid() && m.ag.TaskModeSnapshot() != agent.TaskAgent {
		taskChip = "  " + chipOn.Render(m.ag.TaskModeSnapshot().Label())
	}
	if m.ag != nil && m.ag.GoalSnapshot() != "" {
		taskChip += "  " + chipOn.Render("goal")
	}
	right := fmt.Sprintf("%s%s  %s  %s", mode, taskChip, conn, metaStyle.Render(m.cfg.Model))
	if m.ag != nil && m.ag.Tools != nil && m.ag.Tools.HasMCP() {
		if mcp := m.ag.Tools.MCPManagerSnapshot(); mcp != nil {
			right += "  " + chipOn.Render(fmt.Sprintf("%d MCP", len(mcp.Tools())))
		}
	}
	head := lipgloss.JoinHorizontal(lipgloss.Center, left, "  ", right)
	ws := metaStyle.Render(clip(m.cfg.Workspace, max(20, m.width-4)))
	taskProgress := ""
	if text := formatTaskProgressWithProof(m.task, m.completion); text != "" {
		taskProgress = metaStyle.Render(clip(text, max(20, m.width-4)))
	}
	body := m.vp.View()
	permBox := ""
	if m.perm != nil {
		body := m.perm.Summary
		if m.perm.Hint != "" {
			body = m.perm.Hint + "\n\n" + body
		}
		permBox = permStyle.Width(max(m.width-4, 20)).Render(body + "?\n\n  [Y]  Yes      [N]  No      [A]  This turn      [L]  Always")
	}
	help := "enter send · ctrl-c stop/quit · /help · " + tuiRecoveryHelp
	if m.busy {
		help = "working…  ctrl-c stops this turn"
	}
	box := inputBox.Width(max(m.width-2, 20)).Render(m.ta.View())
	sections := []string{head, ws}
	if taskProgress != "" {
		sections = append(sections, taskProgress)
	}
	sections = append(sections, "", body, permBox, box, helpStyle.Render(help))
	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func formatTaskProgress(task *taskstate.Task) string {
	return formatTaskProgressWithProof(task, agent.CompletionProof(task))
}

func formatTaskProgressWithProof(task *taskstate.Task, proof taskstate.CompletionCheck) string {
	if task == nil {
		return ""
	}
	done := 0
	for _, step := range task.Steps {
		if step.Done {
			done++
		}
	}
	detail := "complete"
	if task.Status == taskstate.StatusBlocked && strings.TrimSpace(task.BlockedBy) != "" {
		detail = "blocked: " + strings.TrimSpace(task.BlockedBy)
	} else if task.CurrentStep >= 0 && task.CurrentStep < len(task.Steps) {
		detail = task.Steps[task.CurrentStep].Description
	} else if task.Status != taskstate.StatusDone {
		detail = task.Goal
	}
	files := "files"
	if len(task.ChangedFiles) == 1 {
		files = "file"
	}
	return fmt.Sprintf("task · %s · %d/%d · %s · %s · %d %s", task.Status, done, len(task.Steps), detail, completionProofLabel(proof), len(task.ChangedFiles), files)
}

func completionProofLabel(proof taskstate.CompletionCheck) string {
	if proof.Ready {
		return "proof ready"
	}
	reason := strings.TrimSpace(proof.Reason)
	if reason == "" {
		reason = "completion proof is incomplete"
	}
	return "proof pending: " + reason
}

func clip(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
