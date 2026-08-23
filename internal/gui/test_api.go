package gui

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/learn"
)

func (s *server) testAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	s.mu.Lock()
	ws := s.cfg.Workspace
	busy := s.busy
	s.mu.Unlock()
	if busy {
		http.Error(w, "agent busy", 409)
		return
	}

	var in struct {
		Package string `json:"package"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	pkg := strings.TrimSpace(in.Package)
	if pkg == "" {
		pkg = "./..."
	}

	go s.runTests(ws, pkg)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "started": true})
}

func (s *server) runTests(workspace, pkg string) {
	s.emit(event{Type: "think", Text: "Running tests…", Kind: "test", Status: "start"})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "test", "-json", pkg)
	cmd.Dir = workspace
	out, err := cmd.CombinedOutput()
	text := string(out)

	passed, failed, skipped := 0, 0, 0
	var failures []map[string]string

	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		var ev map[string]any
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		action, _ := ev["Action"].(string)
		switch action {
		case "pass":
			passed++
		case "fail":
			failed++
			test := ""
			if pkg, ok := ev["Package"].(string); ok {
				test = pkg
			}
			if t, ok := ev["Test"].(string); ok {
				test += " " + t
			}
			failures = append(failures, map[string]string{
				"test":   strings.TrimSpace(test),
				"output": clipStr(stringFrom(ev["Output"]), 500),
			})
		case "skip":
			skipped++
		}
	}

	if passed == 0 && failed == 0 {
		passed, failed, skipped = parseTestOutput(text)
	}

	status := "done"
	if err != nil || failed > 0 {
		status = "fail"
	}

	s.emit(event{
		Type:    "test",
		Text:    formatTestSummary(passed, failed, skipped),
		Summary: clipStr(text, 2000),
		Count:   passed,
		Added:   failed,
		Removed: skipped,
		Status:  status,
		Kind:    "test",
	})

	store, _ := learn.Load(workspace)
	store.RecordTest(passed, failed, skipped, text)
	_ = learn.Save(store)

	s.emit(event{Type: "overview", Text: "refresh"})
}

func formatTestSummary(passed, failed, skipped int) string {
	return strings.TrimSpace(
		itoaStr(passed) + " passed · " + itoaStr(failed) + " failed · " + itoaStr(skipped) + " skipped",
	)
}

func stringFrom(v any) string {
	s, _ := v.(string)
	return s
}

func clipStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
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
