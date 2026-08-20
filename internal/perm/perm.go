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
	Allow     Decision = "allow"
	Deny      Decision = "deny"
	AllowTurn Decision = "allow_turn"
)

type Request struct {
	Tool             string
	Summary          string
	Path             string
	Command          string
	Destructive      bool
	OutsideWorkspace bool
}

type Prompter func(ctx context.Context, req Request) (Decision, error)

type Gate struct {
	Mode      config.Mode
	Workspace string
	Prompt    Prompter
	allowTurn bool
}

func New(mode config.Mode, workspace string, prompt Prompter) *Gate {
	return &Gate{Mode: mode, Workspace: workspace, Prompt: prompt}
}

func (g *Gate) ResetTurn() { g.allowTurn = false }

func (g *Gate) Check(ctx context.Context, req Request) (Decision, error) {
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
	return d, nil
}

func autoAllow(mode config.Mode, req Request) bool {
	if req.OutsideWorkspace || req.Destructive {
		return false
	}
	switch req.Tool {
	case "read_file", "glob", "grep", "list_dir", "todo_write":
		return true
	case "git":
		return req.Command == "status" || req.Command == "diff"
	}
	if strings.HasPrefix(req.Tool, "mcp_") {
		if mode == config.ModeFast {
			return true
		}
		return false
	}
	if mode == config.ModeFast {
		switch req.Tool {
		case "write_file", "edit_file", "bash", "web_fetch":
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
