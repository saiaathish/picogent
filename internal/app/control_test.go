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

func TestMCPListEmpty(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("PICOGENT_HOME", root)
	got := mcpListText(t.TempDir())
	if got != "no MCP servers configured" {
		t.Fatalf("list: %q", got)
	}
}
