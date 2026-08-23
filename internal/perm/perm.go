package perm

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

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

// WorkspacePath is a requested path resolved against a workspace. Path is
// canonical for every existing component, so permissions describe the real
// target instead of a symlink-shaped alias.
type WorkspacePath struct {
	Root             string
	Path             string
	OutsideWorkspace bool
}

type Prompter func(ctx context.Context, req Request) (Decision, error)

type Gate struct {
	mu            sync.RWMutex
	Mode          config.Mode
	Workspace     string
	Prompt        Prompter
	allowTurn     bool
	alwaysAllowed map[string]bool
}

func New(mode config.Mode, workspace string, prompt Prompter) *Gate {
	return &Gate{Mode: mode, Workspace: workspace, Prompt: prompt, alwaysAllowed: map[string]bool{}}
}

func (g *Gate) ResetTurn() {
	g.mu.Lock()
	g.allowTurn = false
	g.mu.Unlock()
}

// SetMode updates the permission mode without racing an in-flight Check.
func (g *Gate) SetMode(mode config.Mode) {
	g.mu.Lock()
	g.Mode = mode
	g.mu.Unlock()
}

// SetWorkspace updates the workspace captured by future per-turn clones.
// Tool requests still carry their own resolved paths, but keeping this value
// current prevents a settings change from cloning a stale workspace boundary.
func (g *Gate) SetWorkspace(workspace string) {
	if g == nil {
		return
	}
	g.mu.Lock()
	g.Workspace = workspace
	g.mu.Unlock()
}

// SetPrompt replaces the per-turn permission callback. Check snapshots it
// before invoking the callback, so callers are never run under Gate's lock.
func (g *Gate) SetPrompt(prompt Prompter) {
	g.mu.Lock()
	g.Prompt = prompt
	g.mu.Unlock()
}

// CloneForTurn freezes the policy inputs for one agent turn while leaving the
// live gate available for settings changes and persisted approvals. The turn
// installs its own prompt callback after cloning.
func (g *Gate) CloneForTurn() *Gate {
	if g == nil {
		return nil
	}
	g.mu.RLock()
	mode := g.Mode
	workspace := g.Workspace
	allowed := make([]string, 0, len(g.alwaysAllowed))
	for tool, ok := range g.alwaysAllowed {
		if ok {
			allowed = append(allowed, tool)
		}
	}
	g.mu.RUnlock()
	clone := New(mode, workspace, nil)
	clone.SetAlwaysAllowed(allowed)
	return clone
}

func (g *Gate) SetAlwaysAllowed(tools []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.alwaysAllowed = map[string]bool{}
	for _, t := range tools {
		if t != "" {
			g.alwaysAllowed[t] = true
		}
	}
}

func (g *Gate) AddAlwaysAllowed(tool string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.alwaysAllowed == nil {
		g.alwaysAllowed = map[string]bool{}
	}
	g.alwaysAllowed[tool] = true
}

func (g *Gate) AlwaysAllowedTools() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var out []string
	for t := range g.alwaysAllowed {
		out = append(out, t)
	}
	return out
}

func (g *Gate) Check(ctx context.Context, req Request) (Decision, error) {
	// Take one coherent decision snapshot, then invoke Prompt after releasing
	// the lock. GUI approval can update persisted always-allow state while a
	// request is waiting, and callbacks must remain free to re-enter Gate.
	g.mu.RLock()
	alwaysAllowed := g.alwaysAllowed != nil && g.alwaysAllowed[req.Tool]
	mode := g.Mode
	allowTurn := g.allowTurn
	prompt := g.Prompt
	g.mu.RUnlock()

	// A tool-wide approval must not silently extend to destructive or
	// out-of-workspace targets. Those always need a decision for this call.
	if alwaysAllowed && !req.Destructive && !req.OutsideWorkspace {
		return Allow, nil
	}
	if autoAllow(mode, req) {
		return Allow, nil
	}
	if allowTurn && !req.Destructive && !req.OutsideWorkspace {
		return Allow, nil
	}
	if prompt == nil {
		return Deny, nil
	}
	d, err := prompt(ctx, req)
	if err != nil {
		return Deny, err
	}
	if d == AllowTurn {
		g.mu.Lock()
		g.allowTurn = true
		g.mu.Unlock()
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
	case "read_file", "glob", "grep", "list_dir", "repo_map", "todo_write", "verify":
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
		case "write_file", "edit_file", "bash", "verify":
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

// bashSafeCommands is intentionally small.  A shell command is only
// auto-allowed in Fast mode when its executable and syntax are simple enough
// to keep the one-workspace promise legible.  Everything else still works
// after an explicit approval, but cannot silently run with the model's
// authority.
var bashSafeCommands = map[string]bool{
	"awk": true, "bun": true, "cargo": true, "cat": true, "cmake": true,
	"cut": true, "diff": true, "dirname": true, "du": true, "echo": true,
	"file": true, "find": true, "git": true, "go": true, "grep": true,
	"head": true, "jq": true, "just": true, "ls": true, "make": true,
	"node": true, "npm": true, "npx": true, "pnpm": true, "printf": true,
	"pwd": true, "pytest": true, "rg": true, "ruby": true, "rustc": true,
	"sed": true, "sort": true, "tail": true, "tr": true, "uniq": true,
	"wc": true, "which": true, "where": true, "yarn": true,
}

var bashDestructiveRe = regexp.MustCompile(`(?i)(^|[\s;&|])(?:rm|sudo|mkfs|dd|shutdown|reboot|chmod|chown|kill|pkill|git\s+(?:push|commit|reset|clean|checkout|restore|rebase)|npm\s+(?:install|uninstall|publish)|pnpm\s+(?:install|remove|publish)|yarn\s+(?:add|remove|publish)|cargo\s+(?:install|publish))\b`)

func ClassifyBash(command, workspace string) Request {
	cmd := strings.TrimSpace(command)
	req := Request{
		Tool:        "bash",
		Summary:     "run `" + truncate(cmd, 120) + "`",
		Command:     cmd,
		Destructive: destructiveRe.MatchString(cmd) || bashDestructiveRe.MatchString(cmd) || strings.Contains(cmd, "rm -rf") || strings.Contains(cmd, "git push"),
	}
	req.OutsideWorkspace, req.Hint = bashNeedsApproval(cmd, workspace)
	return req
}

// bashNeedsApproval is a conservative shell boundary, not a shell parser.
// Shell syntax is intentionally treated as unsafe when it could redirect,
// chain, substitute, or otherwise make the eventual filesystem targets hard
// to prove from the command string alone.  The command can still be run after
// a user explicitly approves it; Fast/--yes simply cannot auto-authorize it.
func bashNeedsApproval(command, workspace string) (bool, string) {
	if command == "" {
		return true, "shell command is empty"
	}
	if strings.IndexByte(command, 0) >= 0 {
		return true, "shell command contains an invalid byte"
	}
	if strings.ContainsAny(command, "\r\n;|&<>`") || (runtime.GOOS != "windows" && strings.Contains(command, "\\")) || strings.Contains(command, "$ (") || strings.Contains(command, "$(") {
		return true, "shell operators and substitutions require approval"
	}
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return true, "shell command is empty"
	}
	first := strings.Trim(fields[0], "\"'")
	if strings.Contains(first, "=") && !strings.HasPrefix(first, "./") {
		return true, "environment assignments are not auto-allowed"
	}
	base := strings.ToLower(filepath.Base(first))
	if !bashSafeCommands[base] {
		return true, "this shell executable is not constrained to the workspace"
	}
	root := strings.TrimSpace(workspace)
	if root != "" {
		if abs, err := filepath.Abs(root); err == nil {
			root = filepath.Clean(abs)
		}
	}
	for i, raw := range fields {
		token := strings.Trim(raw, "\"'()[]{},")
		if token == "" || i == 0 {
			continue
		}
		if strings.HasPrefix(token, "~") || strings.ContainsAny(token, "$`") {
			return true, "home and environment paths require approval"
		}
		// Parent traversal is unsafe even when the final target happens to
		// resolve back into the workspace through a symlink.
		if token == ".." || strings.HasPrefix(token, "../") || strings.HasPrefix(token, `..\`) || strings.Contains(token, "/../") || strings.Contains(token, `\..\`) {
			return true, "parent paths require approval"
		}
		candidate := token
		if eq := strings.IndexByte(candidate, '='); eq >= 0 {
			candidate = candidate[eq+1:]
		}
		if !shellPathIsAbsolute(candidate) {
			continue
		}
		if root == "" {
			return true, "absolute paths require approval"
		}
		abs, err := filepath.Abs(candidate)
		if err != nil || !within(abs, root) {
			return true, "path leaves the workspace"
		}
	}
	return false, ""
}

// shellPathIsAbsolute recognizes both the host path form and slash-rooted
// paths accepted by common shells. On Windows, filepath.IsAbs("/etc/passwd")
// is false even though a shell may interpret it as a rooted path; do not let
// that evade the workspace boundary. A leading backslash is similarly rooted
// on the current Windows volume.
func shellPathIsAbsolute(path string) bool {
	if filepath.IsAbs(path) || strings.HasPrefix(path, "/") {
		return true
	}
	return runtime.GOOS == "windows" && strings.HasPrefix(path, `\`)
}

func ClassifyPath(tool, relOrAbs, workspace, summary string) Request {
	req := Request{Tool: tool, Summary: summary, Path: relOrAbs}
	resolved, err := ResolveWorkspacePath(workspace, relOrAbs)
	if err != nil {
		// Do not auto-allow a path whose real target could not be established
		// (for example, a dangling or looping symlink).
		req.OutsideWorkspace = true
		req.Hint = "path could not be safely resolved"
	} else {
		if resolved.OutsideWorkspace {
			req.Path = resolved.Path
			req.OutsideWorkspace = true
		}
	}
	if tool == "git" && summary == "commit" {
		req.Destructive = false // still asked in Fast via git commit special-case below
		req.Command = "commit"
	}
	return req
}

// ResolveWorkspacePath resolves a requested path and reports whether its real
// target leaves workspace. Existing path components are canonicalized one by
// one so a symlink inside the workspace cannot be mistaken for an in-workspace
// target during permission classification.
//
// Callers must resolve again immediately before I/O. This closes the normal
// symlink-alias bypass; fully hostile filesystem races require platform-specific
// descriptor-relative no-follow operations and are intentionally not hidden by
// this helper.
func ResolveWorkspacePath(workspace, requested string) (WorkspacePath, error) {
	if strings.TrimSpace(workspace) == "" {
		return WorkspacePath{}, errors.New("workspace path is empty")
	}
	if strings.TrimSpace(requested) == "" {
		return WorkspacePath{}, errors.New("path is empty")
	}

	input, err := filepath.Abs(workspace)
	if err != nil {
		return WorkspacePath{}, err
	}
	root, err := filepath.EvalSymlinks(input)
	if err != nil {
		return WorkspacePath{}, fmt.Errorf("resolve workspace: %w", err)
	}
	root = filepath.Clean(root)
	info, err := os.Stat(root)
	if err != nil {
		return WorkspacePath{}, err
	}
	if !info.IsDir() {
		return WorkspacePath{}, errors.New("workspace is not a directory")
	}

	candidate := requested
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return WorkspacePath{}, err
	}
	resolved, err := resolveExistingComponents(candidate)
	if err != nil {
		return WorkspacePath{}, err
	}
	resolved = filepath.Clean(resolved)
	return WorkspacePath{
		Root:             root,
		Path:             resolved,
		OutsideWorkspace: !within(resolved, root),
	}, nil
}

// resolveExistingComponents follows each existing path component. For a new
// file, its nearest existing parent is canonicalized and the missing suffix is
// appended without changing it.
func resolveExistingComponents(path string) (string, error) {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return "", errors.New("path is not absolute")
	}

	volume := filepath.VolumeName(path)
	rest := strings.TrimPrefix(path, volume)
	separator := string(filepath.Separator)
	root := volume
	if strings.HasPrefix(rest, separator) {
		root += separator
		rest = strings.TrimPrefix(rest, separator)
	}
	if root == "" {
		return "", errors.New("path root is empty")
	}

	current := root
	parts := strings.Split(rest, separator)
	for i, part := range parts {
		if part == "" || part == "." {
			continue
		}
		next := filepath.Join(current, part)
		if _, err := os.Lstat(next); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				for _, remaining := range parts[i:] {
					if remaining != "" && remaining != "." {
						current = filepath.Join(current, remaining)
					}
				}
				return current, nil
			}
			return "", err
		}
		resolved, err := filepath.EvalSymlinks(next)
		if err != nil {
			return "", fmt.Errorf("resolve path component %q: %w", next, err)
		}
		current = filepath.Clean(resolved)
	}
	return current, nil
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
