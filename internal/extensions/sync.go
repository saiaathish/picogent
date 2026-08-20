package extensions

import (
	"os"
	"path/filepath"
)

// SyncCursorSkills discovers skills already in ~/.cursor/skills-cursor and returns their folder names.
func SyncCursorSkills() ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	root := filepath.Join(home, ".cursor", "skills-cursor")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			skillMD := filepath.Join(root, e.Name(), "SKILL.md")
			if _, err := os.Stat(skillMD); err == nil {
				out = append(out, e.Name())
			}
		}
	}
	return out, nil
}

// LoadDeveloperExtensions merges Cursor-installed skills into config and returns updated skill list.
func LoadDeveloperExtensions(cfgSkills []string) []string {
	seen := map[string]bool{}
	out := append([]string{}, cfgSkills...)
	for _, s := range cfgSkills {
		seen[s] = true
	}
	cursor, err := SyncCursorSkills()
	if err != nil {
		return out
	}
	for _, s := range cursor {
		if !seen[s] {
			out = append(out, s)
			seen[s] = true
		}
	}
	return out
}
