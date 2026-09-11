package agent

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/mcpbridge"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
)

func TestAgentBindsLiveBrowserScreenshotOnlyToNextModelRequest(t *testing.T) {
	data := agentVisualTestPNG(t)
	server := mcp.NewServer(&mcp.Implementation{Name: "browser-fixture", Version: "v1"}, nil)
	server.AddTool(&mcp.Tool{
		Name:        "take_screenshot",
		Description: "Capture the current browser viewport.",
		InputSchema: map[string]any{"type": "object"},
	}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.Content{
			&mcp.ImageContent{Data: data, MIMEType: "image/png"},
		}}, nil
	})
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{JSONResponse: true}))
	defer httpServer.Close()

	manager, warnings := mcpbridge.ConnectBestEffort(context.Background(), map[string]mcpbridge.ServerConfig{
		"browseros": {URL: httpServer.URL, Type: "http"},
	})
	defer manager.Close()
	if len(warnings) != 0 {
		t.Fatalf("MCP connection warnings = %v", warnings)
	}

	dir := t.TempDir()
	registry := tools.NewRegistry(tools.Context{Workspace: dir})
	if err := registry.AttachMCP(manager); err != nil {
		t.Fatal(err)
	}
	defer registry.Close()
	client := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{
			ID: "screenshot", Name: "mcp_browseros_take_screenshot", Arguments: "{}",
		}}}},
		{Message: llm.Message{Role: "assistant", Content: "I inspected the live screenshot."}},
	}}
	cfg := config.Default()
	cfg.Workspace = dir
	cfg.Provider = config.ProviderOllama
	a := New(cfg, client, registry, perm.New(config.ModeFast, dir, nil))
	a.TaskStore = taskstate.NewStore(t.TempDir())
	if err := a.SetTaskSession("visual-evidence"); err != nil {
		t.Fatal(err)
	}

	history, result, err := a.Run(context.Background(), nil, llm.Message{
		Role:    "user",
		Content: "review the browser UI visually",
	}, allowVisualEvidence{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Task == nil || result.Task.Intent == nil || !result.Task.Intent.NeedsVisual {
		t.Fatalf("visual intent/task missing: task=%#v", result.Task)
	}
	status, current, origin := result.Task.RequirementEvidenceState(taskstate.EvidenceKindVisual)
	if status != "PASS" || !current || origin != taskstate.EvidenceOriginBrowser {
		t.Fatalf("visual evidence = status=%q current=%v origin=%q task=%#v", status, current, origin, result.Task)
	}
	reloaded, err := a.TaskStore.Load("visual-evidence")
	if err != nil {
		t.Fatal(err)
	}
	if status, current, origin := reloaded.RequirementEvidenceState(taskstate.EvidenceKindVisual); status != "PASS" || current || origin != taskstate.EvidenceOriginBrowser {
		t.Fatalf("reloaded visual evidence = status=%q current=%v origin=%q", status, current, origin)
	}

	foundImage := false
	foundRefreshedFocus := false
	for _, call := range client.Calls {
		for _, message := range call.Messages {
			if message.Role == "system" && strings.Contains(message.Content, "Completion gaps: criteria=0,1,2,3 requirements=none") {
				foundRefreshedFocus = true
			}
			if len(message.Parts) == 0 {
				continue
			}
			if message.Role != "user" || message.Parts[0].Type != "image" || !bytes.Equal(message.Parts[0].Data, data) {
				t.Fatalf("unexpected visual message = %+v", message)
			}
			foundImage = true
		}
	}
	if !foundImage {
		t.Fatal("next model request did not receive the live screenshot")
	}
	if !foundRefreshedFocus {
		t.Fatal("next model request did not receive the refreshed durable outcome focus")
	}
	for _, message := range history {
		if len(message.Parts) > 0 {
			t.Fatalf("transient screenshot leaked into returned history: %+v", message)
		}
	}
}

type allowVisualEvidence struct{ NopHandler }

func (allowVisualEvidence) OnNeedPermission(context.Context, perm.Request) (perm.Decision, error) {
	return perm.Allow, nil
}

func agentVisualTestPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(1, 0, color.RGBA{R: 255, B: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
