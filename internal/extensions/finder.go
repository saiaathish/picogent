package extensions

import (
	"strings"
)

// Recommend finds catalog items matching prompt text and not yet installed.
func Recommend(prompt string, installed map[string]bool, dismissed map[string]bool) []Item {
	if prompt == "" {
		return nil
	}
	lower := strings.ToLower(prompt)
	var out []Item
	for _, it := range Catalog() {
		if installed[it.ID] || dismissed[it.ID] {
			continue
		}
		score := matchScore(lower, it.Keywords)
		if score > 0 {
			out = append(out, it)
		}
	}
	sortByScore(out, lower)
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}

func matchScore(text string, keywords []string) int {
	score := 0
	for _, kw := range keywords {
		kw = strings.ToLower(kw)
		if strings.Contains(text, kw) {
			score += len(kw)
		}
	}
	return score
}

// MatchScore is the exported keyword matcher for other packages in extensions.
func MatchScore(text string, keywords []string) int {
	return matchScore(text, keywords)
}

func sortByScore(items []Item, text string) {
	for i := 1; i < len(items); i++ {
		j := i
		for j > 0 && matchScore(text, items[j].Keywords) > matchScore(text, items[j-1].Keywords) {
			items[j], items[j-1] = items[j-1], items[j]
			j--
		}
	}
}

// SearchResult is a catalog item plus install state for API responses.
type SearchResult struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Kind         Kind     `json:"kind"`
	Description  string   `json:"description"`
	Keywords     []string `json:"keywords"`
	Source       string   `json:"source"`
	Stars        int      `json:"stars,omitempty"`
	AuthRequired bool     `json:"auth_required,omitempty"`
	AuthHint     string   `json:"auth_hint,omitempty"`
	Reliability  string   `json:"reliability,omitempty"`
	Library      string   `json:"library,omitempty"`
	Essential    bool     `json:"essential,omitempty"`
	Active       bool     `json:"active,omitempty"`
	Installed    bool     `json:"installed"`
}

// Search finds items by free-text query across name, description, and keywords.
func Search(query string, installed map[string]bool) []SearchResult {
	q := strings.ToLower(strings.TrimSpace(query))
	items := Catalog()
	if q == "" {
		return toSearchResults(items, installed)
	}
	var matched []Item
	for _, it := range items {
		hay := strings.ToLower(it.Name + " " + it.Description + " " + strings.Join(it.Keywords, " "))
		if strings.Contains(hay, q) || matchScore(q, it.Keywords) > 0 {
			matched = append(matched, it)
		}
	}
	return toSearchResults(matched, installed)
}

func toSearchResults(items []Item, installed map[string]bool) []SearchResult {
	out := make([]SearchResult, 0, len(items))
	for _, it := range items {
		out = append(out, SearchResult{
			ID: it.ID, Name: it.Name, Kind: it.Kind, Description: it.Description,
			Keywords: it.Keywords, Source: it.Source, Stars: it.Stars,
			AuthRequired: it.AuthRequired, AuthHint: it.AuthHint,
			Installed: installed[it.ID],
		})
	}
	return out
}

// ToSearchResults converts catalog items to API search results.
func ToSearchResults(items []Item, installed map[string]bool) []SearchResult {
	return toSearchResults(items, installed)
}
