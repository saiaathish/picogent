package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
)

type mcpManage struct{}

func (mcpManage) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Name:        "mcp_manage",
		Description: "List, add, or remove Picogent MCP servers from the catalog. For listing connected servers you MUST use action=list. Never use browser MCP tools to inspect MCP config.",
		Parameters: schema(map[string]any{
			"action": map[string]any{"type": "string", "description": "list | suggest | add | remove"},
			"id":     map[string]any{"type": "string", "description": "Catalog id for add/remove, e.g. mcp-github"},
			"query":  map[string]any{"type": "string", "description": "Free text for suggest"},
		}, []string{"action"}),
	}
}

func (mcpManage) Permission(args string, _ Context) perm.Request {
	var in struct {
		Action string `json:"action"`
		ID     string `json:"id"`
	}
	_ = parseJSON(args, &in)
	act := strings.ToLower(strings.TrimSpace(in.Action))
	req := perm.Request{Tool: "mcp_manage", Command: act, Summary: "MCP " + act}
	if in.ID != "" {
		req.Summary = "MCP " + act + " " + in.ID
	}
	if act == "add" || act == "remove" {
		req.Destructive = act == "remove"
	}
	return req
}

func (mcpManage) Run(ctx context.Context, args string, c Context) (string, error) {
	var in struct {
		Action string `json:"action"`
		ID     string `json:"id"`
		Query  string `json:"query"`
	}
	if err := parseJSON(args, &in); err != nil {
		return "", err
	}
	act := strings.ToLower(strings.TrimSpace(in.Action))
	switch act {
	case "list":
		if c.MCPList == nil {
			return "no MCP manager", nil
		}
		return c.MCPList(), nil
	case "suggest":
		if c.MCPSuggest == nil {
			return "no MCP suggest", nil
		}
		q := in.Query
		if q == "" {
			q = in.ID
		}
		return c.MCPSuggest(q), nil
	case "add":
		if c.MCPAdd == nil {
			return "", fmt.Errorf("mcp add not wired")
		}
		if in.ID == "" {
			return "", fmt.Errorf("id is required")
		}
		return c.MCPAdd(ctx, in.ID)
	case "remove":
		if c.MCPRemove == nil {
			return "", fmt.Errorf("mcp remove not wired")
		}
		if in.ID == "" {
			return "", fmt.Errorf("id is required")
		}
		return c.MCPRemove(ctx, in.ID)
	default:
		return "", fmt.Errorf("unknown action %q (list|suggest|add|remove)", in.Action)
	}
}

type verifyTool struct{}

func (verifyTool) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Name:        "verify",
		Description: "Run the workspace test suite (go test, npm test, or pytest). Call this after code changes to prove they work.",
		Parameters:  schema(map[string]any{}, []string{}),
	}
}

func (verifyTool) Permission(_ string, _ Context) perm.Request {
	return perm.Request{Tool: "verify", Summary: "run workspace tests", Command: "verify"}
}

func (verifyTool) Run(ctx context.Context, _ string, c Context) (string, error) {
	if c.Verify == nil {
		return "", fmt.Errorf("verify not wired")
	}
	return c.Verify(ctx)
}
