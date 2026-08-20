package extensions

import (
	"os"
	"path/filepath"
	"strings"
)

// SkillsPrompt loads installed skill summaries for the system prompt.
func SkillsPrompt(skillNames []string) string {
	if len(skillNames) == 0 {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	root := filepath.Join(home, ".cursor", "skills-cursor")
	var parts []string
	for _, name := range skillNames {
		skillMD := filepath.Join(root, name, "SKILL.md")
		data, err := os.ReadFile(skillMD)
		if err != nil {
			continue
		}
		body := strings.TrimSpace(string(data))
		if body == "" {
			continue
		}
		if len(body) > 1200 {
			body = body[:1200] + "\n…"
		}
		parts = append(parts, "### Skill: "+name+"\n"+body)
	}
	if len(parts) == 0 {
		return ""
	}
	return "Installed agent skills (follow when relevant):\n" + strings.Join(parts, "\n\n")
}
