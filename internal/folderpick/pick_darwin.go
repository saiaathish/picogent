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
	script := fmt.Sprintf(`
try
	tell application "Finder" to activate
	set picked to choose folder with prompt %q
	return POSIX path of picked
on error number -128
	return ""
end try
`, prompt)
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
	return strings.TrimSuffix(path, "/"), nil
}
