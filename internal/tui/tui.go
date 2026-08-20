package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/app"
	"github.com/saiaathish/picogent/internal/codexauth"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
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

type logLine struct{ Kind, Text string }

type permAskMsg perm.Request
type logMsg logLine
type doneMsg struct {
	history []llm.Message
	err     error
}

type handler struct {
	send   func(tea.Msg)
	permCh chan perm.Decision
}

func (h *handler) OnText(text string) { h.send(logMsg{Kind: "assistant", Text: text}) }
func (h *handler) OnToolStart(call llm.ToolCall) {
	h.send(logMsg{Kind: "tool", Text: "→  " + call.Name + "  " + clip(call.Arguments, 100)})
}
func (h *handler) OnToolEnd(_ llm.ToolCall, result string, err error) {
	if err != nil {
		h.send(logMsg{Kind: "error", Text: "   " + err.Error()})
		return
	}
	h.send(logMsg{Kind: "tool", Text: "   " + clip(result, 180)})
}
func (h *handler) OnNeedPermission(ctx context.Context, req perm.Request) (perm.Decision, error) {
	h.send(permAskMsg(req))
	select {
	case <-ctx.Done():
		return perm.Deny, ctx.Err()
	case d := <-h.permCh:
		return d, nil
	}
}
func (h *handler) OnError(err error) { h.send(logMsg{Kind: "error", Text: err.Error()}) }

type model struct {
	cfg     config.Config
	ag      *agent.Agent
	history []llm.Message
	lines   []logLine
	vp      viewport.Model
	ta      textarea.Model
	busy    bool
	perm    *perm.Request
	h       *handler
	width   int
	height  int
	cancel  context.CancelFunc
}

func Run() error {
	cfg, a, err := app.Load(".")
	if err != nil {
		return err
	}
	m := newModel(cfg, a)
	p := tea.NewProgram(m, tea.WithAltScreen())
	m.h.send = func(msg tea.Msg) { p.Send(msg) }
	_, err = p.Run()
	return err
}

func newModel(cfg config.Config, a *agent.Agent) *model {
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
	m := &model{cfg: cfg, ag: a, ta: ta, vp: vp, h: h}
	m.lines = []logLine{{Kind: "system", Text: greeting(cfg)}}
	if err := cfg.MissingAuth(); err != nil {
		m.lines = append(m.lines, logLine{Kind: "error", Text: err.Error()})
	}
	m.refresh()
	return m
}

func greeting(cfg config.Config) string {
	if cfg.Provider == config.ProviderCodex && codexauth.LoggedIn() {
		return "Codex connected · " + cfg.Model + " · type a task, or /help"
	}
	return "picogent · " + string(cfg.Provider) + " · " + cfg.Model + " · type a task, or /help"
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
			return m, tea.Quit
		case "ctrl+d":
			if strings.TrimSpace(m.ta.Value()) == "" {
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
		req := perm.Request(msg)
		m.perm = &req
		m.lines = append(m.lines, logLine{Kind: "perm", Text: "Allow " + req.Summary + "?"})
		m.layout()
		return m, nil
	case logMsg:
		m.lines = append(m.lines, logLine(msg))
		m.refresh()
		return m, nil
	case doneMsg:
		m.busy = false
		m.cancel = nil
		if msg.history != nil {
			m.history = msg.history
		}
		if msg.err != nil && !strings.Contains(strings.ToLower(msg.err.Error()), "context canceled") {
			m.lines = append(m.lines, logLine{Kind: "error", Text: msg.err.Error()})
		}
		m.refresh()
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

func (m *model) decide(d perm.Decision) {
	select {
	case m.h.permCh <- d:
	default:
	}
	m.perm = nil
	m.layout()
}

func (m *model) stop() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	select {
	case m.h.permCh <- perm.Deny:
	default:
	}
	m.perm = nil
	m.busy = false
}

func (m *model) submit(line string) tea.Cmd {
	if strings.HasPrefix(line, "/") {
		return m.slash(line)
	}
	m.busy = true
	m.lines = append(m.lines, logLine{Kind: "user", Text: line})
	m.refresh()
	h := m.h
	ag := m.ag
	hist := m.history
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	return func() tea.Msg {
		hist, _, err := ag.Run(ctx, hist, line, h)
		return doneMsg{history: hist, err: err}
	}
}

func (m *model) slash(line string) tea.Cmd {
	parts := strings.Fields(line)
	cmd := strings.ToLower(parts[0])
	switch cmd {
	case "/quit", "/exit", "/q":
		return tea.Quit
	case "/help":
		m.lines = append(m.lines, logLine{Kind: "system", Text: "enter send · y/n allow tools · /safe /fast /model [name] /provider codex|ollama|openai /reset /quit\nSafe asks before writes and shell. Fast auto-edits inside this folder. Codex uses ~/.codex/auth.json."})
	case "/safe":
		m.cfg.Mode = config.ModeSafe
		m.ag.CFG.Mode = config.ModeSafe
		m.ag.Gate.Mode = config.ModeSafe
		m.lines = append(m.lines, logLine{Kind: "system", Text: "mode: safe"})
	case "/fast":
		m.cfg.Mode = config.ModeFast
		m.ag.CFG.Mode = config.ModeFast
		m.ag.Gate.Mode = config.ModeFast
		m.lines = append(m.lines, logLine{Kind: "system", Text: "mode: fast"})
	case "/model":
		if len(parts) < 2 {
			m.lines = append(m.lines, logLine{Kind: "system", Text: "current model: " + m.cfg.Model})
			break
		}
		m.cfg.Model = parts[1]
		m.ag.CFG.Model = parts[1]
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
		default:
			m.lines = append(m.lines, logLine{Kind: "error", Text: "provider must be codex, ollama, or openai"})
			m.refresh()
			return nil
		}
		client, err := app.NewClient(m.cfg)
		if err != nil {
			m.lines = append(m.lines, logLine{Kind: "error", Text: err.Error()})
			break
		}
		m.ag.LLM = client
		m.ag.CFG = m.cfg
		_ = config.Save(m.cfg)
		m.lines = append(m.lines, logLine{Kind: "system", Text: "provider: " + string(m.cfg.Provider) + "  model: " + m.cfg.Model})
	case "/reset":
		m.history = nil
		m.lines = []logLine{{Kind: "system", Text: "new session"}}
	default:
		m.lines = append(m.lines, logLine{Kind: "error", Text: "unknown command " + cmd + "  (try /help)"})
	}
	m.refresh()
	return nil
}

func (m *model) layout() {
	headerH, permH, inputH := 2, 0, 6
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
	right := fmt.Sprintf("%s  %s  %s", mode, conn, metaStyle.Render(m.cfg.Model))
	head := lipgloss.JoinHorizontal(lipgloss.Center, left, "  ", right)
	ws := metaStyle.Render(clip(m.cfg.Workspace, max(20, m.width-4)))
	body := m.vp.View()
	permBox := ""
	if m.perm != nil {
		permBox = permStyle.Width(max(m.width-4, 20)).Render("Allow " + m.perm.Summary + "?\n\n  [Y]  Yes      [N]  No      [A]  Yes for this turn")
	}
	help := "enter send · ctrl-c stop/quit · /help"
	if m.busy {
		help = "working…  ctrl-c stops this turn"
	}
	box := inputBox.Width(max(m.width-2, 20)).Render(m.ta.View())
	return lipgloss.JoinVertical(lipgloss.Left, head, ws, "", body, permBox, box, helpStyle.Render(help))
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
