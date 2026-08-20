package extensions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/saiaathish/picogent/internal/mcpbridge"
)

// Pool manages on-demand extension activation — only loads what's needed per task.
type Pool struct {
	Workspace string
	Essential []string
	Transient []string
}

// NewPool creates a pool from config lists.
func NewPool(workspace string, essential, transient []string) *Pool {
	return &Pool{
		Workspace: workspace,
		Essential: append([]string(nil), essential...),
		Transient: append([]string(nil), transient...),
	}
}

// EnsureForPrompt activates extensions matching the prompt (transient, not permanent).
func (p *Pool) EnsureForPrompt(prompt string) ([]string, error) {
	installed, _ := InstalledSet(p.Workspace, nil)
	dismissed := map[string]bool{}
	recs := Recommend(prompt, installed, dismissed)

	// Also search Claude library by keyword.
	claudeItems, _ := LoadClaudeLibrary()
	lower := strings.ToLower(prompt)
	for _, it := range claudeItems {
		if matchScore(lower, it.Keywords) >= 8 && !installed[it.ID] {
			recs = append(recs, Item{
				ID: it.ID, Name: it.Name, Kind: it.Kind,
				Description: it.Description, Keywords: it.Keywords,
			})
		}
	}

	var activated []string
	seen := map[string]bool{}
	for _, id := range p.Essential {
		seen[id] = true
	}
	for _, id := range p.Transient {
		seen[id] = true
	}

	for _, it := range recs {
		if seen[it.ID] || p.isEssential(it.ID) {
			continue
		}
		if err := p.activate(it); err != nil {
			continue
		}
		p.Transient = appendUnique(p.Transient, it.ID)
		activated = append(activated, it.ID)
		seen[it.ID] = true
	}
	return activated, nil
}

// CleanupTransient deactivates extensions that aren't essential.
func (p *Pool) CleanupTransient() error {
	var keep []string
	for _, id := range p.Transient {
		if p.isEssential(id) {
			keep = append(keep, id)
			continue
		}
		_ = p.deactivate(id)
	}
	p.Transient = keep
	return nil
}

func (p *Pool) isEssential(id string) bool {
	for _, e := range p.Essential {
		if e == id {
			return true
		}
	}
	return false
}

func (p *Pool) activate(it Item) error {
	if strings.HasPrefix(it.ID, "claude:") {
		return ActivateClaudePlugin(strings.TrimPrefix(it.ID, "claude:"))
	}
	if it.Kind == KindMCP && it.MCP != nil {
		name := mcpServerName(it)
		return mcpbridge.SaveServer(name, *it.MCP)
	}
	if it.Kind == KindSkill {
		_, _, err := Install(it, p.Workspace)
		return err
	}
	return nil
}

func (p *Pool) deactivate(id string) error {
	if strings.HasPrefix(id, "claude:") {
		name := strings.TrimPrefix(id, "claude:")
		return mcpbridge.RemoveServersWithPrefix("claude-" + name)
	}
	it := ByID(id)
	if it == nil {
		return nil
	}
	if it.Kind == KindMCP {
		return mcpbridge.RemoveServer(mcpServerName(*it))
	}
	if it.Kind == KindSkill && it.SkillPath != "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		return os.RemoveAll(filepath.Join(home, ".cursor", "skills-cursor", it.SkillPath))
	}
	return nil
}

// ActivateClaudePlugin loads MCP + skills from a local Claude Code plugin cache.
func ActivateClaudePlugin(name string) error {
	dir := ClaudePluginDir(name)
	if dir == "" {
		return fmt.Errorf("claude plugin %q not cached — open Claude Code once to sync the marketplace", name)
	}
	mcpPath := filepath.Join(dir, ".mcp.json")
	data, err := os.ReadFile(mcpPath)
	if err != nil {
		// Plugin without MCP — still valid; skills load via ClaudePluginSkills.
		return nil
	}
	servers, err := parseClaudeMCPJSON(data)
	if err != nil {
		return err
	}
	prefix := "claude-" + name
	for srvName, cfg := range servers {
		key := prefix
		if srvName != name && srvName != "" {
			key = prefix + "-" + srvName
		}
		if err := mcpbridge.SaveServer(key, cfg); err != nil {
			return err
		}
	}
	return nil
}

// ClaudePluginSkills reads skill summaries from a Claude plugin directory.
func ClaudePluginSkills(name string) string {
	dir := ClaudePluginDir(name)
	if dir == "" {
		return ""
	}
	skillsDir := filepath.Join(dir, "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return ""
	}
	var parts []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillMD := filepath.Join(skillsDir, e.Name(), "SKILL.md")
		data, err := os.ReadFile(skillMD)
		if err != nil {
			continue
		}
		body := strings.TrimSpace(string(data))
		if len(body) > 800 {
			body = body[:800] + "…"
		}
		parts = append(parts, "### "+e.Name()+"\n"+body)
	}
	if len(parts) == 0 {
		return ""
	}
	return "Active Claude plugin skills (" + name + "):\n" + strings.Join(parts, "\n\n")
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

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

// MarkEssential adds an extension to the always-keep list.
func MarkEssential(list []string, id string) []string {
	return appendUnique(list, id)
}

// ActiveStatus annotates browse results with pool state.
func ActiveStatus(items []SearchResult, essential, transient []string) []SearchResult {
	ess := map[string]bool{}
	tran := map[string]bool{}
	for _, id := range essential {
		ess[id] = true
	}
	for _, id := range transient {
		tran[id] = true
	}
	out := make([]SearchResult, len(items))
	for i, it := range items {
		out[i] = it
		out[i].Essential = ess[it.ID]
		out[i].Active = ess[it.ID] || tran[it.ID] || it.Installed
	}
	return out
}
