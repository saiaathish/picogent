package agent

import (
	"path/filepath"
	"testing"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/evolve"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/tools"
)

func TestRememberVerificationPreservesWorkspacePathIdentity(t *testing.T) {
	staleWorkspace := filepath.Join(t.TempDir(), "workspace")
	workspace := staleWorkspace + " "
	t.Setenv("PICOGENT_HOME", t.TempDir())
	if verificationStatus("VERIFY PASS") != "PASS" {
		t.Fatal("test evidence must be recognized as passing")
	}
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Mode = config.ModeFast
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{Workspace: workspace})
	a := New(cfg, &llm.Scripted{}, reg, perm.New(config.ModeFast, workspace, nil))
	a.SetMemory(evolve.Store{Workspace: staleWorkspace})

	a.rememberVerification("VERIFY PASS")

	stored, err := evolve.Load(staleWorkspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.VerificationRoutes) != 0 {
		t.Fatalf("verification for %q was written into stale workspace memory %q", workspace, staleWorkspace)
	}
}

func TestRememberVerificationInitializesEmptyWorkspaceMemory(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("PICOGENT_HOME", t.TempDir())
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Mode = config.ModeFast
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{Workspace: workspace})
	a := New(cfg, &llm.Scripted{}, reg, perm.New(config.ModeFast, workspace, nil))
	a.SetGoal("verify this workspace")

	a.rememberVerification("VERIFY PASS")

	stored, err := evolve.Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Workspace != workspace || len(stored.VerificationRoutes) != 1 {
		t.Fatalf("verification memory = %#v, want an initialized route for %q", stored, workspace)
	}
}
