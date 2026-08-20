package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/mcpbridge"
	"github.com/saiaathish/picogent/internal/perm"
)

const (
	maxReadBytes = 256 << 10
	maxReadLines = 2000
	maxToolOut   = 32 << 10
)

type Context struct {
	Workspace    string
	BashTimeout  time.Duration
	Todos        []TodoItem
	ClassifyBash func(command, workspace string) perm.Request
	ClassifyPath func(tool, path, workspace, summary string) perm.Request
	MCPList      func() string
	MCPSuggest   func(query string) string
	MCPAdd       func(ctx context.Context, id string) (string, error)
	MCPRemove    func(ctx context.Context, id string) (string, error)
	Verify       func(ctx context.Context) (string, error)
}

type Tool interface {
	Spec() llm.ToolSpec
	Permission(args string, c Context) perm.Request
	Run(ctx context.Context, args string, c Context) (string, error)
}

type Registry struct {
	byName map[string]Tool
	order  []string
	Ctx    Context
	MCP    *mcpbridge.Manager
}

func NewRegistry(c Context) *Registry {
	if c.ClassifyBash == nil {
		c.ClassifyBash = perm.ClassifyBash
	}
	if c.ClassifyPath == nil {
		c.ClassifyPath = perm.ClassifyPath
	}
	if c.BashTimeout <= 0 {
		c.BashTimeout = 60 * time.Second
	}
	r := &Registry{byName: map[string]Tool{}, Ctx: c}
	for _, t := range []Tool{
		readFile{},
		listDir{},
		writeFile{},
		editFile{},
		globTool{},
		grepTool{},
		bashTool{},
		gitTool{},
		webFetch{},
		todoWrite{},
		mcpManage{},
		verifyTool{},
	} {
		r.register(t)
	}
	return r
}

func (r *Registry) AttachMCP(m *mcpbridge.Manager) {
	r.MCP = m
	if m == nil {
		return
	}
	// Drop prior MCP tools if re-attaching.
	filtered := r.order[:0]
	for _, name := range r.order {
		if !strings.HasPrefix(name, "mcp_") {
			filtered = append(filtered, name)
		} else {
			delete(r.byName, name)
		}
	}
	r.order = filtered
	for _, spec := range m.Specs() {
		r.register(mcpTool{mgr: m, name: spec.Name})
	}
}

func (r *Registry) register(t Tool) {
	name := t.Spec().Name
	r.byName[name] = t
	r.order = append(r.order, name)
}

func (r *Registry) Specs() []llm.ToolSpec {
	out := make([]llm.ToolSpec, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.byName[name].Spec())
	}
	return out
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.byName[name]
	return t, ok
}

func (r *Registry) HasMCP() bool {
	return r.MCP != nil && len(r.MCP.Tools()) > 0
}

func (r *Registry) HasBrowserMCP() bool {
	return r.MCP != nil && r.MCP.HasBrowser()
}

type mcpTool struct {
	mgr  *mcpbridge.Manager
	name string
}

func (t mcpTool) Spec() llm.ToolSpec {
	for _, spec := range t.mgr.Specs() {
		if spec.Name == t.name {
			return spec
		}
	}
	return llm.ToolSpec{Name: t.name}
}

func (t mcpTool) Permission(args string, _ Context) perm.Request {
	summary := t.name
	var payload map[string]any
	if json.Unmarshal([]byte(strings.TrimSpace(args)), &payload) == nil {
		if v, ok := payload["url"].(string); ok && v != "" {
			summary = t.name + " → " + truncate(v, 80)
		}
	}
	return perm.ClassifyMCP(t.name, summary)
}

func (t mcpTool) Run(ctx context.Context, args string, _ Context) (string, error) {
	bt, ok := t.mgr.Get(t.name)
	if !ok {
		return "", fmt.Errorf("mcp tool %s not found", t.name)
	}
	return t.mgr.Call(ctx, bt, args)
}

func parseJSON(args string, dest any) error {
	args = strings.TrimSpace(args)
	if args == "" {
		args = "{}"
	}
	if err := json.Unmarshal([]byte(args), dest); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}

func resolvePath(workspace, p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("path is required")
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p), nil
	}
	return filepath.Abs(filepath.Join(workspace, p))
}

func relDisplay(workspace, abs string) string {
	rel, err := filepath.Rel(workspace, abs)
	if err != nil {
		return abs
	}
	return rel
}

func clip(s string) string {
	if len(s) <= maxToolOut {
		return s
	}
	return s[:maxToolOut] + "\n… truncated …"
}

func skipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "dist", "build", ".venv", "vendor", "target", ".picogent", ".idea", ".next":
		return true
	}
	return false
}

func mustWorkspace(c Context) (string, error) {
	ws, err := filepath.Abs(c.Workspace)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(ws)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		return "", fmt.Errorf("workspace is not a directory: %s", ws)
	}
	return ws, nil
}

func schema(props map[string]any, required []string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           props,
		"required":             required,
		"additionalProperties": false,
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
