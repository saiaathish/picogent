package mcpbridge

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestInspectCallResultKeepsOnlyValidatedImages(t *testing.T) {
	data := testPNG(t)
	result := inspectCallResult(&mcp.CallToolResult{Content: []mcp.Content{
		&mcp.TextContent{Text: "screenshot ready"},
		&mcp.ImageContent{Data: data, MIMEType: "image/png"},
		&mcp.ImageContent{Data: []byte("not an image"), MIMEType: "image/png"},
		&mcp.AudioContent{Data: []byte("audio"), MIMEType: "audio/wav"},
	}})

	if result.Text != "screenshot ready" {
		t.Fatalf("text result = %q", result.Text)
	}
	if result.ImageCount != 1 || len(result.Images) != 1 {
		t.Fatalf("validated images = %d/%d, want 1/1", result.ImageCount, len(result.Images))
	}
	image := result.Images[0]
	if image.MIMEType != "image/png" || image.Format != "png" || image.Width != 2 || image.Height != 2 || image.SHA256 == "" {
		t.Fatalf("image observation = %+v", image)
	}
	if !bytes.Equal(image.Data, data) {
		t.Fatal("validated image data was not retained for the transient model request")
	}
}

func TestBrowserVisualEvidenceRequiresCatalogBoundScreenshotIdentity(t *testing.T) {
	data := testPNG(t)
	result := inspectCallResult(&mcp.CallToolResult{Content: []mcp.Content{
		&mcp.ImageContent{Data: data, MIMEType: "image/png"},
	}})

	good, ok := BrowserVisualEvidence(Tool{Server: "browseros", Original: "browser_take_screenshot"}, result)
	if !ok || good.ImageCount != 1 || len(good.Images) != 1 || !strings.Contains(good.Reference, "sha256=") {
		t.Fatalf("recognized browser screenshot = ok=%v observation=%+v", ok, good)
	}
	for _, tool := range []Tool{
		{Server: "unknown-browser", Original: "take_screenshot"},
		{Server: "browseros", Original: "browser_snapshot"},
		{Server: "browseros", Original: "read_page"},
	} {
		if _, ok := BrowserVisualEvidence(tool, result); ok {
			t.Fatalf("untrusted visual identity was recognized: %+v", tool)
		}
	}
}

func TestBrowserVisualEvidenceRecognizesFailedScreenshotButDoesNotPassIt(t *testing.T) {
	observation, ok := BrowserVisualEvidence(
		Tool{Server: "puppeteer", Original: "puppeteer_screenshot"},
		CallResult{ResultError: true},
	)
	if !ok || !observation.ResultError || observation.ImageCount != 0 {
		t.Fatalf("failed screenshot observation = ok=%v %+v", ok, observation)
	}
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 1, color.RGBA{B: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestCallDetailedPreservesExistingCallCompatibility(t *testing.T) {
	if _, err := (&Manager{}).Call(context.Background(), Tool{}, "{}"); err == nil {
		t.Fatal("unattached call unexpectedly succeeded")
	}
}
