package setup_test

import (
	"testing"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/setup"
)

func TestProviderAuthPromptWhenMissing(t *testing.T) {
	t.Setenv("PICOGENT_CODEX_HOME", t.TempDir())
	t.Setenv("PICOGENT_OPENCODE_HOME", t.TempDir())
	t.Setenv("PICOGENT_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")

	cfg := config.Default()
	cfg.Provider = config.ProviderOpenCode
	p := setup.ProviderAuthPrompt(cfg)
	if !p.Needed || p.Target != "opencode" || p.Button == "" {
		t.Fatalf("%+v", p)
	}

	cfg.Provider = config.ProviderCodex
	p = setup.ProviderAuthPrompt(cfg)
	if !p.Needed || p.Target != "codex" || !p.Browser {
		t.Fatalf("%+v", p)
	}
}
