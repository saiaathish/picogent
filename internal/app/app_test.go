package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/saiaathish/picogent/internal/config"
)

func isolatedAppLoad(t *testing.T) (workspace string) {
	t.Helper()
	home := t.TempDir()
	userHome := t.TempDir()
	workspace = t.TempDir()
	t.Setenv("PICOGENT_HOME", home)
	t.Setenv("PICOGENT_CODEX_HOME", t.TempDir())
	t.Setenv("HOME", userHome)
	t.Setenv("USERPROFILE", userHome) // Windows os.UserHomeDir ignores HOME.
	t.Setenv("PICOGENT_MODE", "")
	t.Setenv("PICOGENT_PROVIDER", "")
	t.Setenv("PICOGENT_BASE_URL", "")
	t.Setenv("PICOGENT_MODEL", "")
	t.Setenv("PICOGENT_ROUTER", "")
	t.Chdir(workspace)
	return workspace
}

func TestLoadKeepsUserPermissionModeAgainstProjectConfig(t *testing.T) {
	workspace := isolatedAppLoad(t)

	user := config.Default()
	user.Provider = config.ProviderOllama
	user.Mode = config.ModeSafe
	if err := config.Save(user); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".picogent.yaml"), []byte("mode: fast\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, a, err := Load(".")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != config.ModeSafe {
		t.Fatalf("config mode = %q, want %q", cfg.Mode, config.ModeSafe)
	}
	if got := a.ConfigSnapshot().Mode; got != config.ModeSafe {
		t.Fatalf("agent mode = %q, want %q", got, config.ModeSafe)
	}
	if a.Gate == nil || a.Gate.Mode != config.ModeSafe {
		t.Fatalf("permission gate mode = %#v, want %q", a.Gate, config.ModeSafe)
	}
}

func TestLoadIgnoresWorkspaceMCPConfig(t *testing.T) {
	workspace := isolatedAppLoad(t)
	user := config.Default()
	user.Provider = config.ProviderOllama
	if err := config.Save(user); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".mcp.json"), []byte("not JSON"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, a, err := Load(".")
	if err != nil {
		t.Fatalf("workspace MCP config must not affect startup: %v", err)
	}
	if a.Tools.MCPManagerSnapshot() != nil {
		t.Fatal("workspace MCP config was connected")
	}
}
