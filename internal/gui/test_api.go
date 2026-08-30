package gui

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/saiaathish/picogent/internal/gitobs"
	"github.com/saiaathish/picogent/internal/redact"
)

func formatTestSummary(passed, failed, skipped int) string {
	return strings.TrimSpace(
		itoaStr(passed) + " passed · " + itoaStr(failed) + " failed · " + itoaStr(skipped) + " skipped",
	)
}

func itoaStr(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func (s *server) diffAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeGUIError(w, "GET only", 405)
		return
	}
	rel := strings.TrimSpace(r.URL.Query().Get("path"))
	s.mu.Lock()
	ws := s.cfg.Workspace
	s.mu.Unlock()

	// A workspace controls its Git configuration. The shared Git boundary
	// disables repository-configured helpers and bounds/redacts the result so
	// this read-only endpoint cannot execute or expose their output.
	args := []string{"diff", "--"}
	if rel != "" {
		args = append(args, rel)
	}
	result, err := gitobs.Combined(r.Context(), ws, args...)
	out := redact.Text(result.Output)
	if result.Truncated {
		out += "\n… git output truncated …"
	}
	if err != nil && len(out) == 0 {
		writeGUIError(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"path": rel,
		"diff": out,
	})
}
