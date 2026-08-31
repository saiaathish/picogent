package extensions

import (
	"errors"
	"io/fs"
	"os"
)

// SyncCursorSkills discovers skills already in ~/.cursor/skills-cursor and returns their folder names.
func SyncCursorSkills() ([]string, error) {
	root, _, err := openSkillsRoot(false)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.Type()&os.ModeSymlink != 0 || !e.IsDir() {
			continue
		}
		valid, err := validSkillAtRoot(root, e.Name())
		if err == nil && valid {
			out = append(out, e.Name())
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
