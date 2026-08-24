package mcpbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/saiaathish/picogent/internal/llm"
)

const connectTimeout = 20 * time.Second

// Tool is one MCP tool exposed to Picogent.
type Tool struct {
	PublicName  string
	Server      string
	Original    string
	Description string
	Parameters  map[string]any
	session     *mcp.ClientSession
}

// Manager holds live MCP sessions and discovered tools.
type conn struct {
	name  string
	fp    string
	close func()
}

type Manager struct {
	mu    sync.Mutex
	tools []Tool
	conns []conn
}

func Connect(ctx context.Context, servers map[string]ServerConfig) (*Manager, error) {
	m, warns := ConnectBestEffort(ctx, servers)
	if len(m.Tools()) == 0 && len(warns) > 0 && len(servers) > 0 {
		return m, fmt.Errorf("%s", strings.Join(warns, "; "))
	}
	return m, nil
}

// ConnectBestEffort connects to each server; failures are collected as warnings.
func ConnectBestEffort(ctx context.Context, servers map[string]ServerConfig) (*Manager, []string) {
	m := &Manager{}
	var warns []string
	seen := map[string]string{}
	for name, cfg := range servers {
		fp := fingerprint(cfg)
		if other, ok := seen[fp]; ok {
			warns = append(warns, fmt.Sprintf("%q skipped (same endpoint as %q)", name, other))
			continue
		}
		seen[fp] = name
		if err := m.ConnectServer(ctx, name, cfg); err != nil {
			warns = append(warns, fmt.Sprintf("%q: %v", name, err))
			delete(seen, fp)
		}
	}
	return m, warns
}

// ConnectServer dials one MCP server and merges its tools. Existing servers stay up.
func (m *Manager) ConnectServer(ctx context.Context, name string, cfg ServerConfig) error {
	if m == nil {
		return fmt.Errorf("no manager")
	}
	m.DropServer(name)
	m.dropFingerprint(fingerprint(cfg))
	sctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	session, cleanup, err := dial(sctx, name, cfg)
	if err != nil {
		return err
	}
	list, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		cleanup()
		return fmt.Errorf("list tools: %w", err)
	}
	var added []Tool
	for _, t := range list.Tools {
		if t == nil || t.Name == "" {
			continue
		}
		schema := normalizeSchema(t.InputSchema)
		desc := strings.TrimSpace(t.Description)
		if desc == "" {
			desc = t.Name + " via " + name
		}
		desc = "[" + name + "] " + desc
		added = append(added, Tool{
			PublicName:  publicName(name, t.Name),
			Server:      name,
			Original:    t.Name,
			Description: desc,
			Parameters:  schema,
			session:     session,
		})
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.conns = append(m.conns, conn{name: name, fp: fingerprint(cfg), close: cleanup})
	m.tools = append(m.tools, added...)
	return nil
}

// DropServer disconnects one server without touching the others.
func (m *Manager) DropServer(name string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropLocked(name)
}

func fingerprint(cfg ServerConfig) string {
	u := strings.TrimRight(strings.ToLower(strings.TrimSpace(cfg.URL)), "/")
	if u != "" {
		return "url:" + u
	}
	return "cmd:" + cfg.Command + "\x00" + strings.Join(cfg.Args, "\x00")
}

func (m *Manager) dropFingerprint(fp string) {
	if m == nil || fp == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var names []string
	for _, c := range m.conns {
		if c.fp == fp {
			names = append(names, c.name)
		}
	}
	for _, name := range names {
		m.dropLocked(name)
	}
}

func (m *Manager) dropLocked(name string) {
	filtered := m.tools[:0]
	for _, t := range m.tools {
		if t.Server != name {
			filtered = append(filtered, t)
		}
	}
	m.tools = filtered
	kept := m.conns[:0]
	for _, c := range m.conns {
		if c.name == name {
			if c.close != nil {
				c.close()
			}
			continue
		}
		kept = append(kept, c)
	}
	m.conns = kept
}

func dial(ctx context.Context, name string, cfg ServerConfig) (*mcp.ClientSession, func(), error) {
	client := mcp.NewClient(&mcp.Implementation{Name: "picogent", Version: "1.0.0"}, nil)
	var transport mcp.Transport
	var cmd *exec.Cmd

	switch {
	case cfg.URL != "" || strings.EqualFold(cfg.Type, "http"):
		url := cfg.URL
		if url == "" {
			return nil, nil, fmt.Errorf("http server needs url")
		}
		transport = &mcp.StreamableClientTransport{Endpoint: url}
	case cfg.Command != "":
		cmd = exec.CommandContext(ctx, cfg.Command, cfg.Args...)
		cmd.Env = commandEnv(cfg.Env)
		transport = &mcp.CommandTransport{Command: cmd}
	default:
		return nil, nil, fmt.Errorf("server %q: need url or command", name)
	}

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		_ = session.Close()
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
	return session, cleanup, nil
}

// commandEnv keeps MCP subprocesses from inheriting unrelated credentials or
// loader hooks. A server may request additional values explicitly in config.
func commandEnv(explicit map[string]string) []string {
	allowed := []string{"PATH", "SystemRoot", "WINDIR", "TEMP", "TMP", "TMPDIR"}
	values := make(map[string]string, len(allowed)+len(explicit))
	for _, key := range allowed {
		if value, ok := os.LookupEnv(key); ok {
			values[key] = value
		}
	}
	for key, value := range explicit {
		if strings.TrimSpace(key) == "" {
			continue
		}
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key+"="+values[key])
	}
	return out
}

func normalizeSchema(raw any) map[string]any {
	if raw == nil {
		return map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}
	switch v := raw.(type) {
	case map[string]any:
		return v
	default:
		b, err := json.Marshal(raw)
		if err != nil {
			return map[string]any{"type": "object", "properties": map[string]any{}}
		}
		var out map[string]any
		if json.Unmarshal(b, &out) != nil {
			return map[string]any{"type": "object", "properties": map[string]any{}}
		}
		return out
	}
}

func (m *Manager) Tools() []Tool {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Tool, len(m.tools))
	copy(out, m.tools)
	return out
}

func (m *Manager) Specs() []llm.ToolSpec {
	tools := m.Tools()
	out := make([]llm.ToolSpec, 0, len(tools))
	for _, t := range tools {
		out = append(out, llm.ToolSpec{
			Name:        t.PublicName,
			Description: t.Description,
			Parameters:  t.Parameters,
		})
	}
	return out
}

func (m *Manager) HasBrowser() bool {
	for _, t := range m.Tools() {
		s := strings.ToLower(t.Server + " " + t.Original + " " + t.Description)
		if strings.Contains(s, "browser") || strings.Contains(s, "navigate") || strings.Contains(s, "snapshot") {
			return true
		}
	}
	return false
}

func (m *Manager) Get(publicName string) (Tool, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.tools {
		if t.PublicName == publicName {
			return t, true
		}
	}
	return Tool{}, false
}

func (m *Manager) Call(ctx context.Context, t Tool, args string) (string, error) {
	args = strings.TrimSpace(args)
	if args == "" {
		args = "{}"
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(args), &payload); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	res, err := t.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      t.Original,
		Arguments: payload,
	})
	if err != nil {
		return "", err
	}
	return formatResult(res), nil
}

func formatResult(res *mcp.CallToolResult) string {
	if res == nil {
		return ""
	}
	var parts []string
	for _, c := range res.Content {
		if c == nil {
			continue
		}
		if tc, ok := c.(*mcp.TextContent); ok && tc.Text != "" {
			parts = append(parts, tc.Text)
		}
	}
	if len(parts) == 0 && res.StructuredContent != nil {
		b, _ := json.MarshalIndent(res.StructuredContent, "", "  ")
		if len(b) > 0 {
			parts = append(parts, string(b))
		}
	}
	out := strings.Join(parts, "\n")
	if res.IsError {
		if out == "" {
			out = "tool returned an error"
		}
		return "error: " + out
	}
	if out == "" {
		return "ok"
	}
	if len(out) > 32<<10 {
		out = out[:32<<10] + "\n… truncated …"
	}
	return out
}

// Report returns human-readable MCP status lines.
func (m *Manager) Report() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.tools) == 0 {
		return nil
	}
	byServer := map[string][]string{}
	for _, t := range m.tools {
		byServer[t.Server] = append(byServer[t.Server], t.PublicName)
	}
	var lines []string
	for srv, names := range byServer {
		lines = append(lines, fmt.Sprintf("%s (%d tools)", srv, len(names)))
		for _, n := range names {
			lines = append(lines, "  "+n)
		}
	}
	return lines
}

func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.conns {
		if c.close != nil {
			c.close()
		}
	}
	m.conns = nil
	m.tools = nil
}
