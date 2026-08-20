package perm

import (
	"strings"
	"testing"
)

func TestFriendlyHintBash(t *testing.T) {
	h := FriendlyHint(Request{Tool: "bash", Command: "go test ./..."})
	if h == "" || !strings.Contains(h, "test") {
		t.Fatal(h)
	}
}

func TestFriendlyHintEdit(t *testing.T) {
	h := FriendlyHint(Request{Tool: "edit_file", Path: "internal/agent/agent.go"})
	if !strings.Contains(h, "agent.go") {
		t.Fatal(h)
	}
}
