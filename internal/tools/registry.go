package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
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
	Workspace     string
	BashTimeout   time.Duration
	Todos         []TodoItem
	ClassifyBash  func(command, workspace string) perm.Request
	ClassifyPath  func(tool, path, workspace, summary string) perm.Request
	MCPList       func() string
	MCPSuggest    func(query string) string
	MCPAdd        func(ctx context.Context, id string) (string, error)
	MCPRemove     func(ctx context.Context, id string) (string, error)
	Verify        func(ctx context.Context) (string, error)
	VerifyTargets func(ctx context.Context, targets []string) (string, error)
}

type Tool interface {
	Spec() llm.ToolSpec
	Permission(args string, c Context) perm.Request
	Run(ctx context.Context, args string, c Context) (string, error)
}

type Registry struct {
	mu         sync.RWMutex
	mutationMu sync.Mutex
	byName     map[string]Tool
	order      []string
	Ctx        Context
	MCP        *mcpbridge.Manager
}

// WithExclusive serializes operations that mutate the live tool topology
// (notably mcp_manage) across cloned session agents sharing this registry.
// Ordinary read/edit tools continue to run in parallel.
func (r *Registry) WithExclusive(fn func()) {
	if r == nil || fn == nil {
		return
	}
	r.mutationMu.Lock()
	defer r.mutationMu.Unlock()
	fn()
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
		repoMapTool{},
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
	r.mu.Lock()
	defer r.mu.Unlock()

	filtered := r.order[:0]
	for _, name := range r.order {
		if _, isMCP := r.byName[name].(mcpTool); isMCP {
			delete(r.byName, name)
			continue
		}
		filtered = append(filtered, name)
	}
	r.order = filtered
	r.MCP = m
	if m == nil {
		return
	}
	for _, spec := range m.Specs() {
		r.registerLocked(mcpTool{mgr: m, name: spec.Name})
	}
}

func (r *Registry) register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registerLocked(t)
}

func (r *Registry) registerLocked(t Tool) {
	name := t.Spec().Name
	r.byName[name] = t
	r.order = append(r.order, name)
}

func (r *Registry) Specs() []llm.ToolSpec {
	r.mu.RLock()
	registered := make([]Tool, 0, len(r.order))
	for _, name := range r.order {
		if tool, ok := r.byName[name]; ok {
			registered = append(registered, tool)
		}
	}
	r.mu.RUnlock()

	out := make([]llm.ToolSpec, 0, len(registered))
	for _, tool := range registered {
		out = append(out, tool.Spec())
	}
	return out
}

func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.byName[name]
	return t, ok
}

// ContextSnapshot returns the immutable-at-the-call-boundary tool context.
// Runtime wiring may replace callbacks between turns; a running turn should
// continue using one coherent copy.
func (r *Registry) ContextSnapshot() Context {
	if r == nil {
		return Context{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Ctx
}

// UpdateContext changes runtime callbacks without racing a turn that is
// already using a ContextSnapshot.
func (r *Registry) UpdateContext(update func(*Context)) {
	if r == nil || update == nil {
		return
	}
	r.mu.Lock()
	update(&r.Ctx)
	r.mu.Unlock()
}

// MCPManagerSnapshot returns the currently attached manager. The manager owns
// its own synchronization; callers must treat the pointer as shared.
func (r *Registry) MCPManagerSnapshot() *mcpbridge.Manager {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.MCP
}

func (r *Registry) HasMCP() bool {
	r.mu.RLock()
	mcp := r.MCP
	r.mu.RUnlock()
	return mcp != nil && len(mcp.Tools()) > 0
}

func (r *Registry) HasBrowserMCP() bool {
	r.mu.RLock()
	mcp := r.MCP
	r.mu.RUnlock()
	return mcp != nil && mcp.HasBrowser()
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
	resolved, err := perm.ResolveWorkspacePath(workspace, p)
	if err != nil {
		return "", err
	}
	return resolved.Path, nil
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
	resolved, err := perm.ResolveWorkspacePath(c.Workspace, ".")
	if err != nil {
		return "", err
	}
	return resolved.Root, nil
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
