package extensions

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/mcpbridge"
)

func TestInstallUndoRestoresExistingSkillInsteadOfDeletingIt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	dest := filepath.Join(home, ".cursor", "skills-cursor", "create-rule")
	if err := os.MkdirAll(filepath.Join(dest, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "SKILL.md"), []byte("original skill\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "references", "notes.txt"), []byte("original notes\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	it := ByID("skill-create-rule")
	if it == nil {
		t.Fatal("catalog skill missing")
	}
	_, entry, err := Install(*it, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dest, "SKILL.md"), []byte("overwritten\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "new.txt"), []byte("must disappear\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Undo(entry); err != nil {
		t.Fatal(err)
	}

	if got, err := os.ReadFile(filepath.Join(dest, "SKILL.md")); err != nil || string(got) != "original skill\n" {
		t.Fatalf("restored skill = %q, %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(dest, "references", "notes.txt")); err != nil || string(got) != "original notes\n" {
		t.Fatalf("restored nested skill = %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dest, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("extra file survived undo: %v", err)
	}
	info, err := os.Stat(dest)
	if err != nil || !info.IsDir() {
		t.Fatalf("existing skill directory was removed: info=%#v err=%v", info, err)
	}
}

func TestInstallFailureRestoresExistingMCPEntry(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PICOGENT_HOME", root)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	if err := mcpbridge.SaveServer("github", mcpbridge.ServerConfig{
		Command: "original",
		Args:    []string{"--keep"},
		Env:     map[string]string{"TOKEN": "old"},
	}); err != nil {
		t.Fatal(err)
	}

	_, entry, err := Install(Item{ID: "mcp-github", Kind: KindMCP}, t.TempDir())
	if err == nil {
		t.Fatal("install unexpectedly succeeded without an MCP config")
	}
	if entry.before != nil {
		t.Fatal("failed install retained rollback state")
	}
	got, err := mcpbridge.LoadServers("")
	if err != nil {
		t.Fatal(err)
	}
	if got["github"].Command != "original" || len(got["github"].Args) != 1 || got["github"].Env["TOKEN"] != "old" {
		t.Fatalf("MCP entry after failed install = %#v", got["github"])
	}
}

func TestUndoEntrySnapshotSurvivesRollbackAfterClose(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	it := ByID("skill-create-rule")
	if it == nil {
		t.Fatal("catalog skill missing")
	}
	root, err := skillRoot()
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, it.SkillPath)
	snapshot, err := CaptureState("", []string{it.ID})
	if err != nil {
		t.Fatal(err)
	}
	entry := UndoEntry{ID: "retry", ExtID: it.ID, Kind: it.Kind, before: snapshot}
	create := func() {
		t.Helper()
		if err := os.MkdirAll(dest, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dest, "SKILL.md"), []byte("installed"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	create()
	if err := Undo(entry); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("first undo left the skill behind: %v", err)
	}
	// Recreate the installed state as the GUI does when a failed runtime rebuild
	// rolls back a previously successful extension undo.
	create()
	if err := Undo(entry); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("closed rollback snapshot could not be retried: %v", err)
	}
}

func TestStateSnapshotCloneDeepCopiesMCPConfig(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("PICOGENT_HOME", root)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	enabled := true
	if err := mcpbridge.SaveServer("github", mcpbridge.ServerConfig{
		Command: "original",
		Args:    []string{"--keep"},
		Env:     map[string]string{"TOKEN": "old"},
		Enabled: &enabled,
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := CaptureState("", []string{"mcp-github"})
	if err != nil {
		t.Fatal(err)
	}
	clone := snapshot.Clone()
	if clone == nil || clone.mcp["github"].Config.Command != "original" {
		t.Fatalf("clone lost MCP snapshot: %#v", clone)
	}
	original := snapshot.mcp["github"]
	original.Config.Args[0] = "changed"
	original.Config.Env["TOKEN"] = "changed"
	*original.Config.Enabled = false
	snapshot.mcp["github"] = original
	got := clone.mcp["github"].Config
	if got.Args[0] != "--keep" || got.Env["TOKEN"] != "old" || got.Enabled == nil || !*got.Enabled {
		t.Fatalf("MCP snapshot clone shares mutable state: %#v", got)
	}
}

func TestSkillConfigPathPreservesNestedPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	root, err := skillRoot()
	if err != nil {
		t.Fatal(err)
	}
	got, err := SkillConfigPath(filepath.Join(root, "team", "review-security"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "team/review-security" {
		t.Fatalf("normalized nested skill path = %q", got)
	}
}

func TestClaudePluginDirRejectsInstallPathOutsidePluginRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	pluginsRoot := filepath.Join(home, ".claude", "plugins")
	if err := os.MkdirAll(pluginsRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	payload, err := json.Marshal(map[string]any{
		"plugins": map[string]any{
			"outside@1.0.0": []map[string]string{{"installPath": outside}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginsRoot, "installed_plugins.json"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := ClaudePluginDir("outside"); got != "" {
		t.Fatalf("outside Claude plugin path = %q", got)
	}
}

func TestActivateClaudePluginRejectsSymlinkedMCPConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PICOGENT_HOME", filepath.Join(home, ".picogent"))

	dir := filepath.Join(home, ".claude", "plugins", "marketplaces", "claude-plugins-official", "plugins", "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(outside, []byte(`{"mcpServers":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, ".mcp.json")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := ActivateClaudePlugin("demo"); err == nil {
		t.Fatal("symlinked Claude MCP config was accepted")
	}
}

func TestCacheClaudeMarketplaceRejectsSymlinkedTarget(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)
	target := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(target, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(home, "claude-marketplace.json")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := cacheClaudeMarketplace([]byte("replace\n")); err == nil {
		t.Fatal("marketplace cache accepted a symlinked target")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep\n" {
		t.Fatalf("symlink target changed to %q", got)
	}
}

func TestUndoRejectsSkillPathOutsideSkillsRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	outside := filepath.Join(t.TempDir(), "do-not-delete")
	if err := os.WriteFile(outside, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := Undo(UndoEntry{Kind: KindSkill, SkillPath: outside})
	if err == nil {
		t.Fatal("outside skill path was accepted")
	}
	if _, statErr := os.Stat(outside); statErr != nil {
		t.Fatalf("outside path changed after rejected undo: %v", statErr)
	}
}

func TestInstallRejectsUnsupportedPluginAndEmptyClaudeCache(t *testing.T) {
	home := t.TempDir()
	setupExtensionHome(t, home)
	if err := os.MkdirAll(claudePluginPath(home, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, entry, err := Install(Item{ID: "claude:demo", Name: "Demo", Kind: KindPlugin}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "no supported") {
		t.Fatalf("empty Claude plugin install = err %v, want truthful rejection", err)
	}
	if entry.before != nil {
		t.Fatal("rejected Claude plugin retained rollback state")
	}

	_, _, err = Install(Item{ID: "plugin-test", Name: "Built-in", Kind: KindPlugin}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("unsupported built-in plugin install = err %v, want explicit rejection", err)
	}
}

func TestClaudeActivationUsesCollisionFreeNamespaceAndExactRemoval(t *testing.T) {
	home := t.TempDir()
	setupExtensionHome(t, home)
	writeClaudeMCPPlugin(t, home, "demo", map[string]mcpbridge.ServerConfig{
		"extra": {Command: "demo-extra"},
	})
	writeClaudeMCPPlugin(t, home, "demo-extra", map[string]mcpbridge.ServerConfig{
		"demo-extra": {Command: "other-plugin"},
	})

	if err := ActivateClaudePlugin("demo"); err != nil {
		t.Fatal(err)
	}
	if err := ActivateClaudePlugin("demo-extra"); err != nil {
		t.Fatal(err)
	}
	servers, err := mcpbridge.LoadServers("")
	if err != nil {
		t.Fatal(err)
	}
	demoKey := claudeMCPServerKey("demo", "extra")
	otherKey := claudeMCPServerKey("demo-extra", "demo-extra")
	if servers[demoKey].Command != "demo-extra" || servers[otherKey].Command != "other-plugin" {
		t.Fatalf("isolated Claude MCP entries = %#v", servers)
	}
	if demoKey == otherKey {
		t.Fatal("Claude MCP namespace is not injective")
	}

	p := NewPool(t.TempDir(), nil, nil)
	if err := p.deactivate("claude:demo"); err != nil {
		t.Fatal(err)
	}
	servers, err = mcpbridge.LoadServers("")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := servers[demoKey]; ok {
		t.Fatalf("demo MCP entry survived exact removal: %#v", servers)
	}
	if servers[otherKey].Command != "other-plugin" {
		t.Fatalf("removing demo damaged sibling plugin: %#v", servers)
	}
}

func TestInstallRejectsInvalidExistingSkillDestinations(t *testing.T) {
	home := t.TempDir()
	setupExtensionHome(t, home)
	root := filepath.Join(home, ".cursor", "skills-cursor")
	tests := []struct {
		name  string
		setup func(string) error
	}{
		{name: "regular-file", setup: func(path string) error {
			return os.WriteFile(path, []byte("not a directory"), 0o600)
		}},
		{name: "empty-directory", setup: func(path string) error {
			return os.Mkdir(path, 0o755)
		}},
		{name: "missing-skill-md", setup: func(path string) error {
			return os.MkdirAll(filepath.Join(path, "references"), 0o755)
		}},
		{name: "empty-skill-md", setup: func(path string) error {
			if err := os.Mkdir(path, 0o755); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(path, "SKILL.md"), nil, 0o644)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(root, test.name)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := test.setup(path); err != nil {
				t.Fatal(err)
			}
			_, err := installSkill(Item{ID: "skill-" + test.name, Name: test.name, Kind: KindSkill, SkillPath: test.name})
			if err == nil {
				t.Fatal("invalid existing skill destination was accepted")
			}
		})
	}

	validPath := filepath.Join(root, "valid")
	if err := os.MkdirAll(validPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(validPath, "SKILL.md"), []byte("valid skill"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := installSkill(Item{ID: "skill-valid", Name: "valid", Kind: KindSkill, SkillPath: "valid"}); err != nil {
		t.Fatalf("valid existing skill was not idempotent: %v", err)
	}
}

func TestCursorSkillReadsAndRemovalStayInsideRoot(t *testing.T) {
	home := t.TempDir()
	setupExtensionHome(t, home)
	root := filepath.Join(home, ".cursor", "skills-cursor")
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "SKILL.md"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if got, err := SyncCursorSkills(); err != nil {
		t.Fatal(err)
	} else if len(got) != 0 {
		t.Fatalf("symlink escape appeared as a skill: %#v", got)
	}
	if got := SkillsPrompt([]string{"escape", "../outside"}); got != "" {
		t.Fatalf("unsafe skill path produced prompt: %q", got)
	}
	if err := removeSkill("escape"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outside, "SKILL.md")); err != nil {
		t.Fatalf("outside skill was affected by rooted removal: %v", err)
	}
}

func TestFailedSkillCopyRemovesPartialDestination(t *testing.T) {
	home := t.TempDir()
	setupExtensionHome(t, home)
	repo := t.TempDir()
	if err := runGit(repo, "init", "--quiet"); err != nil {
		t.Fatal(err)
	}
	if err := runGit(repo, "config", "user.email", "picogent@example.invalid"); err != nil {
		t.Fatal(err)
	}
	if err := runGit(repo, "config", "user.name", "Picogent Test"); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(repo, "skill")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("SKILL.md", filepath.Join(source, "unsupported-link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := runGit(repo, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := runGit(repo, "commit", "--quiet", "-m", "fixture"); err != nil {
		t.Fatal(err)
	}

	_, _, err := Install(Item{
		ID: "skill-partial", Name: "partial", Kind: KindSkill,
		SkillRepo: "file://" + repo, SkillPath: "skill",
	}, t.TempDir())
	if err == nil {
		t.Fatal("symlinked source skill unexpectedly copied")
	}
	dest := filepath.Join(home, ".cursor", "skills-cursor", "skill")
	if _, statErr := os.Lstat(dest); !os.IsNotExist(statErr) {
		t.Fatalf("failed copy left partial destination: %v", statErr)
	}
}

func setupExtensionHome(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PICOGENT_HOME", filepath.Join(home, ".picogent"))
}

func claudePluginPath(home, name string) string {
	return filepath.Join(home, ".claude", "plugins", "marketplaces", "claude-plugins-official", "plugins", name)
}

func writeClaudeMCPPlugin(t *testing.T, home, name string, servers map[string]mcpbridge.ServerConfig) {
	t.Helper()
	dir := claudePluginPath(home, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{"mcpServers": servers})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func runGit(dir string, args ...string) error {
	cmdArgs := append([]string{"-C", dir}, args...)
	return exec.Command("git", cmdArgs...).Run()
}
