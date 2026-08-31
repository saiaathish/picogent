package extensions

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/saiaathish/picogent/internal/mcpbridge"
	"github.com/saiaathish/picogent/internal/securefile"
)

const (
	claudeMarketplaceURL     = "https://raw.githubusercontent.com/anthropics/claude-plugins-official/main/.claude-plugin/marketplace.json"
	claudeLibraryName        = "claude-official"
	maxClaudeMarketplaceSize = 4 << 20
	claudeMarketplaceTimeout = 15 * time.Second
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
	return loadClaudeLibrary(true)
}

// LoadClaudeLibraryCached is the turn-safe form of marketplace discovery. It
// only uses an already-loaded in-memory result or a local Claude/Picogent cache;
// it never downloads marketplace metadata or writes a cache file.
func LoadClaudeLibraryCached() ([]SearchResult, LibraryStats) {
	return loadClaudeLibrary(false)
}

func loadClaudeLibrary(allowNetwork bool) ([]SearchResult, LibraryStats) {
	claudeCacheMu.RLock()
	if !claudeLoaded.IsZero() && time.Since(claudeLoaded) < 30*time.Minute && (len(claudeCache) > 0 || !allowNetwork) {
		out := append([]SearchResult(nil), claudeCache...)
		st := claudeStats
		claudeCacheMu.RUnlock()
		return out, st
	}
	claudeCacheMu.RUnlock()

	items, stats := fetchClaudeMarketplace(allowNetwork)
	claudeCacheMu.Lock()
	claudeCache = items
	claudeStats = stats
	claudeLoaded = time.Now()
	claudeCacheMu.Unlock()
	return items, stats
}

func fetchClaudeMarketplace(allowNetwork bool) ([]SearchResult, LibraryStats) {
	data, err := readClaudeMarketplaceFile()
	if err != nil {
		if !allowNetwork {
			return nil, LibraryStats{}
		}
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
	if root, _, err := openClaudePluginsRoot(); err == nil {
		defer root.Close()
		for _, path := range []string{
			filepath.Join("marketplaces", "claude-plugins-official", ".claude-plugin", "marketplace.json"),
			filepath.Join("marketplaces", "claude-plugins-official", "marketplace.json"),
		} {
			data, readErr := readBoundedRoot(root, path, 4<<20)
			if readErr == nil && len(data) > 0 {
				return data, nil
			}
		}
	}
	home, err := picogentHome()
	if err != nil {
		return nil, err
	}
	cached := filepath.Join(home, "claude-marketplace.json")
	return securefile.ReadFileLimited(cached, maxClaudeMarketplaceSize)
}

func downloadClaudeMarketplace() ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, claudeMarketplaceURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "picogent-extension-finder")
	client := &http.Client{
		Timeout: claudeMarketplaceTimeout,
		CheckRedirect: func(next *http.Request, _ []*http.Request) error {
			if next.URL.Scheme != "https" || next.URL.Hostname() != "raw.githubusercontent.com" {
				return fmt.Errorf("marketplace redirect leaves the trusted host")
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return readClaudeMarketplaceBody(resp)
}

func readClaudeMarketplaceBody(resp *http.Response) ([]byte, error) {
	if resp == nil || resp.Body == nil {
		return nil, errors.New("marketplace response has no body")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download marketplace: %s", resp.Status)
	}
	if resp.ContentLength > maxClaudeMarketplaceSize {
		return nil, fmt.Errorf("marketplace response exceeds %d bytes", maxClaudeMarketplaceSize)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxClaudeMarketplaceSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxClaudeMarketplaceSize {
		return nil, fmt.Errorf("marketplace response exceeds %d bytes", maxClaudeMarketplaceSize)
	}
	return data, nil
}

func cacheClaudeMarketplace(data []byte) error {
	home, err := picogentHome()
	if err != nil {
		return err
	}
	return securefile.WriteAtomic(filepath.Join(home, "claude-marketplace.json"), data, 0o600)
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

func hasLocalMCP(name string) bool {
	root, _, err := openClaudePluginRoot(name)
	if err != nil {
		return false
	}
	defer root.Close()
	servers, present, err := readClaudeMCPConfig(root)
	return err == nil && present && len(servers) > 0
}

type claudeInstalledPlugins struct {
	Plugins map[string][]struct {
		InstallPath string `json:"installPath"`
	} `json:"plugins"`
}

// openClaudePluginsRoot returns a descriptor-backed root for ~/.claude/plugins.
// Every subsequent Claude cache read is relative to this root; no caller uses
// a checked absolute path as a security boundary.
func openClaudePluginsRoot() (*os.Root, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, "", err
	}
	homeRoot, err := os.OpenRoot(home)
	if err != nil {
		return nil, "", err
	}
	pluginsRoot, err := homeRoot.OpenRoot(filepath.Join(".claude", "plugins"))
	if err != nil {
		_ = homeRoot.Close()
		return nil, "", err
	}
	_ = homeRoot.Close()
	return pluginsRoot, filepath.Join(home, ".claude", "plugins"), nil
}

// openClaudePluginRoot resolves a cached plugin while retaining a descriptor
// for the selected directory. Root.OpenRoot prevents a concurrent rename or
// symlink swap from turning later reads into reads outside ~/.claude/plugins.
func openClaudePluginRoot(name string) (*os.Root, string, error) {
	if err := validateClaudeName(name); err != nil {
		return nil, "", err
	}
	pluginsRoot, displayRoot, err := openClaudePluginsRoot()
	if err != nil {
		return nil, "", err
	}
	closePluginsRoot := func() {
		_ = pluginsRoot.Close()
	}

	candidates := []string{
		filepath.Join("marketplaces", "claude-plugins-official", "plugins", name),
		filepath.Join("marketplaces", "claude-plugins-official", "external_plugins", name),
	}
	for _, rel := range candidates {
		pluginRoot, found, candidateErr := openClaudePluginCandidate(pluginsRoot, rel)
		if candidateErr != nil {
			closePluginsRoot()
			return nil, "", candidateErr
		}
		if found {
			closePluginsRoot()
			return pluginRoot, filepath.Join(displayRoot, rel), nil
		}
	}

	data, err := readBoundedRoot(pluginsRoot, "installed_plugins.json", 4<<20)
	if err != nil {
		closePluginsRoot()
		return nil, "", err
	}
	var installed claudeInstalledPlugins
	if err := json.Unmarshal(data, &installed); err != nil {
		closePluginsRoot()
		return nil, "", fmt.Errorf("parse Claude installed plugins: %w", err)
	}
	keys := make([]string, 0, len(installed.Plugins))
	for key := range installed.Plugins {
		pluginName, _, ok := strings.Cut(key, "@")
		if ok && pluginName == name {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		for _, entry := range installed.Plugins[key] {
			rel, relErr := claudeInstalledPluginRelative(displayRoot, entry.InstallPath)
			if relErr != nil {
				continue
			}
			pluginRoot, found, candidateErr := openClaudePluginCandidate(pluginsRoot, rel)
			if candidateErr != nil {
				closePluginsRoot()
				return nil, "", candidateErr
			}
			if found {
				closePluginsRoot()
				return pluginRoot, filepath.Join(displayRoot, rel), nil
			}
		}
	}
	closePluginsRoot()
	return nil, "", os.ErrNotExist
}

func openClaudePluginCandidate(root *os.Root, rel string) (*os.Root, bool, error) {
	clean := filepath.Clean(rel)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(clean) || filepath.VolumeName(clean) != "" {
		return nil, false, fmt.Errorf("Claude plugin path %q escapes the plugin cache", rel)
	}
	current := ""
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		if current == "" {
			current = part
		} else {
			current = filepath.Join(current, part)
		}
		info, err := root.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, false, fmt.Errorf("Claude plugin path contains symbolic link %q", current)
		}
		if !info.IsDir() {
			return nil, false, fmt.Errorf("Claude plugin path component %q is not a directory", current)
		}
	}
	pluginRoot, err := root.OpenRoot(clean)
	if err != nil {
		return nil, false, err
	}
	return pluginRoot, true, nil
}

func claudeInstalledPluginRelative(root, path string) (string, error) {
	if strings.TrimSpace(path) == "" || !filepath.IsAbs(path) {
		return "", errors.New("Claude install path is not absolute")
	}
	rootAbs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	candidate, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(rootAbs, candidate)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" {
		return "", errors.New("Claude install path is outside the plugin cache")
	}
	return rel, nil
}

// ClaudePluginDir returns the display path to a Claude Code plugin if cached.
// Reads and mutations must use openClaudePluginRoot instead of reopening this
// returned string.
func ClaudePluginDir(name string) string {
	root, path, err := openClaudePluginRoot(name)
	if err != nil {
		return ""
	}
	_ = root.Close()
	return path
}

func validateClaudeName(name string) error {
	clean := strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	if clean == "" || clean == "." || clean == ".." || strings.Contains(clean, "/") || filepath.IsAbs(filepath.FromSlash(clean)) || filepath.VolumeName(filepath.FromSlash(clean)) != "" {
		return fmt.Errorf("invalid Claude plugin name %q", name)
	}
	return nil
}

func readClaudeMCPConfig(root *os.Root) (map[string]mcpbridge.ServerConfig, bool, error) {
	info, err := root.Lstat(".mcp.json")
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, false, errors.New("Claude plugin MCP config is not a regular file")
	}
	data, err := readBoundedRoot(root, ".mcp.json", 1<<20)
	if err != nil {
		return nil, false, fmt.Errorf("read Claude plugin MCP config: %w", err)
	}
	servers, err := parseClaudeMCPJSON(data)
	if err != nil {
		return nil, false, err
	}
	return servers, true, nil
}

const claudeMCPNamespace = "picogent-claude-"

func claudeMCPServerKey(plugin, server string) string {
	return claudeMCPNamespace + hex.EncodeToString([]byte(plugin)) + "-" + hex.EncodeToString([]byte(server))
}

func parseClaudeMCPServerKey(key string) (string, string, bool) {
	rest, ok := strings.CutPrefix(key, claudeMCPNamespace)
	if !ok {
		return "", "", false
	}
	pluginHex, serverHex, ok := strings.Cut(rest, "-")
	if !ok || pluginHex == "" {
		return "", "", false
	}
	plugin, err := hex.DecodeString(pluginHex)
	if err != nil {
		return "", "", false
	}
	server, err := hex.DecodeString(serverHex)
	if err != nil {
		return "", "", false
	}
	return string(plugin), string(server), true
}

func claudeMCPConfiguredKeys(name string) ([]string, error) {
	if err := validateClaudeName(name); err != nil {
		return nil, err
	}
	servers, err := mcpbridge.LoadServers("")
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0)
	for key := range servers {
		plugin, _, ok := parseClaudeMCPServerKey(key)
		if ok && plugin == name {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys, nil
}

// claudeMCPServerKeys includes both currently configured keys and keys that a
// cached plugin is about to write. The latter makes a failed multi-server
// activation able to remove a newly-created first server during rollback.
func claudeMCPServerKeys(name string) ([]string, error) {
	if err := validateClaudeName(name); err != nil {
		return nil, err
	}
	keys := make(map[string]bool)
	if root, _, err := openClaudePluginRoot(name); err == nil {
		defer root.Close()
		servers, _, err := readClaudeMCPConfig(root)
		if err != nil {
			return nil, err
		}
		for server := range servers {
			keys[claudeMCPServerKey(name, server)] = true
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	configured, err := claudeMCPConfiguredKeys(name)
	if err != nil {
		return nil, err
	}
	for _, key := range configured {
		keys[key] = true
	}
	out := make([]string, 0, len(keys))
	for key := range keys {
		out = append(out, key)
	}
	sort.Strings(out)
	return out, nil
}

// ClaudeMCPServerKeys reports the Picogent config keys owned by one Claude
// plugin. It is used by explicit GUI activation to connect every supported
// server before publishing the replacement runtime.
func ClaudeMCPServerKeys(name string) ([]string, error) {
	return claudeMCPServerKeys(name)
}

func removeClaudePluginMCP(name string) error {
	keys, err := claudeMCPServerKeys(name)
	if err != nil {
		return err
	}
	var errs []error
	for _, key := range keys {
		if err := mcpbridge.RemoveServer(key); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// ActivateClaudePlugin activates only payload that Picogent can actually
// consume from the local Claude cache: supported MCP servers and/or non-empty
// cached skills. A cache entry with neither is rejected so callers do not
// persist a successful no-op.
func ActivateClaudePlugin(name string) error {
	if err := validateClaudeName(name); err != nil {
		return err
	}
	before, err := CaptureState("", []string{"claude:" + name})
	if err != nil {
		return err
	}
	defer before.Close()
	root, _, err := openClaudePluginRoot(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("claude plugin %q not cached — open Claude Code once to sync the marketplace", name)
		}
		return fmt.Errorf("open Claude plugin %q: %w", name, err)
	}
	defer root.Close()
	servers, mcpPresent, err := readClaudeMCPConfig(root)
	if err != nil {
		return err
	}
	_, hasSkills, err := claudePluginSkillPrompt(root, name)
	if err != nil {
		return fmt.Errorf("inspect Claude plugin skills: %w", err)
	}
	if len(servers) == 0 && !hasSkills {
		if mcpPresent {
			return fmt.Errorf("Claude plugin %q has no supported MCP servers or non-empty cached skills", name)
		}
		return fmt.Errorf("Claude plugin %q has no supported cached MCP servers or skills", name)
	}
	serverNames := make([]string, 0, len(servers))
	for server := range servers {
		serverNames = append(serverNames, server)
	}
	sort.Strings(serverNames)
	for _, server := range serverNames {
		if err := mcpbridge.SaveServer(claudeMCPServerKey(name, server), servers[server]); err != nil {
			return errors.Join(err, before.Restore())
		}
	}
	return nil
}

// ClaudePluginSkills reads bounded skill summaries from a cached Claude
// plugin. The descriptor remains the filesystem boundary for every read.
func ClaudePluginSkills(name string) string {
	root, _, err := openClaudePluginRoot(name)
	if err != nil {
		return ""
	}
	defer root.Close()
	prompt, _, err := claudePluginSkillPrompt(root, name)
	if err != nil {
		return ""
	}
	return prompt
}

func claudePluginSkillPrompt(root *os.Root, name string) (string, bool, error) {
	info, err := root.Lstat("skills")
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", false, errors.New("Claude plugin skills path is not a directory")
	}
	dir, err := root.Open("skills")
	if err != nil {
		return "", false, err
	}
	entries, err := dir.Readdir(-1)
	closeErr := dir.Close()
	if err != nil {
		return "", false, err
	}
	if closeErr != nil {
		return "", false, closeErr
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	const (
		maxSkills      = 16
		maxSkillBytes  = 64 << 10
		maxPromptBytes = 8 << 10
	)
	parts := make([]string, 0, min(maxSkills, len(entries)))
	total := len("Active Claude plugin skills (" + name + "):\n")
	for _, entry := range entries {
		if len(parts) >= maxSkills || entry.Mode()&os.ModeSymlink != 0 || !entry.IsDir() {
			continue
		}
		rel := filepath.Join("skills", entry.Name(), "SKILL.md")
		info, err := root.Lstat(rel)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			continue
		}
		data, err := readBoundedRoot(root, rel, maxSkillBytes)
		if err != nil {
			continue
		}
		body := strings.TrimSpace(string(data))
		if body == "" {
			continue
		}
		if len(body) > 800 {
			body = body[:800] + "…"
		}
		block := "### " + entry.Name() + "\n" + body
		if total+len(block)+2 > maxPromptBytes {
			break
		}
		parts = append(parts, block)
		total += len(block) + 2
	}
	if len(parts) == 0 {
		return "", false, nil
	}
	return "Active Claude plugin skills (" + name + "):\n" + strings.Join(parts, "\n\n"), true, nil
}

// ClaudePluginsPrompt loads cached skills for Claude plugin IDs persisted in
// any extension lifetime list. Invalid or non-Claude IDs are ignored.
func ClaudePluginsPrompt(ids []string) string {
	const maxPlugins = 8
	const maxTotal = 16 << 10
	seen := make(map[string]bool)
	parts := make([]string, 0)
	total := 0
	for _, id := range ids {
		name, ok := strings.CutPrefix(strings.TrimSpace(id), "claude:")
		if !ok || seen[name] || validateClaudeName(name) != nil {
			continue
		}
		seen[name] = true
		if len(parts) >= maxPlugins {
			break
		}
		part := ClaudePluginSkills(name)
		if part == "" {
			continue
		}
		if total+len(part)+2 > maxTotal {
			break
		}
		parts = append(parts, part)
		total += len(part) + 2
	}
	return strings.Join(parts, "\n\n")
}

func parseClaudeMCPJSON(data []byte) (map[string]mcpbridge.ServerConfig, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if wrapped, ok := raw["mcpServers"]; ok {
		var servers map[string]mcpbridge.ServerConfig
		if err := json.Unmarshal(wrapped, &servers); err != nil {
			return nil, err
		}
		return servers, nil
	}
	out := map[string]mcpbridge.ServerConfig{}
	for name, chunk := range raw {
		var cfg mcpbridge.ServerConfig
		if err := json.Unmarshal(chunk, &cfg); err != nil {
			continue
		}
		if cfg.Command != "" || cfg.URL != "" {
			out[name] = cfg
		}
	}
	return out, nil
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
	return byClaudeName(name, items)
}

// ByClaudeNameCached resolves a Claude plugin without refreshing marketplace
// metadata. It is used while a Safe-mode install is waiting for approval.
func ByClaudeNameCached(name string) *Item {
	items, _ := LoadClaudeLibraryCached()
	return byClaudeName(name, items)
}

func byClaudeName(name string, items []SearchResult) *Item {
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
