package extensions_test

import (
	"testing"

	"github.com/saiaathish/picogent/internal/extensions"
)

func TestClaudeLibrary(t *testing.T) {
	items, stats := extensions.LoadClaudeLibrary()
	if len(items) < 50 {
		t.Fatalf("expected 50+ claude plugins, got %d", len(items))
	}
	if stats.Plugins < 50 {
		t.Fatalf("stats plugins %d", stats.Plugins)
	}
	filtered := extensions.FilterClaude(items, extensions.KindPlugin, "")
	if len(filtered) < 50 {
		t.Fatalf("plugin filter got %d", len(filtered))
	}
}

func TestBrowsePlugins(t *testing.T) {
	items, stats, err := extensions.Browse(extensions.KindPlugin, "", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) < 20 {
		t.Fatalf("browse plugins got %d items", len(items))
	}
	if stats.Plugins < 50 {
		t.Fatalf("stats.Plugins=%d", stats.Plugins)
	}
}
