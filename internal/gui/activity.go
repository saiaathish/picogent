package gui

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
)

func diffStats(workspace, relPath string) (added, removed int) {
	if workspace == "" || relPath == "" {
		return 0, 0
	}
	cmd := exec.Command("git", "-C", workspace, "diff", "--numstat", "--", relPath)
	out, err := cmd.Output()
	if err == nil && len(strings.TrimSpace(string(out))) > 0 {
		fields := strings.Fields(string(out))
		if len(fields) >= 2 {
			added = atoi(fields[0])
			removed = atoi(fields[1])
			return added, removed
		}
	}
	// Untracked or new file: count lines in working tree file.
	abs := filepath.Join(workspace, relPath)
	data, err := exec.Command("wc", "-l", abs).Output()
	if err != nil {
		return 0, 0
	}
	fields := strings.Fields(string(data))
	if len(fields) > 0 {
		return atoi(fields[0]), 0
	}
	return 0, 0
}

func lineDelta(oldStr, newStr string) (added, removed int) {
	oldLines := 1
	newLines := 1
	if oldStr != "" {
		oldLines = strings.Count(oldStr, "\n") + 1
	}
	if newStr != "" {
		newLines = strings.Count(newStr, "\n") + 1
	}
	if newLines >= oldLines {
		added = newLines - oldLines
	} else {
		removed = oldLines - newLines
	}
	if oldStr != newStr && added == 0 && removed == 0 {
		added = 1
	}
	return added, removed
}

func atoi(s string) int {
	if s == "-" {
		return 0
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func parseToolPath(args string) string {
	var in struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal([]byte(args), &in)
	return in.Path
}

func parseEditArgs(args string) (path, oldStr, newStr string) {
	var in struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	_ = json.Unmarshal([]byte(args), &in)
	return in.Path, in.OldString, in.NewString
}

func parseWriteContent(args string) (path, content string) {
	var in struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	_ = json.Unmarshal([]byte(args), &in)
	return in.Path, in.Content
}

func isTestCommand(args string) bool {
	low := strings.ToLower(args)
	return strings.Contains(low, "go test") ||
		strings.Contains(low, "npm test") ||
		strings.Contains(low, "pytest") ||
		strings.Contains(low, "make test")
}

func parseTestOutput(text string) (passed, failed, skipped int) {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ok ") {
			passed++
		}
		if strings.HasPrefix(line, "FAIL") {
			failed++
		}
		if strings.Contains(line, "--- FAIL:") {
			failed++
		}
	}
	if passed == 0 && failed == 0 {
		if strings.Contains(text, "PASS") {
			passed = strings.Count(text, "PASS")
		}
	}
	return passed, failed, skipped
}
