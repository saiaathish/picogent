package extensions

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	claudeMarketplaceURL = "https://raw.githubusercontent.com/anthropics/claude-plugins-official/main/.claude-plugin/marketplace.json"
	claudeLibraryName    = "claude-official"
)

var (
	claudeCache   []SearchResult
	claudeStats   LibraryStats
	claudeLoaded  time.Time
	claudeCacheMu sync.RWMutex
)

type claudeMarketplace struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Plugins     []struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Category    string `json:"category"`
		Homepage    string `json:"homepage"`
		Author      struct {
			Name string `json:"name"`
		} `json:"author"`
		Source any `json:"source"`
	} `json:"plugins"`
}

// LoadClaudeLibrary returns Claude Code official marketplace plugins as browse results.
func LoadClaudeLibrary() ([]SearchResult, LibraryStats) {
	claudeCacheMu.RLock()
	if len(claudeCache) > 0 && time.Since(claudeLoaded) < 30*time.Minute {
		out := append([]SearchResult(nil), claudeCache...)
		st := claudeStats
		claudeCacheMu.RUnlock()
		return out, st
	}
	claudeCacheMu.RUnlock()

	items, stats := fetchClaudeMarketplace()
	claudeCacheMu.Lock()
	claudeCache = items
	claudeStats = stats
	claudeLoaded = time.Now()
	claudeCacheMu.Unlock()
	return items, stats
}

func fetchClaudeMarketplace() ([]SearchResult, LibraryStats) {
	data, err := readClaudeMarketplaceFile()
	if err != nil {
		data, err = downloadClaudeMarketplace()
		if err != nil {
			return nil, LibraryStats{}
		}
		_ = cacheClaudeMarketplace(data)
	}

	var mp claudeMarketplace
	if err := json.Unmarshal(data, &mp); err != nil {
		return nil, LibraryStats{}
	}

	items := make([]SearchResult, 0, len(mp.Plugins))
	mcpCount := 0
	for _, p := range mp.Plugins {
		kind := KindPlugin
		if hasLocalMCP(p.Name) {
			kind = KindMCP
			mcpCount++
		}
		keywords := claudeKeywords(p.Name, p.Category, p.Description)
		items = append(items, SearchResult{
			ID:          "claude:" + p.Name,
			Name:        p.Name,
			Kind:        kind,
			Description: strings.TrimSpace(p.Description),
			Keywords:    keywords,
			Source:      claudePluginURL(p),
			Reliability: "high",
			Library:     claudeLibraryName,
		})
	}

	stats := LibraryStats{
		Plugins: len(items),
		MCP:     mcpCount,
		Total:   len(items),
	}
	return items, stats
}

func readClaudeMarketplaceFile() ([]byte, error) {
	paths := claudeMarketplacePaths()
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err == nil && len(data) > 0 {
			return data, nil
		}
	}
	home, err := picogentHome()
	if err != nil {
		return nil, err
	}
	cached := filepath.Join(home, "claude-marketplace.json")
	return os.ReadFile(cached)
}

func claudeMarketplacePaths() []string {
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, ".claude", "plugins", "marketplaces", "claude-plugins-official", ".claude-plugin", "marketplace.json"),
		filepath.Join(home, ".claude", "plugins", "marketplaces", "claude-plugins-official", "marketplace.json"),
	}
}

func downloadClaudeMarketplace() ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, claudeMarketplaceURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "picogent-extension-finder")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download marketplace: %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func cacheClaudeMarketplace(data []byte) error {
	home, err := picogentHome()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(home, "claude-marketplace.json"), data, 0o600)
}

func picogentHome() (string, error) {
	if v := os.Getenv("PICOGENT_HOME"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".picogent"), nil
}

func claudePluginURL(p struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Homepage    string `json:"homepage"`
	Author      struct {
		Name string `json:"name"`
	} `json:"author"`
	Source any `json:"source"`
}) string {
	if p.Homepage != "" {
		return p.Homepage
	}
	return "https://github.com/anthropics/claude-plugins-official"
}

func claudeKeywords(name, category, desc string) []string {
	var out []string
	if category != "" {
		out = append(out, category)
	}
	out = append(out, strings.Fields(strings.ReplaceAll(name, "-", " "))...)
	lower := strings.ToLower(name + " " + desc)
	for _, kw := range []string{"mcp", "database", "github", "slack", "supabase", "firebase", "security", "deploy"} {
		if strings.Contains(lower, kw) {
			out = append(out, kw)
		}
	}
	return out
}

// ClaudePluginDir returns the local path to a Claude Code plugin if cached.
func ClaudePluginDir(name string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	base := filepath.Join(home, ".claude", "plugins", "marketplaces", "claude-plugins-official")
	for _, sub := range []string{"plugins", "external_plugins"} {
		p := filepath.Join(base, sub, name)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
	}
	// Installed plugin cache from Claude Code.
	if data, err := os.ReadFile(filepath.Join(home, ".claude", "plugins", "installed_plugins.json")); err == nil {
		var inst struct {
			Plugins map[string][]struct {
				InstallPath string `json:"installPath"`
			} `json:"plugins"`
		}
		if json.Unmarshal(data, &inst) == nil {
			for key, entries := range inst.Plugins {
				if strings.HasPrefix(key, name+"@") && len(entries) > 0 && entries[0].InstallPath != "" {
					return entries[0].InstallPath
				}
			}
		}
	}
	return ""
}

func hasLocalMCP(name string) bool {
	dir := ClaudePluginDir(name)
	if dir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, ".mcp.json"))
	return err == nil
}

// FilterClaude filters Claude library items by kind and query.
func FilterClaude(items []SearchResult, kind Kind, query string) []SearchResult {
	var out []SearchResult
	for _, it := range items {
		switch kind {
		case KindPlugin:
			// Every Claude marketplace entry is a plugin.
		case KindMCP:
			if it.Kind != KindMCP && !hasLocalMCP(it.Name) {
				continue
			}
		case KindSkill:
			continue
		case "":
			// all
		default:
			if it.Kind != kind {
				continue
			}
		}
		if query != "" && !matchesQuery(it.Name+" "+it.Description+" "+strings.Join(it.Keywords, " "), query) {
			continue
		}
		out = append(out, it)
	}
	return out
}

// ByClaudeName returns a catalog-style item for a Claude plugin slug.
func ByClaudeName(name string) *Item {
	items, _ := LoadClaudeLibrary()
	id := "claude:" + name
	for _, sr := range items {
		if sr.ID == id {
			it := Item{
				ID:          id,
				Name:        sr.Name,
				Kind:        sr.Kind,
				Description: sr.Description,
				Keywords:    sr.Keywords,
				Source:      sr.Source,
			}
			return &it
		}
	}
	return &Item{
		ID:          id,
		Name:        name,
		Kind:        KindPlugin,
		Description: "Claude Code plugin",
		Keywords:    claudeKeywords(name, "", ""),
	}
}
