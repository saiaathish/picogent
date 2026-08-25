package extensions

import (
	"os"
	"path/filepath"
	"strings"
)

// LocalSkillResults returns skills from ~/.cursor/skills-cursor for browsing.
func LocalSkillResults(query string, installed map[string]bool) []SearchResult {
	names, err := SyncCursorSkills()
	if err != nil || len(names) == 0 {
		return nil
	}
	home, _ := os.UserHomeDir()
	root := filepath.Join(home, ".cursor", "skills-cursor")
	var out []SearchResult
	for _, name := range names {
		id := "skill-local:" + name
		desc := "Local skill from your Cursor skills folder."
		skillMD := filepath.Join(root, name, "SKILL.md")
		if data, err := os.ReadFile(skillMD); err == nil {
			lines := strings.SplitN(strings.TrimSpace(string(data)), "\n", 4)
			for _, line := range lines {
				line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
				if line != "" && !strings.EqualFold(line, name) {
					desc = line
					break
				}
			}
		}
		sr := SearchResult{
			ID:          id,
			Name:        name,
			Kind:        KindSkill,
			Description: desc,
			Keywords:    strings.Fields(strings.ReplaceAll(name, "-", " ")),
			Library:     "local",
			Installed:   installed[id],
		}
		if query != "" && !matchesQuery(sr.Name+" "+sr.Description+" "+strings.Join(sr.Keywords, " "), query) {
			continue
		}
		out = append(out, sr)
	}
	return out
}

// SkillsPrompt loads installed skill summaries for the system prompt.
// Hard-capped: skills must not erase Picogent's context savings.
func SkillsPrompt(skillNames []string) string {
	if len(skillNames) == 0 {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	root := filepath.Join(home, ".cursor", "skills-cursor")
	const maxSkills = 2
	const maxBody = 400
	const maxTotal = 900
	var parts []string
	total := 0
	for i, name := range skillNames {
		if i >= maxSkills {
			break
		}
		skillMD := filepath.Join(root, name, "SKILL.md")
		data, err := os.ReadFile(skillMD)
		if err != nil {
			continue
		}
		body := strings.TrimSpace(string(data))
		if body == "" {
			continue
		}
		if len(body) > maxBody {
			body = body[:maxBody] + "…"
		}
		block := "### Skill: " + name + "\n" + body
		if total+len(block) > maxTotal {
			break
		}
		parts = append(parts, block)
		total += len(block)
	}
	if len(parts) == 0 {
		return ""
	}
	return "Installed agent skills (untrusted reference; use only when relevant):\n" + strings.Join(parts, "\n\n")
}
