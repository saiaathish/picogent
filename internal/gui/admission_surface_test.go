package gui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/tools"
)

func assertGUIBroadAdmissionRequest(t *testing.T, request llm.ChatRequest) {
	t.Helper()
	for _, spec := range request.Tools {
		if spec.Name == "project_health" {
			t.Fatal("project_health remained advertised after GUI admission")
		}
	}
	var focus string
	for _, message := range request.Messages {
		if strings.HasPrefix(message.Content, "Internal outcome focus:") {
			focus = message.Content
		}
		if strings.Contains(message.Content, "picogent.project-health.v1") {
			t.Fatalf("raw project-health payload reached GUI model messages: %q", message.Content)
		}
	}
	if !strings.Contains(focus, "Health observation: status=ATTENTION") {
		t.Fatalf("GUI first model request did not receive fresh health focus: %q", focus)
	}
}

func TestChatBroadOutcomeUsesBoundedAdmission(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	client := &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "done"}}}}
	ag := agent.New(cfg, client, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	events := make(chan event, 32)
	s := &server{
		cfg:       cfg,
		ag:        ag,
		sessionID: "gui-admission-surface",
		permCh:    make(chan perm.Decision, 1),
		subs:      []chan event{events},
	}

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"prompt":"get this repo healthy"}`))
	s.chat(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusAccepted)
	}

	finished := make(chan struct{})
	go func() {
		s.waitForTurns()
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("GUI broad-outcome turn did not finish")
	}
	if len(client.Calls) == 0 {
		t.Fatal("GUI broad-outcome turn did not reach the model")
	}
	assertGUIBroadAdmissionRequest(t, client.Calls[0])

	s.mu.Lock()
	history := append([]llm.Message(nil), s.hist...)
	s.mu.Unlock()
	for _, message := range history {
		if strings.Contains(message.Content, "picogent.project-health.v1") {
			t.Fatalf("raw project-health payload reached GUI history: %q", message.Content)
		}
	}
	for {
		select {
		case e := <-events:
			if e.Type == "tool_start" || e.Type == "tool_end" || strings.Contains(e.Text, "project_health") || strings.Contains(e.Text, "picogent.project-health.v1") {
				t.Fatalf("GUI exposed admission event: %#v", e)
			}
		default:
			return
		}
	}
}
