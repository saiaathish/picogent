package extensions

import (
	"github.com/saiaathish/picogent/internal/mcpbridge"
)

// Kind identifies an extension type.
type Kind string

const (
	KindMCP    Kind = "mcp"
	KindSkill  Kind = "skill"
	KindPlugin Kind = "plugin"
)

// Item is a discoverable MCP server, skill, or plugin.
type Item struct {
	ID           string                  `json:"id"`
	Name         string                  `json:"name"`
	Kind         Kind                    `json:"kind"`
	Description  string                  `json:"description"`
	Keywords     []string                `json:"keywords"`
	Source       string                  `json:"source"`
	Stars        int                     `json:"stars,omitempty"`
	AuthRequired bool                    `json:"auth_required,omitempty"`
	AuthHint     string                  `json:"auth_hint,omitempty"`
	MCP          *mcpbridge.ServerConfig `json:"mcp,omitempty"`
	SkillRepo    string                  `json:"skill_repo,omitempty"`
	SkillPath    string                  `json:"skill_path,omitempty"`
	PluginCmd    string                  `json:"plugin_cmd,omitempty"`
}

// Catalog returns the built-in curated extension catalog.
func Catalog() []Item {
	return []Item{
		{
			ID: "mcp-github", Name: "GitHub", Kind: KindMCP,
			Description:  "Browse repos, issues, and pull requests from GitHub.",
			Keywords:     []string{"github", "pull request", "pr", "issue", "repo", "commit"},
			Source:       "https://github.com/modelcontextprotocol/servers/tree/main/src/github",
			Stars:        4200,
			AuthRequired: true,
			AuthHint:     "Set GITHUB_PERSONAL_ACCESS_TOKEN in MCP env or authorize when prompted.",
			MCP: &mcpbridge.ServerConfig{
				Command: "npx",
				Args:    []string{"-y", "@modelcontextprotocol/server-github"},
				Env:     map[string]string{"GITHUB_PERSONAL_ACCESS_TOKEN": ""},
			},
		},
		{
			ID: "mcp-brave-search", Name: "Brave Search", Kind: KindMCP,
			Description:  "Search the web for docs, errors, and current information.",
			Keywords:     []string{"search", "web", "google", "lookup", "docs", "documentation", "api"},
			Source:       "https://github.com/modelcontextprotocol/servers/tree/main/src/brave-search",
			Stars:        3800,
			AuthRequired: true,
			AuthHint:     "Get a Brave Search API key and set BRAVE_API_KEY.",
			MCP: &mcpbridge.ServerConfig{
				Command: "npx",
				Args:    []string{"-y", "@modelcontextprotocol/server-brave-search"},
				Env:     map[string]string{"BRAVE_API_KEY": ""},
			},
		},
		{
			ID: "mcp-filesystem", Name: "Filesystem", Kind: KindMCP,
			Description: "Extended file operations beyond the built-in tools.",
			Keywords:    []string{"file", "folder", "directory", "read", "write", "copy"},
			Source:      "https://github.com/modelcontextprotocol/servers/tree/main/src/filesystem",
			Stars:       4100,
			MCP: &mcpbridge.ServerConfig{
				Command: "npx",
				Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", "."},
			},
		},
		{
			ID: "mcp-postgres", Name: "PostgreSQL", Kind: KindMCP,
			Description:  "Query and inspect PostgreSQL databases.",
			Keywords:     []string{"postgres", "postgresql", "sql", "database", "db", "query"},
			Source:       "https://github.com/modelcontextprotocol/servers/tree/main/src/postgres",
			Stars:        2900,
			AuthRequired: true,
			AuthHint:     "Set POSTGRES_CONNECTION_STRING to your database URL.",
			MCP: &mcpbridge.ServerConfig{
				Command: "npx",
				Args:    []string{"-y", "@modelcontextprotocol/server-postgres"},
				Env:     map[string]string{"POSTGRES_CONNECTION_STRING": ""},
			},
		},
		{
			ID: "mcp-slack", Name: "Slack", Kind: KindMCP,
			Description:  "Send messages and read channels in Slack.",
			Keywords:     []string{"slack", "message", "channel", "notify", "notification"},
			Source:       "https://github.com/modelcontextprotocol/servers/tree/main/src/slack",
			Stars:        2100,
			AuthRequired: true,
			AuthHint:     "Set SLACK_BOT_TOKEN and SLACK_TEAM_ID.",
			MCP: &mcpbridge.ServerConfig{
				Command: "npx",
				Args:    []string{"-y", "@modelcontextprotocol/server-slack"},
				Env: map[string]string{
					"SLACK_BOT_TOKEN": "",
					"SLACK_TEAM_ID":   "",
				},
			},
		},
		{
			ID: "mcp-linear", Name: "Linear", Kind: KindMCP,
			Description:  "Manage Linear issues and projects.",
			Keywords:     []string{"linear", "ticket", "issue", "task", "project management"},
			Source:       "https://github.com/modelcontextprotocol/servers/tree/main/src/linear",
			Stars:        1800,
			AuthRequired: true,
			AuthHint:     "Set LINEAR_API_KEY from Linear settings.",
			MCP: &mcpbridge.ServerConfig{
				Command: "npx",
				Args:    []string{"-y", "@modelcontextprotocol/server-linear"},
				Env:     map[string]string{"LINEAR_API_KEY": ""},
			},
		},
		{
			ID: "mcp-browseros", Name: "BrowserOS", Kind: KindMCP,
			Description: "Control a real browser for testing sites you are signed into.",
			Keywords:    []string{"browseros", "browser", "tab", "snapshot", "navigate", "click", "website"},
			Source:      "https://browseros.com",
			MCP: &mcpbridge.ServerConfig{
				URL:  "http://127.0.0.1:9010/mcp",
				Type: "http",
			},
		},
		{
			ID: "mcp-puppeteer", Name: "Browser (Puppeteer)", Kind: KindMCP,
			Description: "Automate a headless browser for scraping and testing.",
			Keywords:    []string{"browser", "scrape", "screenshot", "puppeteer", "playwright", "web page", "click"},
			Source:      "https://github.com/modelcontextprotocol/servers/tree/main/src/puppeteer",
			Stars:       3500,
			MCP: &mcpbridge.ServerConfig{
				Command: "npx",
				Args:    []string{"-y", "@modelcontextprotocol/server-puppeteer"},
			},
		},
		{
			ID: "mcp-fetch", Name: "Fetch", Kind: KindMCP,
			Description: "Fetch web pages and convert them to markdown.",
			Keywords:    []string{"fetch", "url", "http", "html", "markdown", "web"},
			Source:      "https://github.com/modelcontextprotocol/servers/tree/main/src/fetch",
			Stars:       2200,
			MCP: &mcpbridge.ServerConfig{
				Command: "npx",
				Args:    []string{"-y", "@modelcontextprotocol/server-fetch"},
			},
		},
		{
			ID: "mcp-memory", Name: "Memory", Kind: KindMCP,
			Description: "Persistent memory graph for facts across sessions.",
			Keywords:    []string{"memory", "remember", "context", "knowledge", "graph"},
			Source:      "https://github.com/modelcontextprotocol/servers/tree/main/src/memory",
			Stars:       2600,
			MCP: &mcpbridge.ServerConfig{
				Command: "npx",
				Args:    []string{"-y", "@modelcontextprotocol/server-memory"},
			},
		},
		{
			ID: "skill-create-rule", Name: "Create Cursor Rule", Kind: KindSkill,
			Description: "Guide the agent to create .cursor/rules for your project.",
			Keywords:    []string{"rule", "cursor rule", "convention", "guideline", "agents.md"},
			Source:      "https://github.com/cursor/skills",
			SkillRepo:   "https://github.com/cursor/skills-cursor",
			SkillPath:   "create-rule",
		},
		{
			ID: "skill-create-skill", Name: "Create Skill", Kind: KindSkill,
			Description: "Help author reusable agent skills (SKILL.md format).",
			Keywords:    []string{"skill", "create skill", "workflow", "automation"},
			Source:      "https://github.com/cursor/skills",
			SkillRepo:   "https://github.com/cursor/skills-cursor",
			SkillPath:   "create-skill",
		},
		{
			ID: "skill-security-review", Name: "Security Review", Kind: KindSkill,
			Description: "Structured security review of code changes.",
			Keywords:    []string{"security", "audit", "vulnerability", "cve", "xss", "injection"},
			Source:      "https://github.com/cursor/skills",
			SkillRepo:   "https://github.com/cursor/skills-cursor",
			SkillPath:   "review-security",
		},
		{
			ID: "skill-update-settings", Name: "Update Cursor Settings", Kind: KindSkill,
			Description: "Modify editor settings.json safely.",
			Keywords:    []string{"settings", "vscode", "cursor settings", "preferences"},
			Source:      "https://github.com/cursor/skills",
			SkillRepo:   "https://github.com/cursor/skills-cursor",
			SkillPath:   "update-cursor-settings",
		},
		{
			ID: "plugin-go-test", Name: "Go Test Runner", Kind: KindPlugin,
			Description: "Enhanced Go test workflow with coverage hints.",
			Keywords:    []string{"go test", "golang", "test", "coverage", "unit test"},
			Source:      "https://github.com/saiaathish/picogent",
			PluginCmd:   "builtin:go-test",
		},
		{
			ID: "plugin-git-helper", Name: "Git Helper", Kind: KindPlugin,
			Description: "Safer git workflows with branch and commit helpers.",
			Keywords:    []string{"git", "branch", "commit", "merge", "rebase", "pull request"},
			Source:      "https://github.com/saiaathish/picogent",
			PluginCmd:   "builtin:git-helper",
		},
	}
}

// ByID returns a catalog item or nil.
func ByID(id string) *Item {
	for _, it := range Catalog() {
		if it.ID == id {
			copy := it
			return &copy
		}
	}
	return nil
}
