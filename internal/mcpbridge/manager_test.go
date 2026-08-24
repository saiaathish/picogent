package mcpbridge

import (
	"os"
	"strings"
	"testing"
)

func TestDropServerKeepsOthers(t *testing.T) {
	m := &Manager{
		tools: []Tool{
			{PublicName: "mcp_a_x", Server: "a"},
			{PublicName: "mcp_b_y", Server: "b"},
		},
	}
	m.DropServer("a")
	got := m.Tools()
	if len(got) != 1 || got[0].Server != "b" {
		t.Fatalf("%+v", got)
	}
}

func TestDropFingerprintSameURL(t *testing.T) {
	m := &Manager{
		tools: []Tool{{PublicName: "mcp_neo_tabs", Server: "neo"}},
		conns: []conn{{name: "neo", fp: "url:http://127.0.0.1:9010/mcp"}},
	}
	m.dropFingerprint("url:http://127.0.0.1:9010/mcp")
	if len(m.Tools()) != 0 {
		t.Fatalf("still have tools: %+v", m.Tools())
	}
}

func TestDropServerEmpty(t *testing.T) {
	var m *Manager
	m.DropServer("x")
	(&Manager{}).DropServer("x")
}

func TestCommandEnvDoesNotInheritParentSecrets(t *testing.T) {
	t.Setenv("PICOGENT_TEST_SECRET", "should-not-cross")
	t.Setenv("PATH", "/usr/bin")
	env := commandEnv(map[string]string{"EXPLICIT_TOKEN": "configured", "PATH": "/custom/bin"})
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "PICOGENT_TEST_SECRET") || strings.Contains(joined, "should-not-cross") {
		t.Fatalf("parent secret inherited: %v", env)
	}
	if !strings.Contains(joined, "EXPLICIT_TOKEN=configured") {
		t.Fatalf("explicit MCP environment missing: %v", env)
	}
	if !strings.Contains(joined, "PATH=/custom/bin") {
		t.Fatalf("explicit PATH override missing: %v", env)
	}
	if _, ok := os.LookupEnv("PICOGENT_TEST_SECRET"); !ok {
		t.Fatal("test environment was unexpectedly changed")
	}
}
