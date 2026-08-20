//go:build darwin

package folderpick

import (
	"fmt"
	"os/exec"
	"strings"
)

// ChooseFiles opens Finder and returns absolute paths for selected files.
func ChooseFiles(prompt string) ([]string, error) {
	if prompt == "" {
		prompt = "Select files to attach"
	}
	script := fmt.Sprintf(`
try
	tell application "Finder" to activate
	set picked to choose file with prompt %q with multiple selections allowed
	set out to {}
	repeat with f in picked
		set end of out to POSIX path of f
	end repeat
	return out as text
on error number -128
	return ""
end try
`, prompt)
	raw, err := exec.Command("osascript", "-e", script).CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(raw))
		if text == "" || strings.Contains(text, "User canceled") || strings.Contains(text, "-128") {
			return nil, ErrCancelled
		}
		return nil, fmt.Errorf("file picker: %s", text)
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return nil, ErrCancelled
	}
	var paths []string
	for _, line := range strings.Split(text, ", ") {
		line = strings.TrimSpace(line)
		if line != "" {
			paths = append(paths, line)
		}
	}
	if len(paths) == 0 {
		return nil, ErrCancelled
	}
	return paths, nil
}
