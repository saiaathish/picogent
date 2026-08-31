package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/extensions"
	"github.com/saiaathish/picogent/internal/mcpbridge"
	"github.com/saiaathish/picogent/internal/tools"
	"github.com/saiaathish/picogent/internal/trace"
	"github.com/saiaathish/picogent/internal/verify"
)

func wireRuntime(a *agent.Agent) {
	if a == nil || a.Tools == nil {
		return
	}
	cfg := a.ConfigSnapshot()
	log, err := trace.Open(cfg.Workspace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "picogent: trace: %v\n", err)
	}
	a.SetTrace(log)
	a.Tools.UpdateContext(func(c *tools.Context) {
		c.MCPList = func() string {
			workspace := a.ConfigSnapshot().Workspace
			return mcpListText(workspace, a.Tools.MCPManagerSnapshot())
		}
		c.MCPSuggest = func(query string) string {
			return mcpSuggestText(a.ConfigSnapshot().Workspace, query)
		}
		c.MCPAdd = func(ctx context.Context, id string) (string, error) {
			return mcpAdd(a, id)
		}
		c.MCPRemove = func(ctx context.Context, id string) (string, error) {
			return mcpRemove(a, id)
		}
		c.Verify = func(ctx context.Context) (string, error) {
			res := verify.Run(ctx, a.ConfigSnapshot().Workspace)
			return verify.Format(res), nil
		}
		c.VerifyTargets = func(ctx context.Context, targets []string) (string, error) {
			res := verify.RunPipeline(ctx, a.ConfigSnapshot().Workspace, verify.Options{Targets: targets})
			return verify.FormatPipeline(res), nil
		}
	})
}

func mcpListText(workspace string, live *mcpbridge.Manager) string {
	servers, err := mcpbridge.LoadServers(workspace)
	if err != nil {
		return err.Error()
	}
	liveCount := map[string]int{}
	if live != nil {
		for _, t := range live.Tools() {
			liveCount[t.Server]++
		}
	}
	if len(servers) == 0 && len(liveCount) == 0 {
		return "no MCP servers configured"
	}
	var b strings.Builder
	for name := range servers {
		if n, ok := liveCount[name]; ok {
			fmt.Fprintf(&b, "%s — connected (%d tools)\n", name, n)
			delete(liveCount, name)
			continue
		}
		fmt.Fprintf(&b, "%s — configured (not connected)\n", name)
	}
	for name, n := range liveCount {
		fmt.Fprintf(&b, "%s — connected (%d tools)\n", name, n)
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
	workspace := a.ConfigSnapshot().Workspace
	res, _, err := extensions.Install(*it, workspace)
	if err != nil {
		return "", err
	}
	msg := res.Message
	if res.AuthNeeded {
		msg += " — needs auth: " + res.AuthHint
	}
	if a.Tools != nil {
		cfg := *it.MCP
		if servers, err := mcpbridge.LoadServers(workspace); err == nil {
			if saved, ok := servers[res.MCPName]; ok {
				cfg = saved
			}
		}
		if err := connectOne(a, res.MCPName, cfg); err != nil {
			msg += " (connect warning: " + err.Error() + ")"
		}
	}
	_ = a.TraceSnapshot().Append("mcp_add", "mcp_manage", id, trace.Bool(true), 0)
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
	if a.Tools != nil {
		if mcp := a.Tools.MCPManagerSnapshot(); mcp != nil {
			mcp.DropServer(name)
			if err := a.Tools.AttachMCP(mcp); err != nil {
				return "", err
			}
		}
	}
	_ = a.TraceSnapshot().Append("mcp_remove", "mcp_manage", id, trace.Bool(true), 0)
	return "removed " + name, nil
}

func connectOne(a *agent.Agent, name string, cfg mcpbridge.ServerConfig) error {
	if a == nil || a.Tools == nil || name == "" {
		return nil
	}
	mcp := a.Tools.MCPManagerSnapshot()
	created := false
	if mcp == nil {
		mcp = &mcpbridge.Manager{}
		created = true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 22*time.Second)
	defer cancel()
	err := mcp.ConnectServer(ctx, name, cfg)
	if err != nil {
		if created {
			mcp.Close()
		}
		return err
	}
	if err := a.Tools.AttachMCP(mcp); err != nil {
		if created {
			mcp.Close()
		}
		return err
	}
	return err
}

// ReloadMCP reconnects MCP servers onto the live registry.
func ReloadMCP(a *agent.Agent) error {
	if a == nil || a.Tools == nil {
		return fmt.Errorf("no agent")
	}
	workspace := a.ConfigSnapshot().Workspace
	servers, err := mcpbridge.LoadServers(workspace)
	if err != nil {
		return err
	}
	if len(servers) == 0 {
		return a.Tools.AttachMCP(nil)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	mgr, warns := mcpbridge.ConnectBestEffort(ctx, servers)
	if err := a.Tools.AttachMCP(mgr); err != nil {
		mgr.Close()
		return err
	}
	if len(warns) > 0 {
		return fmt.Errorf("%s", strings.Join(warns, "; "))
	}
	return nil
}
