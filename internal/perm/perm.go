package perm

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/saiaathish/picogent/internal/config"
)

type Decision string

const (
	Allow       Decision = "allow"
	Deny        Decision = "deny"
	AllowTurn   Decision = "allow_turn"
	AllowAlways Decision = "allow_always"
)

type Request struct {
	Tool             string
	Summary          string
	Hint             string
	Path             string
	Command          string
	Destructive      bool
	OutsideWorkspace bool
}

type Prompter func(ctx context.Context, req Request) (Decision, error)

type Gate struct {
	Mode          config.Mode
	Workspace     string
	Prompt        Prompter
	allowTurn     bool
	alwaysAllowed map[string]bool
}

func New(mode config.Mode, workspace string, prompt Prompter) *Gate {
	return &Gate{Mode: mode, Workspace: workspace, Prompt: prompt, alwaysAllowed: map[string]bool{}}
}

func (g *Gate) ResetTurn() { g.allowTurn = false }

func (g *Gate) SetAlwaysAllowed(tools []string) {
	g.alwaysAllowed = map[string]bool{}
	for _, t := range tools {
		if t != "" {
			g.alwaysAllowed[t] = true
		}
	}
}

func (g *Gate) AddAlwaysAllowed(tool string) {
	if g.alwaysAllowed == nil {
		g.alwaysAllowed = map[string]bool{}
	}
	g.alwaysAllowed[tool] = true
}

func (g *Gate) AlwaysAllowedTools() []string {
	var out []string
	for t := range g.alwaysAllowed {
		out = append(out, t)
	}
	return out
}

func (g *Gate) Check(ctx context.Context, req Request) (Decision, error) {
	if g.alwaysAllowed != nil && g.alwaysAllowed[req.Tool] {
		return Allow, nil
	}
	if autoAllow(g.Mode, req) {
		return Allow, nil
	}
	if g.allowTurn && !req.Destructive && !req.OutsideWorkspace {
		return Allow, nil
	}
	if g.Prompt == nil {
		return Deny, nil
	}
	d, err := g.Prompt(ctx, req)
	if err != nil {
		return Deny, err
	}
	if d == AllowTurn {
		g.allowTurn = true
		return Allow, nil
	}
	if d == AllowAlways {
		g.AddAlwaysAllowed(req.Tool)
		return Allow, nil
	}
	return d, nil
}

func autoAllow(mode config.Mode, req Request) bool {
	if req.OutsideWorkspace || req.Destructive {
		return false
	}
	switch req.Tool {
	case "read_file", "glob", "grep", "list_dir", "todo_write", "verify":
		return true
	case "git":
		return req.Command == "status" || req.Command == "diff"
	}
	if req.Tool == "mcp_manage" {
		return req.Command == "list" || req.Command == "suggest"
	}
	if strings.HasPrefix(req.Tool, "mcp_") {
		if mode == config.ModeFast {
			// Fast: read/list/navigate MCP tools auto-allow; write/act-style still ask.
			return !LooksWriteMCP(req.Tool)
		}
		return false
	}
	if mode == config.ModeFast {
		switch req.Tool {
		case "write_file", "edit_file", "bash", "web_fetch", "verify":
			return true
		}
	}
	return false
}

// LooksWriteMCP reports whether an MCP tool name looks mutating (write/act/send…).
func LooksWriteMCP(tool string) bool {
	lower := strings.ToLower(tool)
	for _, w := range []string{
		"write", "edit", "create", "delete", "remove", "drop", "send", "post", "push", "commit",
		"act", "click", "type", "fill", "upload", "download", "drag",
	} {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}

func ClassifyMCP(tool, summary string) Request {
	return Request{
		Tool:    tool,
		Summary: summary,
	}
}

var destructiveRe = regexp.MustCompile(`(?i)(^|[;&|]\s*)(rm|sudo|mkfs|dd|shutdown|reboot)\b`)

func ClassifyBash(command, workspace string) Request {
	cmd := strings.TrimSpace(command)
	req := Request{
		Tool:        "bash",
		Summary:     "run `" + truncate(cmd, 120) + "`",
		Command:     cmd,
		Destructive: destructiveRe.MatchString(cmd) || strings.Contains(cmd, "rm -rf") || strings.Contains(cmd, "git push"),
	}
	return req
}

func ClassifyPath(tool, relOrAbs, workspace, summary string) Request {
	req := Request{Tool: tool, Summary: summary, Path: relOrAbs}
	abs := relOrAbs
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(workspace, relOrAbs)
	}
	cleanWS, err := filepath.Abs(workspace)
	if err == nil {
		cleanFile, err := filepath.Abs(abs)
		if err == nil && !within(cleanFile, cleanWS) {
			req.OutsideWorkspace = true
			req.Path = cleanFile
		}
	}
	if tool == "git" && summary == "commit" {
		req.Destructive = false // still asked in Fast via git commit special-case below
		req.Command = "commit"
	}
	return req
}

func within(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
