package mcpbridge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"mime"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	maxVisualImages     = 4
	maxVisualImageBytes = 8 << 20
	maxVisualDimension  = 8192
)

// CallResult is the bounded result of a live MCP call. Text is already
// redacted/clipped; Images contains only decodable, bounded image content and
// is intended for the current in-memory model request, not persistence.
type CallResult struct {
	Text        string
	ResultError bool
	ImageCount  int
	Images      []ImageObservation
}

// ImageObservation describes one validated image without retaining any
// provider-controlled label as evidence. Data is ephemeral and is never copied
// into task or session state.
type ImageObservation struct {
	MIMEType string
	Format   string
	Width    int
	Height   int
	SHA256   string
	Data     []byte
}

// BrowserVisualObservation is returned only for a catalog-bound browser
// screenshot tool. The boolean says the tool identity was recognized; callers
// must still require ImageCount > 0 and no result error before treating it as a
// passing visual observation.
type BrowserVisualObservation struct {
	Reference   string
	ResultError bool
	ImageCount  int
	Images      []ImageObservation
}

func inspectCallResult(res *mcp.CallToolResult) CallResult {
	if res == nil {
		return CallResult{}
	}
	result := CallResult{
		Text:        formatResult(res),
		ResultError: res.IsError,
	}
	for _, content := range res.Content {
		imageContent, ok := content.(*mcp.ImageContent)
		if !ok {
			continue
		}
		observation, ok := validateImage(imageContent)
		if !ok {
			continue
		}
		result.ImageCount++
		if len(result.Images) < maxVisualImages {
			result.Images = append(result.Images, observation)
		}
	}
	return result
}

func validateImage(content *mcp.ImageContent) (ImageObservation, bool) {
	if content == nil || len(content.Data) == 0 || len(content.Data) > maxVisualImageBytes {
		return ImageObservation{}, false
	}
	mimeType, _, err := mime.ParseMediaType(strings.TrimSpace(content.MIMEType))
	if err != nil || !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		return ImageObservation{}, false
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(content.Data))
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > maxVisualDimension || config.Height > maxVisualDimension || format == "" {
		return ImageObservation{}, false
	}
	digest := sha256.Sum256(content.Data)
	return ImageObservation{
		MIMEType: strings.ToLower(mimeType),
		Format:   format,
		Width:    config.Width,
		Height:   config.Height,
		SHA256:   hex.EncodeToString(digest[:]),
		Data:     append([]byte(nil), content.Data...),
	}, true
}

// BrowserVisualEvidence recognizes only the curated BrowserOS/Puppeteer server
// identities and an explicitly screenshot-named tool. A generic browser-ish
// MCP description or a text result cannot enter the trusted visual producer.
func BrowserVisualEvidence(tool Tool, result CallResult) (BrowserVisualObservation, bool) {
	server := strings.ToLower(strings.TrimSpace(tool.Server))
	if server != "browseros" && server != "puppeteer" && server != "browser" {
		return BrowserVisualObservation{}, false
	}
	if !strings.Contains(strings.ToLower(strings.TrimSpace(tool.Original)), "screenshot") {
		return BrowserVisualObservation{}, false
	}
	observation := BrowserVisualObservation{
		Reference:   browserVisualReference(tool, result),
		ResultError: result.ResultError,
		ImageCount:  result.ImageCount,
		Images:      append([]ImageObservation(nil), result.Images...),
	}
	return observation, true
}

func browserVisualReference(tool Tool, result CallResult) string {
	server := compactMCPMetadata(tool.Server, 64)
	original := compactMCPMetadata(tool.Original, 96)
	if server == "" {
		server = "browser"
	}
	if original == "" {
		original = "screenshot"
	}
	reference := fmt.Sprintf("browser screenshot via %s/%s", server, original)
	for i, image := range result.Images {
		if i >= 2 || image.SHA256 == "" {
			break
		}
		reference += " sha256=" + image.SHA256
	}
	if result.ImageCount > len(result.Images) {
		reference += fmt.Sprintf(" (+%d image(s))", result.ImageCount-len(result.Images))
	}
	return reference
}
