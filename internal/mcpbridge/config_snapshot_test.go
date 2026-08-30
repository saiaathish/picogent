package mcpbridge_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/saiaathish/picogent/internal/mcpbridge"
)

func TestServerSnapshotRestoresOverwriteAndPreservesUnrelatedEntries(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PICOGENT_HOME", root)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	original := mcpbridge.ServerConfig{
		Command: "original",
		Args:    []string{"--old"},
		Env:     map[string]string{"TOKEN": "old"},
	}
	unrelated := mcpbridge.ServerConfig{URL: "http://unrelated", Type: "http"}
	if err := mcpbridge.SaveServer("target", original); err != nil {
		t.Fatal(err)
	}
	if err := mcpbridge.SaveServer("unrelated", unrelated); err != nil {
		t.Fatal(err)
	}

	snapshot, err := mcpbridge.SnapshotServer("target")
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Present {
		t.Fatal("existing target was not marked present")
	}
	// Prove the snapshot owns its nested mutable values.
	snapshot.Config.Args[0] = "mutated"
	snapshot.Config.Env["TOKEN"] = "mutated"
	if err := mcpbridge.RestoreServers(map[string]mcpbridge.ServerSnapshot{
		"target": snapshot,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := mcpbridge.LoadServers("")
	if err != nil {
		t.Fatal(err)
	}
	if got["target"].Command != original.Command || got["target"].Args[0] != "mutated" || got["target"].Env["TOKEN"] != "mutated" {
		t.Fatalf("snapshot restore did not use the captured value: %#v", got["target"])
	}
	if got["unrelated"].URL != unrelated.URL || got["unrelated"].Type != unrelated.Type {
		t.Fatalf("unrelated MCP entry changed: got=%#v want=%#v", got["unrelated"], unrelated)
	}

	if err := mcpbridge.SaveServer("new", mcpbridge.ServerConfig{URL: "http://new", Type: "http"}); err != nil {
		t.Fatal(err)
	}
	absent, err := mcpbridge.SnapshotServer("missing")
	if err != nil {
		t.Fatal(err)
	}
	if absent.Present {
		t.Fatal("missing server was marked present")
	}
	if err := mcpbridge.SaveServer("missing", mcpbridge.ServerConfig{Command: "temporary"}); err != nil {
		t.Fatal(err)
	}
	if err := mcpbridge.RestoreServers(map[string]mcpbridge.ServerSnapshot{"missing": absent}); err != nil {
		t.Fatal(err)
	}
	got, err = mcpbridge.LoadServers("")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["missing"]; ok {
		t.Fatalf("absent snapshot restored a server: %#v", got["missing"])
	}
}

func TestServerPrefixSnapshotRestoresOnlyPluginEntries(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PICOGENT_HOME", root)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	for name, command := range map[string]string{
		"claude-demo":        "old-main",
		"claude-demo-search": "old-search",
		"other":              "keep",
	} {
		if err := mcpbridge.SaveServer(name, mcpbridge.ServerConfig{Command: command}); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := mcpbridge.SnapshotServersWithPrefix("claude-demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := mcpbridge.SaveServer("claude-demo", mcpbridge.ServerConfig{Command: "new-main"}); err != nil {
		t.Fatal(err)
	}
	if err := mcpbridge.SaveServer("claude-demo-extra", mcpbridge.ServerConfig{Command: "new-extra"}); err != nil {
		t.Fatal(err)
	}
	if err := mcpbridge.RestoreServersWithPrefix("claude-demo", snapshot); err != nil {
		t.Fatal(err)
	}
	got, err := mcpbridge.LoadServers("")
	if err != nil {
		t.Fatal(err)
	}
	if got["claude-demo"].Command != "old-main" || got["claude-demo-search"].Command != "old-search" {
		t.Fatalf("plugin entries not restored: %#v", got)
	}
	if _, ok := got["claude-demo-extra"]; ok {
		t.Fatalf("new plugin entry survived prefix restore: %#v", got["claude-demo-extra"])
	}
	if got["other"].Command != "keep" {
		t.Fatalf("unrelated entry changed: %#v", got["other"])
	}
}

func TestConcurrentServerUpdatesPreserveDistinctEntries(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PICOGENT_HOME", root)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	const count = 32
	start := make(chan struct{})
	errs := make(chan error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("server-%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- mcpbridge.SaveServer(name, mcpbridge.ServerConfig{Command: name})
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	servers, err := mcpbridge.LoadServers("")
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != count {
		t.Fatalf("concurrent saves preserved %d servers, want %d: %#v", len(servers), count, servers)
	}
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("server-%d", i)
		if servers[name].Command != name {
			t.Fatalf("server %q was lost or changed: %#v", name, servers[name])
		}
	}
}

func TestServerUpdatePreservesExistingFileMode(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PICOGENT_HOME", root)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	if err := mcpbridge.SaveServer("server", mcpbridge.ServerConfig{Command: "before"}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "mcp.yaml")
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := mcpbridge.SaveServer("server", mcpbridge.ServerConfig{Command: "after"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("config mode = %o, want 640", got)
	}
}

func TestConcurrentRestoreAndSavePreserveUnrelatedEntries(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PICOGENT_HOME", root)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	if err := mcpbridge.SaveServer("target", mcpbridge.ServerConfig{Command: "before"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := mcpbridge.SnapshotServer("target")
	if err != nil {
		t.Fatal(err)
	}
	const rounds = 24
	errs := make(chan error, rounds*2)
	var wg sync.WaitGroup
	for i := 0; i < rounds; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			errs <- mcpbridge.RestoreServers(map[string]mcpbridge.ServerSnapshot{"target": snapshot})
		}()
		i := i
		go func() {
			defer wg.Done()
			errs <- mcpbridge.SaveServer("unrelated", mcpbridge.ServerConfig{Command: fmt.Sprintf("keep-%d", i)})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	servers, err := mcpbridge.LoadServers("")
	if err != nil {
		t.Fatal(err)
	}
	if servers["target"].Command != "before" {
		t.Fatalf("target was not restored: %#v", servers["target"])
	}
	if servers["unrelated"].Command == "" {
		t.Fatalf("unrelated entry was lost: %#v", servers)
	}
}

func TestConcurrentReadersNeverParsePartialMCPConfig(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PICOGENT_HOME", root)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	if err := mcpbridge.SaveServer("changing", mcpbridge.ServerConfig{Command: "initial"}); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	done := make(chan struct{})
	errs := make(chan error, 4)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < 160; i++ {
			if err := mcpbridge.SaveServer("changing", mcpbridge.ServerConfig{
				Command: fmt.Sprintf("command-%d", i),
				Args:    []string{fmt.Sprintf("arg-%d", i), "padding-padding-padding"},
			}); err != nil {
				errs <- err
				return
			}
		}
		close(done)
	}()
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for {
				select {
				case <-done:
					return
				default:
				}
				if _, err := mcpbridge.LoadServers(""); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	if _, err := os.Stat(filepath.Join(root, "mcp.yaml")); err != nil {
		t.Fatal(err)
	}
}
