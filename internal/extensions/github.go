package extensions

import (
	"fmt"
	"sort"
	"strings"
)

// LibraryStats summarizes discoverable extension counts.
type LibraryStats struct {
	Total   int `json:"total"`
	MCP     int `json:"mcp"`
	Skills  int `json:"skills"`
	Plugins int `json:"plugins"`
}

// Browse returns paginated extensions from Claude library + catalog, sorted by relevance/stars.
func Browse(kind Kind, query string, page int, installed map[string]bool) ([]SearchResult, LibraryStats, error) {
	if page < 1 {
		page = 1
	}
	perPage := 40

	var out []SearchResult
	stats := LibraryStats{}

	// Claude Code official library (primary source for plugins).
	claudeItems, claudeSt := LoadClaudeLibrary()
	stats.Plugins = claudeSt.Plugins
	stats.MCP += claudeSt.MCP

	// Curated catalog.
	for _, it := range Catalog() {
		if kind != "" && it.Kind != kind {
			continue
		}
		out = append(out, catalogToResult(it, installed))
		switch it.Kind {
		case KindMCP:
			stats.MCP++
		case KindSkill:
			stats.Skills++
		case KindPlugin:
			stats.Plugins++
		}
	}

	// Claude plugins filtered by kind/query.
	filtered := FilterClaude(claudeItems, kind, query)
	out = append(out, filtered...)

	// Local Cursor/Claude skills.
	localSkills := LocalSkillResults(query, installed)
	if kind == "" || kind == KindSkill {
		out = append(out, localSkills...)
		for range localSkills {
			stats.Skills++
		}
	}

	stats.Total = stats.Plugins + stats.MCP + stats.Skills
	if stats.Total == 0 {
		stats.Total = len(out)
	}

	// Optional query filter on catalog items when not already filtered.
	if query != "" {
		var matched []SearchResult
		for _, it := range out {
			if matchesQuery(it.Name+" "+it.Description+" "+strings.Join(it.Keywords, " "), query) {
				matched = append(matched, it)
			}
		}
		out = matched
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Stars != out[j].Stars {
			return out[i].Stars > out[j].Stars
		}
		if out[i].Reliability != out[j].Reliability {
			return reliabilityRank(out[i].Reliability) > reliabilityRank(out[j].Reliability)
		}
		return out[i].Name < out[j].Name
	})

	seen := map[string]bool{}
	unique := out[:0]
	for _, it := range out {
		if seen[it.ID] {
			continue
		}
		seen[it.ID] = true
		unique = append(unique, it)
	}
	out = unique

	start := (page - 1) * perPage
	if start >= len(out) {
		return nil, stats, nil
	}
	end := start + perPage
	if end > len(out) {
		end = len(out)
	}
	return out[start:end], stats, nil
}

func reliabilityRank(r string) int {
	switch r {
	case "high":
		return 4
	case "good":
		return 3
	case "fair":
		return 2
	case "new":
		return 1
	default:
		return 0
	}
}

func countLocalSkills() int {
	n, err := SyncCursorSkills()
	if err != nil {
		return 0
	}
	return len(n)
}

// AssistantFind recommends extensions for a workflow description.
func AssistantFind(workflow string, installed map[string]bool) (string, []SearchResult) {
	wf := strings.TrimSpace(workflow)
	if wf == "" {
		return "Tell me what you're building and I'll suggest MCP servers, plugins, and skills.", nil
	}

	var items []SearchResult
	seen := map[string]bool{}

	add := func(list []SearchResult) {
		for _, it := range list {
			if seen[it.ID] || installed[it.ID] {
				continue
			}
			seen[it.ID] = true
			items = append(items, it)
		}
	}

	for _, it := range Recommend(wf, installed, nil) {
		add([]SearchResult{catalogToResult(it, installed)})
	}

	claudeItems, _ := LoadClaudeLibrary()
	add(FilterClaude(claudeItems, "", wf))
	add(LocalSkillResults(wf, installed))

	sort.SliceStable(items, func(i, j int) bool {
		scoreI := matchScore(strings.ToLower(wf), items[i].Keywords) + items[i].Stars/100 + reliabilityRank(items[i].Reliability)*10
		scoreJ := matchScore(strings.ToLower(wf), items[j].Keywords) + items[j].Stars/100 + reliabilityRank(items[j].Reliability)*10
		return scoreI > scoreJ
	})
	if len(items) > 8 {
		items = items[:8]
	}

	msg := fmt.Sprintf("Found %d match%s. Picogent loads these only when you need them.", len(items), plural(len(items)))
	if len(items) == 0 {
		msg = "No close matches yet. Try different keywords or browse below."
	}
	return msg, items
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func catalogToResult(it Item, installed map[string]bool) SearchResult {
	return SearchResult{
		ID: it.ID, Name: it.Name, Kind: it.Kind, Description: it.Description,
		Keywords: it.Keywords, Source: it.Source, Stars: it.Stars,
		AuthRequired: it.AuthRequired, AuthHint: it.AuthHint,
		Installed: installed[it.ID], Library: "catalog",
	}
}

func matchesQuery(hay, query string) bool {
	hay = strings.ToLower(hay)
	for _, word := range strings.Fields(strings.ToLower(query)) {
		if len(word) > 2 && strings.Contains(hay, word) {
			return true
		}
	}
	return query == ""
}
