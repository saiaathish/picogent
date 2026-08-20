//go:build darwin

package folderpick

import (
	"fmt"
	"os/exec"
	"strings"
)

func Choose(prompt string) (string, error) {
	if prompt == "" {
		prompt = "Select a project folder"
	}
	script := fmt.Sprintf(`POSIX path of (choose folder with prompt "%s")`, escapeAppleScript(prompt))
	out, err := exec.Command("osascript", "-e", script).CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if text == "" || strings.Contains(text, "User canceled") || strings.Contains(text, "-128") {
			return "", ErrCancelled
		}
		return "", fmt.Errorf("folder picker: %s", text)
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", ErrCancelled
	}
	return path, nil
}

func escapeAppleScript(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}
