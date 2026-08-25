package gui

import (
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
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
		http.Error(w, "GET only", 405)
		return
	}
	rel := strings.TrimSpace(r.URL.Query().Get("path"))
	s.mu.Lock()
	ws := s.cfg.Workspace
	s.mu.Unlock()

	// A workspace controls its Git configuration. Disable external diff and
	// textconv helpers so this read-only endpoint cannot execute
	// repository-configured commands.
	args := []string{"-C", ws, "diff", "--no-ext-diff", "--no-textconv", "--"}
	if rel != "" {
		args = append(args, rel)
	}
	out, err := exec.Command("git", args...).CombinedOutput()
	if err != nil && len(out) == 0 {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"path": rel,
		"diff": string(out),
	})
}
