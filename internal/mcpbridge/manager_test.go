package mcpbridge

import "testing"

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
