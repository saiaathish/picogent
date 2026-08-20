package extensions_test

import (
	"testing"

	"github.com/saiaathish/picogent/internal/extensions"
)

func TestRecommendGitHub(t *testing.T) {
	recs := extensions.Recommend("fix the github pull request", nil, nil)
	if len(recs) == 0 {
		t.Fatal("expected github recommendation")
	}
	if recs[0].ID != "mcp-github" {
		t.Fatalf("got %s", recs[0].ID)
	}
}

func TestRecommendDismissed(t *testing.T) {
	recs := extensions.Recommend("github issue", map[string]bool{"mcp-github": true}, nil)
	for _, r := range recs {
		if r.ID == "mcp-github" {
			t.Fatal("should not recommend installed/dismissed")
		}
	}
}

func TestRecommendIgnoresSmallTalk(t *testing.T) {
	recs := extensions.Recommend("Reply with the single word pong. Do not use tools.", nil, nil)
	if len(recs) != 0 {
		t.Fatalf("unexpected recs %#v", recs)
	}
}

func TestSearch(t *testing.T) {
	items := extensions.Search("postgres", nil)
	found := false
	for _, it := range items {
		if it.ID == "mcp-postgres" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected postgres in search")
	}
}
