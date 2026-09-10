package projectctx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadPreservesOrderAndRulePrefixLimit(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("agents\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("claude\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rulesDir := filepath.Join(root, ".picogent")
	if err := os.Mkdir(rulesDir, 0o700); err != nil {
		t.Fatal(err)
	}
	longRules := strings.Repeat("x", maxRulesBytes+64)
	if err := os.WriteFile(filepath.Join(rulesDir, "rules.md"), []byte(longRules), 0o600); err != nil {
		t.Fatal(err)
	}

	got := Load(root)
	agents := strings.Index(got, "── AGENTS.md ──\nagents")
	claude := strings.Index(got, "── CLAUDE.md ──\nclaude")
	rulesHeader := "── .picogent/rules.md ──\n"
	rules := strings.Index(got, rulesHeader)
	if agents < 0 || claude < 0 || rules < 0 {
		t.Fatalf("Load omitted a normal rule: %q", got)
	}
	if agents >= claude || claude >= rules {
		t.Fatalf("Load changed rule order: %q", got)
	}
	rulesText := got[rules+len(rulesHeader):]
	if len(rulesText) != maxRulesBytes {
		t.Fatalf("rule prefix length=%d, want %d", len(rulesText), maxRulesBytes)
	}
}

func TestLoadSkipsSymlinkedRuleTarget(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "outside.md")
	if err := os.WriteFile(outsideFile, []byte("outside-target-marker\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(root, "AGENTS.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if got := Load(root); strings.Contains(got, "outside-target-marker") {
		t.Fatalf("Load followed a symlinked rule target: %q", got)
	}
}

func TestLoadSkipsSymlinkedRuleParent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "rules.md"), []byte("outside-parent-marker\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".picogent")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if got := Load(root); strings.Contains(got, "outside-parent-marker") {
		t.Fatalf("Load followed a symlinked rule parent: %q", got)
	}
}
