package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/perm"
)

func TestMeasureToolIsRegisteredAndDoesNotAcceptACommand(t *testing.T) {
	workspace := t.TempDir()
	writeToolTestFile(t, workspace, "go.mod", "module example.test/measuretool\n\ngo 1.25\n")
	writeToolTestFile(t, workspace, "bench_test.go", `package measuretool

import "testing"

func BenchmarkExpected(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = i * 2
	}
}
`)
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
	if !strings.HasPrefix(output, "measure PASS") || !strings.Contains(output, "BenchmarkExpected") || strings.Contains(output, "should-not-run") {
		t.Fatalf("supported workspace measurement = %q", output)
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

func writeToolTestFile(t *testing.T, workspace, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(workspace, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
