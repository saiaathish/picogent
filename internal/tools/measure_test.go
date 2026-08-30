package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/perm"
)

func TestMeasureToolIsRegisteredAndDoesNotAcceptACommand(t *testing.T) {
	workspace := t.TempDir()
	registry := NewRegistry(Context{Workspace: workspace})
	tool, ok := registry.Get("measure")
	if !ok {
		t.Fatal("measure tool is not registered")
	}
	if got := tool.Permission(`{"command":"rm -rf /"}`, registry.Ctx); got.Destructive || got.OutsideWorkspace || got.Command != "" {
		t.Fatalf("measure permission accepted command-shaped input: %#v", got)
	}
	output, err := tool.Run(context.Background(), `{"command":"echo should-not-run"}`, registry.Ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(output, "measure INCONCLUSIVE") {
		t.Fatalf("unsupported workspace measurement = %q", output)
	}
}

func TestMeasureToolRequiresTheNormalPermissionGate(t *testing.T) {
	workspace := t.TempDir()
	tool := measureTool{}
	request := tool.Permission("{}", Context{Workspace: workspace})
	decision, err := perm.New(config.ModeFast, workspace, nil).Check(context.Background(), request)
	if err != nil || decision != perm.Deny {
		t.Fatalf("measure permission decision = %s, %v; want deny", decision, err)
	}
}
