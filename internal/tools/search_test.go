package tools

import "testing"

func TestGlobMatch(t *testing.T) {
	cases := []struct {
		pattern string
		rel     string
		want    bool
	}{
		{"**/*.go", "internal/tools/search.go", true},
		{"**/*.go", "search.go", true},
		{"**/*.go", "vendor/x.go", true},
		{"**/foo/**/bar", "foo/a/bar", true},
		{"**/foo/**/bar", "foo/bar", true},
		{"foo/**/bar.go", "foo/internal/bar.go", true},
		{"foo/**/bar.go", "other/foo/bar.go", false},
		{"*.go", "search.go", true},
		{"*.go", "tools/search.go", false},
	}
	for _, tc := range cases {
		got := globMatch(tc.pattern, tc.rel)
		if got != tc.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", tc.pattern, tc.rel, got, tc.want)
		}
	}
}

func TestGlobMatchNoStackOverflow(t *testing.T) {
	// Patterns that previously recursed forever when the **/ prefix branch missed.
	patterns := []string{
		"**/foo/**/bar",
		"**/**/x.go",
		"**/a/**/b/**/c.go",
	}
	rels := []string{"foo/x/bar", "deep/nested/x.go", "a/1/b/2/c.go", "nope.txt"}
	for _, pattern := range patterns {
		for _, rel := range rels {
			_ = globMatch(pattern, rel)
		}
	}
}
