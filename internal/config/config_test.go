package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/saiaathish/picogent/internal/config"
)

func TestMissingAuth(t *testing.T) {
	t.Setenv("PICOGENT_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("PICOGENT_CODEX_HOME", t.TempDir())
	cfg := config.Default()
	if err := cfg.MissingAuth(); err == nil {
		t.Fatal("expected missing Codex login")
	}
	cfg.Provider = config.ProviderOllama
	if err := cfg.MissingAuth(); err != nil {
		t.Fatal(err)
	}
	cfg.Provider = config.ProviderOpenAI
	if err := cfg.MissingAuth(); err == nil {
		t.Fatal("expected missing key")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)
	t.Setenv("PICOGENT_CODEX_HOME", t.TempDir())
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Model = "qwen2.5-coder:7b"
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != config.ProviderOllama || got.Model != "qwen2.5-coder:7b" {
		t.Fatalf("%+v", got)
	}
	if _, err := os.Stat(filepath.Join(home, "config.yaml")); err != nil {
		t.Fatal(err)
	}
}

func TestLoadPromotesCodexWhenLoggedIn(t *testing.T) {
	pic := t.TempDir()
	codex := t.TempDir()
	t.Setenv("PICOGENT_HOME", pic)
	t.Setenv("PICOGENT_CODEX_HOME", codex)
	t.Setenv("PICOGENT_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	cfg := config.Default()
	cfg.Provider = config.ProviderOpenAI
	cfg.Model = "gpt-4.1-mini"
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codex, "auth.json"), []byte(`{"tokens":{"access_token":"x","refresh_token":"y"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != config.ProviderCodex {
		t.Fatalf("provider %s", got.Provider)
	}
}

func TestProjectConfigCannotOverrideUserPermissionMode(t *testing.T) {
	for _, tc := range []struct {
		name    string
		user    config.Mode
		project config.Mode
	}{
		{name: "project cannot promote safe to fast", user: config.ModeSafe, project: config.ModeFast},
		{name: "project cannot demote fast to safe", user: config.ModeFast, project: config.ModeSafe},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			workspace := t.TempDir()
			t.Setenv("PICOGENT_HOME", home)
			t.Setenv("PICOGENT_CODEX_HOME", t.TempDir())
			t.Setenv("PICOGENT_MODE", "")
			t.Setenv("PICOGENT_MODEL", "")
			t.Setenv("PICOGENT_BASE_URL", "")
			t.Setenv("PICOGENT_PROVIDER", "")
			t.Setenv("PICOGENT_ROUTER", "")

			cfg := config.Default()
			cfg.Mode = tc.user
			if err := config.Save(cfg); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(workspace, ".picogent.yaml"), []byte("mode: "+string(tc.project)+"\nmodel: project-model\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Chdir(workspace)

			got, err := config.Load()
			if err != nil {
				t.Fatal(err)
			}
			if got.Mode != tc.user {
				t.Fatalf("mode = %q, want user-owned %q", got.Mode, tc.user)
			}
			if got.Model != "project-model" {
				t.Fatalf("project model overlay = %q, want project-model", got.Model)
			}
			if err := config.Save(got); err != nil {
				t.Fatal(err)
			}
			t.Chdir(t.TempDir())
			reloaded, err := config.Load()
			if err != nil {
				t.Fatal(err)
			}
			if reloaded.Mode != tc.user {
				t.Fatalf("saved mode = %q, want user-owned %q", reloaded.Mode, tc.user)
			}
		})
	}
}

func TestFreshProfileIgnoresProjectPermissionMode(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)
	t.Setenv("PICOGENT_CODEX_HOME", t.TempDir())
	t.Setenv("PICOGENT_MODE", "")
	t.Setenv("PICOGENT_MODEL", "")
	t.Chdir(workspace)
	if err := os.WriteFile(filepath.Join(workspace, ".picogent.yaml"), []byte("mode: fast\nmodel: project-model\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != config.ModeSafe {
		t.Fatalf("mode = %q, want default user-owned %q", got.Mode, config.ModeSafe)
	}
	if got.Model != "project-model" {
		t.Fatalf("project model overlay = %q, want project-model", got.Model)
	}
}

func TestEnvironmentModeRemainsExplicitOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)
	t.Setenv("PICOGENT_CODEX_HOME", t.TempDir())
	t.Setenv("PICOGENT_MODE", "fast")
	cfg := config.Default()
	cfg.Mode = config.ModeSafe
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != config.ModeFast {
		t.Fatalf("mode = %q, want explicit environment override %q", got.Mode, config.ModeFast)
	}
}
