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
	cfg := config.Default()
	if err := cfg.MissingAuth(); err == nil {
		t.Fatal("expected missing key")
	}
	cfg.Provider = config.ProviderOllama
	if err := cfg.MissingAuth(); err != nil {
		t.Fatal(err)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)
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
