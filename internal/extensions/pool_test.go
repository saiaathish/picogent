package extensions

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/mcpbridge"
)

func TestPoolRollbackActivatedOnlyRollsBackLatestActivationTransaction(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PICOGENT_HOME", t.TempDir())

	p := NewPool(t.TempDir(), nil, nil)
	github := ByID("mcp-github")
	if github == nil {
		t.Fatal("missing github catalog item")
	}
	githubEntry, err := p.activate(*github)
	if err != nil {
		t.Fatal(err)
	}
	p.Transient = append(p.Transient, github.ID)
	p.activatedUndo = append(p.activatedUndo, githubEntry.Clone())
	// A later ensure starts a new transaction; the earlier activation is now
	// part of the retained pool state and must not be rolled back with it.
	p.activatedUndo = nil
	browser := ByID("mcp-browseros")
	if browser == nil {
		t.Fatal("missing BrowserOS catalog item")
	}
	browserEntry, err := p.activate(*browser)
	if err != nil {
		t.Fatal(err)
	}
	p.Transient = append(p.Transient, browser.ID)
	p.activatedUndo = append(p.activatedUndo, browserEntry.Clone())

	if err := p.RollbackActivated(); err != nil {
		t.Fatal(err)
	}
	if len(p.Transient) != 1 || p.Transient[0] != "mcp-github" {
		t.Fatalf("transient extensions after rollback = %#v, want [mcp-github]", p.Transient)
	}

	servers, err := mcpbridge.LoadServers("")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := servers["github"]; !ok {
		t.Fatalf("latest rollback removed prior github server: %#v", servers)
	}
	if _, ok := servers["browseros"]; ok {
		t.Fatalf("latest browseros activation survived rollback: %#v", servers)
	}
}

func TestPoolDoesNotAutoActivateExternalExtensions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PICOGENT_HOME", t.TempDir())

	// Keep the optional Claude marketplace out of this policy test.
	claudeCacheMu.Lock()
	previousCache, previousStats, previousLoaded := claudeCache, claudeStats, claudeLoaded
	claudeCache = []SearchResult{{ID: "claude:unrelated", Keywords: []string{"unrelated"}}}
	claudeStats = LibraryStats{}
	claudeLoaded = time.Now()
	claudeCacheMu.Unlock()
	t.Cleanup(func() {
		claudeCacheMu.Lock()
		claudeCache, claudeStats, claudeLoaded = previousCache, previousStats, previousLoaded
		claudeCacheMu.Unlock()
	})

	p := NewPool(t.TempDir(), nil, nil)
	activated, err := p.EnsureForPrompt("github")
	if err != nil {
		t.Fatal(err)
	}
	if len(activated) != 0 || len(p.Transient) != 0 {
		t.Fatalf("external MCP extension was auto-activated: activated=%#v transient=%#v", activated, p.Transient)
	}
	servers, err := mcpbridge.LoadServers("")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := servers["github"]; ok {
		t.Fatalf("automatic recommendation wrote the github MCP server: %#v", servers)
	}
}

func TestPoolPromptPreparationDoesNotAcquireRemoteExtensions(t *testing.T) {
	home := t.TempDir()
	picogentHome := filepath.Join(t.TempDir(), "picogent")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PICOGENT_HOME", picogentHome)

	p := NewPool(t.TempDir(), nil, nil)
	activated, err := p.EnsureForPrompt("please perform a security review")
	if err != nil {
		t.Fatal(err)
	}
	if len(activated) != 0 || len(p.Transient) != 0 {
		t.Fatalf("prompt preparation activated an extension: activated=%#v transient=%#v", activated, p.Transient)
	}
	if _, err := os.Stat(filepath.Join(home, ".cursor", "skills-cursor")); !os.IsNotExist(err) {
		t.Fatalf("prompt preparation created a skill cache: %v", err)
	}
	if _, err := os.Stat(filepath.Join(picogentHome, "claude-marketplace.json")); !os.IsNotExist(err) {
		t.Fatalf("prompt preparation wrote a marketplace cache: %v", err)
	}
}

func TestAutoActivationPolicyAllowsSkillsOnly(t *testing.T) {
	for _, tc := range []struct {
		name string
		item Item
		want bool
	}{
		{name: "skill", item: Item{ID: "skill-review", Kind: KindSkill}, want: true},
		{name: "catalog MCP", item: Item{ID: "mcp-github", Kind: KindMCP}},
		{name: "Claude plugin without MCP metadata", item: Item{ID: "claude:helper", Kind: KindPlugin}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := autoActivationAllowed(tc.item); got != tc.want {
				t.Fatalf("autoActivationAllowed(%#v) = %v, want %v", tc.item, got, tc.want)
			}
		})
	}
}
