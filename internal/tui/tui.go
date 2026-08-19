package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/app"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
)

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("63")).Padding(0, 1)
	modeSafe    = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	modeFast    = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	toolStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	permStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("11")).Padding(0, 1)
	helpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
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
	h.send(logMsg{Kind: "tool", Text: "→ " + call.Name + " " + clip(call.Arguments, 90)})
}
func (h *handler) OnToolEnd(_ llm.ToolCall, result string, err error) {
	if err != nil {
		h.send(logMsg{Kind: "error", Text: "  error: " + err.Error()})
		return
	}
	h.send(logMsg{Kind: "tool", Text: "  " + clip(result, 160)})
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
	vp viewport.Model
	ti textinput.Model
	busy    bool
	perm    *perm.Request
	h       *handler
	width   int
	height  int
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
	ti := textinput.New()
	ti.Placeholder = "what should picogent do?  /help"
	ti.Focus()
	ti.Prompt = "› "
	ti.CharLimit = 8000
	vp := viewport.New(80, 20)
	h := &handler{permCh: make(chan perm.Decision, 1), send: func(tea.Msg) {}}
	m := &model{
		cfg: cfg, ag: a, ti: ti, vp: vp, h: h,
		lines: []logLine{{Kind: "system", Text: "picogent · two modes · " + string(cfg.Mode) + " · type /help"}},
	}
	if err := cfg.MissingAuth(); err != nil {
		m.lines = append(m.lines, logLine{Kind: "error", Text: err.Error()})
	}
	m.refresh()
	return m
}

func (m *model) Init() tea.Cmd { return textinput.Blink }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ti.Width = max(20, msg.Width-2)
		headerH, permH, inputH := 1, 0, 3
		if m.perm != nil {
			permH = 4
		}
		m.vp.Width = msg.Width
		m.vp.Height = max(3, msg.Height-headerH-permH-inputH-1)
		m.refresh()
		return m, nil
	case tea.KeyMsg:
		if m.perm != nil {
			switch msg.String() {
			case "y", "Y":
				m.h.permCh <- perm.Allow
				m.perm = nil
				m.refresh()
			case "n", "N", "esc":
				m.h.permCh <- perm.Deny
				m.perm = nil
				m.refresh()
			case "a", "A":
				m.h.permCh <- perm.AllowTurn
				m.perm = nil
				m.refresh()
			}
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c":
			if m.perm != nil {
				select {
				case m.h.permCh <- perm.Deny:
				default:
				}
				m.perm = nil
			}
			return m, tea.Quit
		case "ctrl+d":
			if m.ti.Value() == "" {
				return m, tea.Quit
			}
		case "enter":
			line := strings.TrimSpace(m.ti.Value())
			m.ti.Reset()
			if line == "" || m.busy {
				return m, nil
			}
			return m, m.submit(line)
		}
	case permAskMsg:
		req := perm.Request(msg)
		m.perm = &req
		m.lines = append(m.lines, logLine{Kind: "perm", Text: "allow " + req.Summary + "?"})
		m.refresh()
		return m, nil
	case logMsg:
		m.lines = append(m.lines, logLine(msg))
		m.refresh()
		return m, nil
	case doneMsg:
		m.busy = false
		if msg.history != nil {
			m.history = msg.history
		}
		if msg.err != nil {
			m.lines = append(m.lines, logLine{Kind: "error", Text: msg.err.Error()})
		}
		m.refresh()
		return m, nil
	}
	var cmds []tea.Cmd
	var cmd tea.Cmd
	m.ti, cmd = m.ti.Update(msg)
	cmds = append(cmds, cmd)
	m.vp, cmd = m.vp.Update(msg)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func (m *model) submit(line string) tea.Cmd {
	if strings.HasPrefix(line, "/") {
		return m.slash(line)
	}
	m.busy = true
	m.lines = append(m.lines, logLine{Kind: "user", Text: "you: " + line})
	m.refresh()
	h := m.h
	ag := m.ag
	hist := m.history
	return func() tea.Msg {
		hist, _, err := ag.Run(context.Background(), hist, line, h)
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
		m.lines = append(m.lines, logLine{Kind: "system", Text: "/safe /fast /model [name] /provider ollama|openai /reset /quit\nSafe asks before writes and shell. Fast auto-edits inside this folder."})
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
		case "ollama":
			m.cfg.Provider = config.ProviderOllama
			if m.cfg.Model == "gpt-4.1-mini" {
				m.cfg.Model = "qwen2.5-coder:7b"
			}
		case "openai":
			m.cfg.Provider = config.ProviderOpenAI
		default:
			m.lines = append(m.lines, logLine{Kind: "error", Text: "provider must be ollama or openai"})
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

func (m *model) refresh() {
	var b strings.Builder
	for _, ln := range m.lines {
		switch ln.Kind {
		case "error":
			b.WriteString(errStyle.Render(ln.Text))
		case "tool":
			b.WriteString(toolStyle.Render(ln.Text))
		default:
			b.WriteString(ln.Text)
		}
		b.WriteByte('\n')
	}
	m.vp.SetContent(b.String())
	m.vp.GotoBottom()
}

func (m *model) View() string {
	mode := modeSafe.Render("safe")
	if m.cfg.Mode == config.ModeFast {
		mode = modeFast.Render("fast")
	}
	head := headerStyle.Width(max(m.width, 20)).Render(fmt.Sprintf("picogent  ·  %s  ·  %s  ·  %s", mode, m.cfg.Model, clip(m.cfg.Workspace, 40)))
	body := m.vp.View()
	permBox := ""
	if m.perm != nil {
		permBox = permStyle.Width(max(m.width-2, 20)).Render("Allow "+m.perm.Summary+"?\n[y] yes   [n] no   [a] yes for this turn")
	}
	help := helpStyle.Render("enter send · ctrl-c quit · /help")
	if m.busy {
		help = helpStyle.Render("working…  ctrl-c quit")
	}
	return lipgloss.JoinVertical(lipgloss.Left, head, body, permBox, m.ti.View(), help)
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
