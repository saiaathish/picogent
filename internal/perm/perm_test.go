package perm_test

import (
	"testing"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/perm"
)

func TestFastAllowsInWorkspaceWrite(t *testing.T) {
	g := perm.New(config.ModeFast, "/tmp/ws", nil)
	d, err := g.Check(nil, perm.Request{Tool: "write_file", Path: "a.go"})
	if err != nil || d != perm.Allow {
		t.Fatalf("%v %v", d, err)
	}
}

func TestFastStillBlocksRM(t *testing.T) {
	g := perm.New(config.ModeFast, "/tmp/ws", nil)
	req := perm.ClassifyBash("rm -rf /", "/tmp/ws")
	d, _ := g.Check(nil, req)
	if d != perm.Deny {
		t.Fatalf("expected deny, got %s", d)
	}
}

func TestFastAllowsVerify(t *testing.T) {
	g := perm.New(config.ModeFast, "/tmp/ws", nil)
	d, _ := g.Check(nil, perm.Request{Tool: "verify"})
	if d != perm.Allow {
		t.Fatalf("expected allow, got %s", d)
	}
}

func TestSafeAllowsVerify(t *testing.T) {
	g := perm.New(config.ModeSafe, "/tmp/ws", nil)
	d, _ := g.Check(nil, perm.Request{Tool: "verify"})
	if d != perm.Allow {
		t.Fatalf("expected allow, got %s", d)
	}
}

func TestMCPManageListAutoAllows(t *testing.T) {
	g := perm.New(config.ModeSafe, "/tmp/ws", nil)
	d, _ := g.Check(nil, perm.Request{Tool: "mcp_manage", Command: "list"})
	if d != perm.Allow {
		t.Fatalf("list should auto-allow, got %s", d)
	}
	d, _ = g.Check(nil, perm.Request{Tool: "mcp_manage", Command: "add", Summary: "MCP add mcp-github"})
	if d != perm.Deny {
		t.Fatalf("add should ask, got %s", d)
	}
}

func TestSafeBlocksWriteWithoutPrompter(t *testing.T) {
	g := perm.New(config.ModeSafe, "/tmp/ws", nil)
	d, _ := g.Check(nil, perm.Request{Tool: "write_file", Path: "a.go"})
	if d != perm.Deny {
		t.Fatalf("expected deny, got %s", d)
	}
}
