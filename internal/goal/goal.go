package goal

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/projects"
)

func storePath(workspace string) (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	sub := filepath.Join(dir, "goals")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(sub, projects.IDForPath(workspace)+".txt"), nil
}

// Load returns the active goal for a workspace, or "" if none.
func Load(workspace string) (string, error) {
	path, err := storePath(workspace)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// Set persists a goal for the workspace.
func Set(workspace, text string) error {
	text = strings.TrimSpace(text)
	path, err := storePath(workspace)
	if err != nil {
		return err
	}
	if text == "" {
		return Clear(workspace)
	}
	return os.WriteFile(path, []byte(text), 0o600)
}

// Clear removes the workspace goal.
func Clear(workspace string) error {
	path, err := storePath(workspace)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// WorkPrompt kicks off agent work on a newly set goal.
func WorkPrompt(text string) string {
	return `Active goal (stays until done):

` + text + `

Work toward this now with tools. Keep going until it is fully met or you are blocked.
When complete, start your reply with "Goal complete:" and summarize what was done.`
}

// PromptSuffix is injected into the system prompt while a goal is active.
func PromptSuffix(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return `

Active goal (keep working until fully complete; then start with "Goal complete:"):
` + text
}

// LooksComplete reports whether the assistant marked the goal done.
func LooksComplete(text string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	return strings.HasPrefix(t, "goal complete:") || strings.HasPrefix(t, "goal complete —")
}
