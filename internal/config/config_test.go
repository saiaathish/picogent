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
