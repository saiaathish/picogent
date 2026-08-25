package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/tools"
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
	t.Setenv("USERPROFILE", root)
	t.Setenv("PICOGENT_HOME", root)
	got := mcpSuggestText(t.TempDir(), "github pull request")
	if !strings.Contains(got, "mcp-github") {
		t.Fatalf("suggest: %q", got)
	}
}

func TestMCPListShowsNotConnected(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	t.Setenv("PICOGENT_HOME", root)
	ws := t.TempDir()
	a := &agent.Agent{}
	a.CFG.Workspace = ws
	if _, err := mcpAdd(a, "mcp-browseros"); err != nil {
		t.Fatal(err)
	}
	got := mcpListText(ws, nil)
	if !strings.Contains(got, "browseros") || !strings.Contains(got, "not connected") {
		t.Fatalf("list: %q", got)
	}
}

func TestMCPAddRemoveWritesConfig(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
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
	list := mcpListText(ws, nil)
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
	if got := mcpListText(ws, nil); got != "no MCP servers configured" {
		t.Fatalf("list after remove: %q", got)
	}
}

func TestMCPListEmpty(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	t.Setenv("PICOGENT_HOME", root)
	got := mcpListText(t.TempDir(), nil)
	if got != "no MCP servers configured" {
		t.Fatalf("list: %q", got)
	}
}

func TestWireRuntimeVerifiesEmptyTargetWithBroaderSuite(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), []byte("module example.com/wired\n\ngo 1.25\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "wired.go"), []byte("package wired\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "wired_test.go"), []byte("package wired\n\nimport \"testing\"\n\nfunc TestWired(t *testing.T) {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	ag := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	wireRuntime(ag)

	wired := ag.Tools.ContextSnapshot().VerifyTargets
	if wired == nil {
		t.Fatal("wireRuntime did not install VerifyTargets")
	}
	evidence, err := wired(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"verify PASS", "targeted SKIPPED", "broader PASS"} {
		if !strings.Contains(evidence, want) {
			t.Fatalf("wired verification missing %q: %s", want, evidence)
		}
	}
}
