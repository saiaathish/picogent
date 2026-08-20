package app

import (
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/agent"
)

func TestMCPAddUnknown(t *testing.T) {
	a := &agent.Agent{}
	_, err := mcpAdd(a, "not-a-catalog-id")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMCPSuggestGitHub(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("PICOGENT_HOME", root)
	got := mcpSuggestText(t.TempDir(), "github pull request")
	if !strings.Contains(got, "mcp-github") {
		t.Fatalf("suggest: %q", got)
	}
}

func TestMCPAddRemoveWritesConfig(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("PICOGENT_HOME", root)
	ws := t.TempDir()
	a := &agent.Agent{}
	a.CFG.Workspace = ws
	msg, err := mcpAdd(a, "mcp-browseros")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(msg), "browser") && !strings.Contains(msg, "Installed") {
		t.Fatalf("add msg: %q", msg)
	}
	list := mcpListText(ws)
	if !strings.Contains(list, "browseros") {
		t.Fatalf("list after add: %q", list)
	}
	out, err := mcpRemove(a, "mcp-browseros")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "removed") {
		t.Fatalf("remove: %q", out)
	}
	if got := mcpListText(ws); got != "no MCP servers configured" {
		t.Fatalf("list after remove: %q", got)
	}
}

func TestMCPListEmpty(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("PICOGENT_HOME", root)
	got := mcpListText(t.TempDir())
	if got != "no MCP servers configured" {
		t.Fatalf("list: %q", got)
	}
}
