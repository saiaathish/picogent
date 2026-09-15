package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/tools"
)

func assertTUIBroadAdmissionRequest(t *testing.T, request llm.ChatRequest) {
	t.Helper()
	for _, spec := range request.Tools {
		if spec.Name == "project_health" {
			t.Fatal("project_health remained advertised after TUI admission")
		}
	}
	var focus string
	for _, message := range request.Messages {
		if strings.HasPrefix(message.Content, "Internal outcome focus:") {
			focus = message.Content
		}
		if strings.Contains(message.Content, "picogent.project-health.v1") {
			t.Fatalf("raw project-health payload reached TUI model messages: %q", message.Content)
		}
	}
	if !strings.Contains(focus, "Health observation: status=ATTENTION") {
		t.Fatalf("TUI first model request did not receive fresh health focus: %q", focus)
	}
}

func TestSubmitBroadOutcomeUsesBoundedAdmission(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	client := &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "done"}}}}
	ag := agent.New(cfg, client, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	var emitted []tea.Msg
	m := &model{
		cfg:       cfg,
		ag:        ag,
		vp:        viewport.New(80, 20),
		h:         &handler{permCh: make(chan perm.Decision, 1), send: func(msg tea.Msg) { emitted = append(emitted, msg) }},
		sessionID: "tui-admission-surface",
	}

	cmd := m.submit("get this repo healthy")
	if cmd == nil {
		t.Fatal("TUI broad-outcome prompt did not start")
	}
	msg := cmd()
	done, ok := msg.(doneMsg)
	if !ok {
		t.Fatalf("TUI broad-outcome result = %T, want doneMsg", msg)
	}
	if done.err != nil {
		t.Fatalf("TUI broad-outcome turn failed: %v", done.err)
	}
	if len(client.Calls) != 1 {
		t.Fatalf("TUI model calls = %d, want 1", len(client.Calls))
	}
	assertTUIBroadAdmissionRequest(t, client.Calls[0])
	for _, message := range done.history {
		if strings.Contains(message.Content, "picogent.project-health.v1") {
			t.Fatalf("raw project-health payload reached TUI history: %q", message.Content)
		}
	}
	for _, msg := range emitted {
		if line, ok := msg.(logMsg); ok && (strings.Contains(line.Text, "project_health") || strings.Contains(line.Text, "picogent.project-health.v1")) {
			t.Fatalf("TUI exposed admission output: %#v", line)
		}
	}
}
