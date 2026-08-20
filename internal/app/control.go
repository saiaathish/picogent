package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/extensions"
	"github.com/saiaathish/picogent/internal/mcpbridge"
	"github.com/saiaathish/picogent/internal/trace"
	"github.com/saiaathish/picogent/internal/verify"
)

func wireRuntime(a *agent.Agent) {
	if a == nil || a.Tools == nil {
		return
	}
	log, _ := trace.Open(a.CFG.Workspace)
	a.Trace = log
	ws := a.CFG.Workspace
	a.Tools.Ctx.MCPList = func() string {
		return mcpListText(ws)
	}
	a.Tools.Ctx.MCPSuggest = func(query string) string {
		return mcpSuggestText(ws, query)
	}
	a.Tools.Ctx.MCPAdd = func(ctx context.Context, id string) (string, error) {
		return mcpAdd(a, id)
	}
	a.Tools.Ctx.MCPRemove = func(ctx context.Context, id string) (string, error) {
		return mcpRemove(a, id)
	}
	a.Tools.Ctx.Verify = func(ctx context.Context) (string, error) {
		res := verify.Run(ctx, ws)
		ok := res.OK
		_ = a.Trace.Append("verify", "verify", verify.Format(res), &ok, 0)
		return verify.Format(res), nil
	}
}

func mcpListText(workspace string) string {
	servers, err := mcpbridge.LoadServers(workspace)
	if err != nil {
		return err.Error()
	}
	if len(servers) == 0 {
		return "no MCP servers configured"
	}
	var b strings.Builder
	for name := range servers {
		b.WriteString(name)
		b.WriteByte('\n')
	}
	installed, _ := extensions.InstalledSet(workspace, nil)
	for _, it := range extensions.Catalog() {
		if it.Kind == extensions.KindMCP && installed[it.ID] {
			fmt.Fprintf(&b, "catalog %s (%s)\n", it.ID, it.Name)
		}
	}
	return strings.TrimSpace(b.String())
}

func mcpSuggestText(workspace, query string) string {
	installed, _ := extensions.InstalledSet(workspace, nil)
	recs := extensions.Recommend(query, installed, nil)
	if len(recs) == 0 {
		return "no catalog match — try mcp-github, mcp-fetch, mcp-browseros, mcp-postgres"
	}
	var b strings.Builder
	for _, it := range recs {
		fmt.Fprintf(&b, "%s — %s\n", it.ID, it.Name)
	}
	return strings.TrimSpace(b.String())
}

func mcpAdd(a *agent.Agent, id string) (string, error) {
	it := extensions.ByID(id)
	if it == nil {
		return "", fmt.Errorf("unknown catalog id %s", id)
	}
	res, _, err := extensions.Install(*it, a.CFG.Workspace)
	if err != nil {
		return "", err
	}
	msg := res.Message
	if res.AuthNeeded {
		msg += " — needs auth: " + res.AuthHint
	}
	if err := ReloadMCP(a); err != nil {
		msg += " (reload warning: " + err.Error() + ")"
	}
	_ = a.Trace.Append("mcp_add", "mcp_manage", id, trace.Bool(true), 0)
	return msg, nil
}

func mcpRemove(a *agent.Agent, id string) (string, error) {
	it := extensions.ByID(id)
	name := id
	if it != nil {
		name = extensions.MCPServerName(*it)
	}
	if err := mcpbridge.RemoveServer(name); err != nil {
		return "", err
	}
	if err := ReloadMCP(a); err != nil {
		_ = a.Trace.Append("mcp_remove", "mcp_manage", id, trace.Bool(false), 0)
		return "removed " + name + " (reload warning: " + err.Error() + ")", nil
	}
	_ = a.Trace.Append("mcp_remove", "mcp_manage", id, trace.Bool(true), 0)
	return "removed " + name, nil
}

// ReloadMCP reconnects MCP servers onto the live registry.
func ReloadMCP(a *agent.Agent) error {
	if a == nil || a.Tools == nil {
		return fmt.Errorf("no agent")
	}
	if a.Tools.MCP != nil {
		a.Tools.MCP.Close()
	}
	servers, err := mcpbridge.LoadServers(a.CFG.Workspace)
	if err != nil {
		return err
	}
	if len(servers) == 0 {
		a.Tools.AttachMCP(nil)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	mgr, warns := mcpbridge.ConnectBestEffort(ctx, servers)
	a.Tools.AttachMCP(mgr)
	if len(warns) > 0 {
		return fmt.Errorf("%s", strings.Join(warns, "; "))
	}
	return nil
}
