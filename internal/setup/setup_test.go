package setup_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/setup"
)

func TestNeedsSetupThenApply(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)
	t.Setenv("PICOGENT_CODEX_HOME", t.TempDir())
	if !config.NeedsSetup() {
		t.Fatal("expected needs setup")
	}
	ws := t.TempDir()
	cfg := config.Default()
	got, err := setup.Apply(cfg, ws, "fast", "gpt-5.6-luna")
	if err != nil {
		t.Fatal(err)
	}
	if !got.SetupComplete || got.Mode != config.ModeFast {
		t.Fatalf("%+v", got)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.SetupComplete {
		t.Fatal("expected saved setup_complete")
	}
	if config.NeedsSetup() {
		t.Fatal("setup should be done")
	}
}

func TestSnapshotListsOnlySelectedProvider(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	t.Setenv("PICOGENT_CODEX_HOME", t.TempDir())
	for _, tc := range []struct {
		provider   config.Provider
		cli, login string
	}{
		{config.ProviderCodex, "codex-cli", "codex"},
		{config.ProviderQuadCode, "claude-cli", "claude"},
		{config.ProviderOpenCode, "opencode-cli", "opencode"},
		{config.ProviderAntigravity, "agy-cli", "antigravity"},
	} {
		st := setup.Snapshot(config.Default(), tc.provider)
		ids := map[string]bool{}
		for _, c := range st.Components {
			ids[c.ID] = true
		}
		if len(st.Components) != 3 || !ids["home"] || !ids["git"] || !ids[tc.cli] {
			t.Fatalf("%s components %+v", tc.provider, st.Components)
		}
		if len(st.Logins) != 1 || st.Logins[0].ID != tc.login {
			t.Fatalf("%s logins %+v", tc.provider, st.Logins)
		}
		for _, option := range st.ModelOptions {
			if option.Value == "" {
				t.Fatalf("%s empty model option", tc.provider)
			}
		}
	}
}

func TestApplyRejectsMissingFolder(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	_, err := setup.Apply(config.Default(), filepath.Join(t.TempDir(), "nope"), "safe", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestInstallCoresCreatesHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)
	t.Setenv("PICOGENT_CODEX_HOME", t.TempDir())
	t.Setenv("PICOGENT_SETUP_SKIP_CLIS", "1")
	log, err := setup.InstallCores(config.ProviderCodex)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "config.yaml")); err != nil {
		t.Fatal(err)
	}
	if log == "" {
		t.Fatal("expected log")
	}
}

func TestInstallCoresOnlyTouchesSelectedCLI(t *testing.T) {
	t.Setenv("PICOGENT_SETUP_SKIP_CLIS", "1")
	for _, tc := range []struct {
		provider config.Provider
		name     string
	}{
		{config.ProviderCodex, "codex cli"},
		{config.ProviderQuadCode, "claude cli"},
		{config.ProviderOpenCode, "opencode cli"},
		{config.ProviderAntigravity, "agy cli"},
	} {
		t.Run(string(tc.provider), func(t *testing.T) {
			t.Setenv("PICOGENT_HOME", t.TempDir())
			log, err := setup.InstallCores(tc.provider)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(log, tc.name) {
				t.Fatalf("missing chosen CLI in log: %s", log)
			}
			for _, other := range []string{"codex cli", "claude cli", "opencode cli", "agy cli"} {
				if other != tc.name && strings.Contains(log, other) {
					t.Fatalf("unselected CLI %q in log: %s", other, log)
				}
			}
		})
	}
}
