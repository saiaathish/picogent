package mcpbridge_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/saiaathish/picogent/internal/mcpbridge"
)

func TestLoadServersFromYAML(t *testing.T) {
	home := t.TempDir()
	yaml := `servers:
  github:
    url: http://127.0.0.1:9010/mcp
    type: http
`
	if err := os.WriteFile(filepath.Join(home, "mcp.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	// Point PICOGENT_HOME at temp and isolate from real ~/.cursor
	t.Setenv("PICOGENT_HOME", home)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME")) // Windows UserHomeDir ignores HOME
	got, err := mcpbridge.LoadServers("")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d servers", len(got))
	}
	if got["github"].URL == "" {
		t.Fatal("missing url")
	}
}

func TestLoadServersProjectOverride(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "myapp")
	picogent := filepath.Join(root, ".picogent")
	if err := os.MkdirAll(proj, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(picogent, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PICOGENT_HOME", picogent)
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root) // Windows UserHomeDir ignores HOME
	global := `{"mcpServers":{"a":{"url":"http://global","type":"http"}}}`
	projCfg := `{"mcpServers":{"b":{"url":"http://project","type":"http"}}}`
	if err := os.MkdirAll(filepath.Join(root, ".cursor"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".cursor", "mcp.json"), []byte(global), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, ".mcp.json"), []byte(projCfg), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := mcpbridge.LoadServers(proj)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d servers, want 2", len(got))
	}
	if got["a"].URL != "http://global" || got["b"].URL != "http://project" {
		t.Fatalf("%+v", got)
	}
}
