package setup_test

import (
	"os"
	"path/filepath"
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

func TestSnapshotListsCores(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	t.Setenv("PICOGENT_CODEX_HOME", t.TempDir())
	st := setup.Snapshot(config.Default())
	ids := map[string]bool{}
	for _, c := range st.Components {
		ids[c.ID] = true
	}
	for _, want := range []string{"home", "git", "codex-cli", "claude-cli"} {
		if !ids[want] {
			t.Fatalf("missing %s", want)
		}
	}
	if len(st.Logins) < 1 || st.Logins[0].ID != "codex" {
		t.Fatalf("logins %+v", st.Logins)
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
	log, err := setup.InstallCores()
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
