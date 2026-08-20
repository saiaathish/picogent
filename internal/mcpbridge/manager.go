package mcpbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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
type Manager struct {
	mu    sync.Mutex
	tools []Tool
	close []func()
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
	if len(servers) == 0 {
		return m, nil
	}
	for name, cfg := range servers {
		sctx, cancel := context.WithTimeout(ctx, connectTimeout)
		session, cleanup, err := dial(sctx, name, cfg)
		cancel()
		if err != nil {
			warns = append(warns, fmt.Sprintf("%q: %v", name, err))
			continue
		}
		m.close = append(m.close, cleanup)
		list, err := session.ListTools(ctx, &mcp.ListToolsParams{})
		if err != nil {
			cleanup()
			warns = append(warns, fmt.Sprintf("%q list tools: %v", name, err))
			continue
		}
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
			m.tools = append(m.tools, Tool{
				PublicName:  publicName(name, t.Name),
				Server:      name,
				Original:    t.Name,
				Description: desc,
				Parameters:  schema,
				session:     session,
			})
		}
	}
	return m, warns
}

func dial(ctx context.Context, name string, cfg ServerConfig) (*mcp.ClientSession, func(), error) {
	client := mcp.NewClient(&mcp.Implementation{Name: "picogent", Version: "0.1.0"}, nil)
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
		cmd.Env = os.Environ()
		for k, v := range cfg.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
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
	for _, fn := range m.close {
		if fn != nil {
			fn()
		}
	}
	m.close = nil
	m.tools = nil
}
