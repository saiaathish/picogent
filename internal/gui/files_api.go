package gui

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/saiaathish/picogent/internal/attachments"
	"github.com/saiaathish/picogent/internal/folderpick"
)

func (s *server) filesPickAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeGUIError(w, "POST only", 405)
		return
	}
	paths, err := folderpick.ChooseFiles("Select files to attach")
	if errors.Is(err, folderpick.ErrCancelled) {
		w.WriteHeader(204)
		return
	}
	if err != nil {
		writeGUIError(w, err.Error(), 500)
		return
	}
	out, err := readAttachmentPaths(paths)
	if err != nil {
		writeGUIError(w, err.Error(), 400)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"files": out})
}

type attachmentFile struct {
	Name string `json:"name"`
	MIME string `json:"mime"`
	Data string `json:"data"`
}

func readAttachmentPaths(paths []string) ([]attachmentFile, error) {
	if len(paths) > attachments.MaxFiles {
		return nil, errors.New("too many files")
	}
	out := make([]attachmentFile, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		if len(data) > attachments.MaxBytes {
			return nil, errors.New(filepath.Base(p) + ": file too large")
		}
		name := filepath.Base(p)
		mime := mimeFromExt(name)
		parts, err := attachments.Decode([]attachments.Input{{
			Name: name,
			MIME: mime,
			Data: base64.StdEncoding.EncodeToString(data),
		}})
		if err != nil {
			return nil, err
		}
		_ = parts
		out = append(out, attachmentFile{
			Name: name,
			MIME: mime,
			Data: base64.StdEncoding.EncodeToString(data),
		})
	}
	return out, nil
}

func mimeFromExt(name string) string {
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
