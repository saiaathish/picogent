package perm_test

import (
	"testing"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/perm"
)

func TestFastAllowsInWorkspaceWrite(t *testing.T) {
	g := perm.New(config.ModeFast, "/tmp/ws", nil)
	d, err := g.Check(nil, perm.Request{Tool: "write_file", Path: "a.go"})
	if err != nil || d != perm.Allow {
		t.Fatalf("%v %v", d, err)
	}
}

func TestFastStillBlocksRM(t *testing.T) {
	g := perm.New(config.ModeFast, "/tmp/ws", nil)
	req := perm.ClassifyBash("rm -rf /", "/tmp/ws")
	d, _ := g.Check(nil, req)
	if d != perm.Deny {
		t.Fatalf("expected deny, got %s", d)
	}
}

func TestSafeBlocksWriteWithoutPrompter(t *testing.T) {
	g := perm.New(config.ModeSafe, "/tmp/ws", nil)
	d, _ := g.Check(nil, perm.Request{Tool: "write_file", Path: "a.go"})
	if d != perm.Deny {
		t.Fatalf("expected deny, got %s", d)
	}
}
