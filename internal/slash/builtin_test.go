package slash

import (
	"testing"
)

func TestCatalogIncludesDocumentedBuiltins(t *testing.T) {
	items := Catalog(t.TempDir())
	want := []string{
		"commit", "review", "status", "diff", "compact", "memory",
		"goal", "agent", "ask", "plan", "debug", "clear",
	}
	got := map[string]Item{}
	for _, it := range items {
		got[it.Name] = it
	}
	for _, name := range want {
		if _, ok := got[name]; !ok {
			t.Fatalf("Catalog missing %q (have %#v)", name, items)
		}
	}
	if got["goal"].Insert != "/goal " {
		t.Fatalf("goal insert=%q, want %q", got["goal"].Insert, "/goal ")
	}
}
