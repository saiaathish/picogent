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

func TestDropServerEmpty(t *testing.T) {
	var m *Manager
	m.DropServer("x")
	(&Manager{}).DropServer("x")
}
