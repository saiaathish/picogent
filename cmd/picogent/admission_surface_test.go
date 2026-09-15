package main

import (
	"bufio"
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/tools"
)

func assertHeadlessBroadAdmissionRequest(t *testing.T, request llm.ChatRequest) {
	t.Helper()
	for _, spec := range request.Tools {
		if spec.Name == "project_health" {
			t.Fatal("project_health remained advertised after headless admission")
		}
	}
	var focus string
	for _, message := range request.Messages {
		if strings.HasPrefix(message.Content, "Internal outcome focus:") {
			focus = message.Content
		}
		if strings.Contains(message.Content, "picogent.project-health.v1") {
			t.Fatalf("raw project-health payload reached headless model messages: %q", message.Content)
		}
	}
	if !strings.Contains(focus, "Health observation: status=ATTENTION") {
		t.Fatalf("headless first model request did not receive fresh health focus: %q", focus)
	}
}

func TestHeadlessBroadOutcomeUsesBoundedAdmission(t *testing.T) {
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	client := &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "done"}}}}
	ag := agent.New(cfg, client, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	h := &stdioHandler{
		in:     bufio.NewReader(strings.NewReader("")),
		out:    stdout,
		errOut: stderr,
	}

	result, err := runHeadlessAgent(context.Background(), ag, h, "get this repo healthy", agent.RunOptions{})
	if err != nil {
		t.Fatalf("headless broad-outcome turn failed: %v", err)
	}
	if result.Text != "done" {
		t.Fatalf("headless result text = %q, want done", result.Text)
	}
	if len(client.Calls) != 1 {
		t.Fatalf("headless model calls = %d, want 1", len(client.Calls))
	}
	assertHeadlessBroadAdmissionRequest(t, client.Calls[0])
	if strings.Contains(stdout.String(), "project_health") || strings.Contains(stdout.String(), "picogent.project-health.v1") || strings.Contains(stderr.String(), "project_health") || strings.Contains(stderr.String(), "picogent.project-health.v1") {
		t.Fatalf("headless exposed admission output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}
