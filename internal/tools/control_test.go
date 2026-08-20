package tools

import (
	"context"
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

func TestVerifyTool(t *testing.T) {
	c := Context{
		Verify: func(context.Context) (string, error) { return "verify PASS", nil },
	}
	out, err := verifyTool{}.Run(context.Background(), "{}", c)
	if err != nil || out != "verify PASS" {
		t.Fatalf("%q %v", out, err)
	}
}
