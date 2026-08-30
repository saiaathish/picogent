package mcpbridge_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/saiaathish/picogent/internal/mcpbridge"
)

func TestLoadServersFromYAML(t *testing.T) {
	home := t.TempDir()
	yaml := `servers:
  github:
    url: http://127.0.0.1:9010/mcp
    type: http
`
	if err := os.WriteFile(filepath.Join(home, "mcp.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	// Point PICOGENT_HOME at temp and isolate from real ~/.cursor
	t.Setenv("PICOGENT_HOME", home)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME")) // Windows UserHomeDir ignores HOME
	got, err := mcpbridge.LoadServers("")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d servers", len(got))
	}
	if got["github"].URL == "" {
		t.Fatal("missing url")
	}
}

func TestLoadServersIgnoresWorkspaceConfig(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "myapp")
	picogent := filepath.Join(root, ".picogent")
	if err := os.MkdirAll(proj, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(picogent, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PICOGENT_HOME", picogent)
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root) // Windows UserHomeDir ignores HOME
	userCursor := `{"mcpServers":{"shared":{"url":"http://user-cursor","type":"http"}}}`
	userPicogent := `servers:
  shared:
    url: http://user-picogent
    type: http
  personal:
    url: http://user-picogent
    type: http
`
	workspaceCursor := `{"mcpServers":{"shared":{"command":"must-not-run"}}}`
	workspaceMCP := `{"mcpServers":{"personal":{"enabled":false},"project-only":{"command":"must-not-run"}}}`
	if err := os.MkdirAll(filepath.Join(root, ".cursor"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".cursor", "mcp.json"), []byte(userCursor), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(picogent, "mcp.yaml"), []byte(userPicogent), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(proj, ".cursor"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, ".cursor", "mcp.json"), []byte(workspaceCursor), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, ".mcp.json"), []byte(workspaceMCP), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := mcpbridge.LoadServers(proj)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d servers, want 2", len(got))
	}
	if got["shared"].URL != "http://user-picogent" || got["personal"].URL != "http://user-picogent" {
		t.Fatalf("%+v", got)
	}
	if _, ok := got["project-only"]; ok {
		t.Fatalf("workspace MCP server was loaded: %+v", got)
	}
}

func TestLoadServersIgnoresMalformedWorkspaceConfig(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(filepath.Join(workspace, ".cursor"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PICOGENT_HOME", filepath.Join(root, ".picogent"))
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root) // Windows os.UserHomeDir ignores HOME
	if err := os.WriteFile(filepath.Join(workspace, ".cursor", "mcp.json"), []byte("not JSON"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".mcp.json"), []byte("not JSON"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := mcpbridge.LoadServers(workspace)
	if err != nil {
		t.Fatalf("workspace config must not be read: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d servers, want none", len(got))
	}
}

func TestLoadServersRejectsSymlinkedUserConfig(t *testing.T) {
	root := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("PICOGENT_HOME", root)
	t.Setenv("HOME", userHome)
	t.Setenv("USERPROFILE", userHome)

	tests := []struct {
		name string
		path string
	}{
		{name: "picogent", path: filepath.Join(root, "mcp.yaml")},
		{name: "cursor", path: filepath.Join(userHome, ".cursor", "mcp.json")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outside := filepath.Join(t.TempDir(), "config")
			if err := os.WriteFile(outside, []byte("servers: {}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(test.path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, test.path); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}

			if _, err := mcpbridge.LoadServers(""); err == nil {
				t.Fatal("LoadServers accepted a symlinked user config")
			}
			if _, err := os.Lstat(test.path); err != nil {
				t.Fatalf("symlink was changed after rejection: %v", err)
			}
		})
	}
}

func TestRemoveServerRejectsSymlinkedUserConfig(t *testing.T) {
	root := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("PICOGENT_HOME", root)
	t.Setenv("HOME", userHome)
	t.Setenv("USERPROFILE", userHome)

	outside := filepath.Join(t.TempDir(), "mcp.yaml")
	if err := os.WriteFile(outside, []byte("servers:\n  target:\n    command: keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "mcp.yaml")
	if err := os.Symlink(outside, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := mcpbridge.RemoveServer("target"); err == nil {
		t.Fatal("RemoveServer accepted a symlinked user config")
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("symlink was changed after rejection: %v", err)
	}
	got, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "servers:\n  target:\n    command: keep\n" {
		t.Fatalf("symlink target changed to %q", got)
	}
}
