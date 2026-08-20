package tools

import (
	"context"
	"strings"
	"testing"
)

func TestMCPManageList(t *testing.T) {
	c := Context{
		MCPList: func() string { return "github" },
	}
	out, err := mcpManage{}.Run(context.Background(), `{"action":"list"}`, c)
	if err != nil || out != "github" {
		t.Fatalf("%q %v", out, err)
	}
}

func TestMCPManageAddRequiresID(t *testing.T) {
	c := Context{
		MCPAdd: func(context.Context, string) (string, error) { return "ok", nil },
	}
	_, err := mcpManage{}.Run(context.Background(), `{"action":"add"}`, c)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMCPManageAdd(t *testing.T) {
	var got string
	c := Context{
		MCPAdd: func(_ context.Context, id string) (string, error) {
			got = id
			return "installed " + id, nil
		},
	}
	out, err := mcpManage{}.Run(context.Background(), `{"action":"add","id":"mcp-fetch"}`, c)
	if err != nil || got != "mcp-fetch" || out != "installed mcp-fetch" {
		t.Fatalf("%q %q %v", out, got, err)
	}
}

func TestMCPManageRemove(t *testing.T) {
	var got string
	c := Context{
		MCPRemove: func(_ context.Context, id string) (string, error) {
			got = id
			return "removed " + id, nil
		},
	}
	out, err := mcpManage{}.Run(context.Background(), `{"action":"remove","id":"mcp-fetch"}`, c)
	if err != nil || got != "mcp-fetch" || !strings.Contains(out, "removed") {
		t.Fatalf("%q %q %v", out, got, err)
	}
}

func TestVerifyTool(t *testing.T) {
	c := Context{
		Verify: func(context.Context) (string, error) { return "verify PASS", nil },
	}
	out, err := verifyTool{}.Run(context.Background(), "{}", c)
	if err != nil || out != "verify PASS" {
		t.Fatalf("%q %v", out, err)
	}
}
