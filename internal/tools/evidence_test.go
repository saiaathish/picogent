package tools

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/mcpbridge"
	"github.com/saiaathish/picogent/internal/perm"
)

func TestRunWithEvidenceCarriesOnlyCatalogBoundBrowserScreenshot(t *testing.T) {
	data := evidenceTestPNG(t)
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
	registry := NewRegistry(Context{Workspace: t.TempDir()})
	if err := registry.AttachMCP(manager); err != nil {
		t.Fatal(err)
	}
	defer registry.Close()

	tool, ok := registry.Get("mcp_browseros_take_screenshot")
	if !ok {
		t.Fatal("connected screenshot tool was not registered")
	}

	out, err, producer := RunWithEvidence(context.Background(), tool, "{}", registry.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	if out == "" {
		t.Fatal("screenshot call returned no bounded result")
	}
	if producer.Visual == nil || producer.Visual.ValidImageCount != 1 || len(producer.Visual.Parts) != 1 {
		t.Fatalf("typed visual producer = %+v", producer.Visual)
	}
	part := producer.Visual.Parts[0]
	if part.Type != "image" || part.MIME != "image/png" || len(part.Data) == 0 {
		t.Fatalf("transient screenshot part = %+v", part)
	}
	if !bytes.Equal(part.Data, data) {
		t.Fatal("typed producer changed the validated image bytes")
	}
	if producer.Visual.Reference == "" || !bytes.Contains([]byte(producer.Visual.Reference), []byte("sha256=")) {
		t.Fatalf("screenshot reference lacks digest binding: %q", producer.Visual.Reference)
	}
}

func TestRunWithEvidenceDoesNotTrustGenericToolOutput(t *testing.T) {
	tool := fakeEvidenceTool{}
	out, err, producer := RunWithEvidence(context.Background(), tool, "{}", Context{})
	if err != nil || out != "measure PASS benchmarks=1" || producer.Visual != nil {
		t.Fatalf("generic tool result = out=%q err=%v producer=%+v", out, err, producer)
	}
}

type fakeEvidenceTool struct{}

func (fakeEvidenceTool) Spec() llm.ToolSpec { return llm.ToolSpec{Name: "fake"} }

func (fakeEvidenceTool) Permission(string, Context) perm.Request { return perm.Request{} }

func (fakeEvidenceTool) Run(context.Context, string, Context) (string, error) {
	return "measure PASS benchmarks=1", nil
}

func evidenceTestPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{G: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
