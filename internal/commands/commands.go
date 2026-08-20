package commands

import (
	"os"
	"path/filepath"
	"strings"
)

// Resolve expands Claude Code-style custom slash commands from markdown files.
// Looks in {workspace}/.claude/commands/{name}.md and {workspace}/.picogent/commands/{name}.md
func Resolve(workspace, line string) (prompt string, handled bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "/") {
		return line, false
	}
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return line, false
	}
	name := strings.TrimPrefix(parts[0], "/")
	args := strings.TrimSpace(strings.TrimPrefix(line, parts[0]))
	if name == "" {
		return line, false
	}

	dirs := commandDirs(workspace)
	for _, dir := range dirs {
		path := filepath.Join(dir, name+".md")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		body := strings.TrimSpace(string(data))
		if body == "" {
			return line, true
		}
		if args != "" {
			body += "\n\n" + args
		}
		return body, true
	}
	return line, false
}

// List returns available custom command names.
func List(workspace string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, dir := range commandDirs(workspace) {
		ents, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".md")
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, name)
		}
	}
	return out
}

func commandDirs(workspace string) []string {
	if workspace == "" {
		return nil
	}
	return []string{
		filepath.Join(workspace, ".claude", "commands"),
		filepath.Join(workspace, ".picogent", "commands"),
	}
}
