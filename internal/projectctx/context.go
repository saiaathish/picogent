package projectctx

import (
	"os"
	"path/filepath"
	"strings"
)

const maxRulesBytes = 24 << 10

var ruleFiles = []string{
	"AGENTS.md",
	"CLAUDE.md",
	".picogent/rules.md",
}

// Load reads project instruction files from the workspace (Claude Code / Cursor style).
func Load(workspace string) string {
	if workspace == "" {
		return ""
	}
	var parts []string
	for _, name := range ruleFiles {
		path := filepath.Join(workspace, name)
		data, err := os.ReadFile(path)
		if err != nil || len(data) == 0 {
			continue
		}
		if len(data) > maxRulesBytes {
			data = data[:maxRulesBytes]
		}
		text := strings.TrimSpace(string(data))
		if text == "" {
			continue
		}
		parts = append(parts, "── "+name+" ──\n"+text)
	}
	return strings.Join(parts, "\n\n")
}
