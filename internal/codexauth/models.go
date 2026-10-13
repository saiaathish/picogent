package codexauth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// CLIModel is a model displayed by the installed Codex CLI for this login.
type CLIModel struct {
	Slug           string `json:"slug"`
	DisplayName    string `json:"display_name"`
	Priority       int    `json:"priority"`
	Visibility     string `json:"visibility"`
	SupportedInAPI bool   `json:"supported_in_api"`
}

var modelSlug = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)
var modelEdition = regexp.MustCompile(`^gpt-(\d+)(?:\.(\d+))?-(luna|terra|sol)$`)

// AvailableModels reads the Codex CLI's local picker cache. It makes no
// network request and never starts a model run.
func AvailableModels() []CLIModel {
	path := filepath.Join(HomeDir(), "models_cache.json")
	st, err := os.Stat(path)
	if err != nil || st.Size() > 2<<20 {
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cache struct {
		Models []CLIModel `json:"models"`
	}
	if json.Unmarshal(b, &cache) != nil {
		return nil
	}
	seen := map[string]bool{}
	out := make([]CLIModel, 0, len(cache.Models))
	for _, m := range cache.Models {
		if m.Visibility != "list" || !m.SupportedInAPI || !modelSlug.MatchString(m.Slug) || seen[m.Slug] {
			continue
		}
		seen[m.Slug] = true
		if m.DisplayName == "" {
			m.DisplayName = m.Slug
		}
		out = append(out, m)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Priority < out[j].Priority
	})
	return out
}

// LatestFamily returns the highest numeric edition of a pickable Luna,
// Terra, or Sol model. An unavailable family returns an empty string.
func LatestFamily(family string) string {
	var best string
	bestMajor, bestMinor := -1, -1
	for _, m := range AvailableModels() {
		parts := modelEdition.FindStringSubmatch(m.Slug)
		if len(parts) != 4 || parts[3] != strings.ToLower(family) {
			continue
		}
		major, _ := strconv.Atoi(parts[1])
		minor, _ := strconv.Atoi(parts[2])
		if major > bestMajor || major == bestMajor && minor > bestMinor {
			best, bestMajor, bestMinor = m.Slug, major, minor
		}
	}
	return best
}
