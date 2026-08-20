package attachments

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/saiaathish/picogent/internal/llm"
)

const MaxFiles = 8
const MaxBytes = 10 << 20 // 10 MiB per file

var allowedMIME = map[string]bool{
	"image/png": true, "image/jpeg": true, "image/jpg": true, "image/gif": true, "image/webp": true,
	"application/pdf": true,
	"text/plain": true, "text/markdown": true, "text/csv": true,
	"application/json": true, "application/xml": true, "text/xml": true,
}

var allowedExt = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true,
	".pdf": true, ".txt": true, ".md": true, ".csv": true, ".json": true,
	".xml": true, ".yaml": true, ".yml": true, ".go": true, ".js": true, ".ts": true,
	".tsx": true, ".jsx": true, ".py": true, ".rs": true, ".html": true, ".css": true,
	".toml": true, ".sql": true, ".sh": true,
}

// Input is one attachment from the chat API (base64 payload).
type Input struct {
	Name string `json:"name"`
	MIME string `json:"mime"`
	Data string `json:"data"`
}

// Decode validates and decodes API attachment inputs.
func Decode(in []Input) ([]llm.Part, error) {
	if len(in) > MaxFiles {
		return nil, fmt.Errorf("too many attachments (max %d)", MaxFiles)
	}
	out := make([]llm.Part, 0, len(in))
	for _, a := range in {
		if strings.TrimSpace(a.Data) == "" {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(a.Data)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid base64", a.Name)
		}
		if len(raw) > MaxBytes {
			return nil, fmt.Errorf("%s: file too large (max %d MB)", a.Name, MaxBytes>>20)
		}
		mime := strings.TrimSpace(a.MIME)
		if mime == "" {
			mime = mimeFromName(a.Name)
		}
		if !allowed(mime, a.Name) {
			return nil, fmt.Errorf("%s: unsupported file type", a.Name)
		}
		typ := "file"
		if strings.HasPrefix(mime, "image/") {
			typ = "image"
		}
		out = append(out, llm.Part{Type: typ, MIME: mime, Name: a.Name, Data: raw})
	}
	return out, nil
}

func allowed(mime, name string) bool {
	if allowedMIME[mime] {
		return true
	}
	ext := strings.ToLower(filepath.Ext(name))
	if allowedExt[ext] {
		return true
	}
	return strings.HasPrefix(mime, "text/")
}

func mimeFromName(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".pdf":
		return "application/pdf"
	case ".md":
		return "text/markdown"
	case ".json":
		return "application/json"
	case ".csv":
		return "text/csv"
	default:
		return "text/plain"
	}
}

// SummaryLine describes attachments for persisted chat history.
func SummaryLine(parts []llm.Part) string {
	if len(parts) == 0 {
		return ""
	}
	names := make([]string, len(parts))
	for i, p := range parts {
		if p.Name != "" {
			names[i] = p.Name
		} else {
			names[i] = p.Type
		}
	}
	return "[Attached: " + strings.Join(names, ", ") + "]\n"
}
