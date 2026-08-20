package setup

import (
	"github.com/saiaathish/picogent/internal/agyauth"
	"github.com/saiaathish/picogent/internal/claudeauth"
	"github.com/saiaathish/picogent/internal/codexauth"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/opencodeauth"
)

// AuthPrompt is the in-app login CTA for the current provider.
type AuthPrompt struct {
	Needed  bool   `json:"needed"`
	Target  string `json:"target,omitempty"`  // codex | claude | opencode | antigravity | settings
	Label   string `json:"label,omitempty"`
	Button  string `json:"button,omitempty"`
	Detail  string `json:"detail,omitempty"`
	Browser bool   `json:"browser,omitempty"` // true → OAuth URL in browser
}

// ProviderAuthPrompt returns a login widget payload when the active provider needs auth.
func ProviderAuthPrompt(cfg config.Config) AuthPrompt {
	if cfg.MissingAuth() == nil {
		return AuthPrompt{}
	}
	switch cfg.Provider {
	case config.ProviderCodex:
		return AuthPrompt{
			Needed:  true,
			Target:  "codex",
			Label:   "ChatGPT Codex",
			Button:  "Log in to Codex",
			Detail:  "Uses your ChatGPT subscription. Tap once — we’ll open the login page.",
			Browser: true,
		}
	case config.ProviderQuadCode:
		if cfg.AnthropicKeyResolved() != "" {
			return AuthPrompt{}
		}
		return AuthPrompt{
			Needed: true,
			Target: "claude",
			Label:  "Claude Code",
			Button: "Log in to Claude",
			Detail: "We’ll open a terminal and run Claude login for you. Or paste an Anthropic API key in Settings.",
		}
	case config.ProviderOpenCode:
		return AuthPrompt{
			Needed: true,
			Target: "opencode",
			Label:  "OpenCode (Zen / Go)",
			Button: "Log in to OpenCode",
			Detail: "We’ll open a terminal and run OpenCode login. Pick Zen and/or Go when asked.",
		}
	case config.ProviderAntigravity:
		return AuthPrompt{
			Needed: true,
			Target: "antigravity",
			Label:  "Antigravity",
			Button: "Log in to Antigravity",
			Detail: "We’ll open Antigravity so you can sign in with Google. Or set a Gemini API key in Settings.",
		}
	case config.ProviderOllama:
		return AuthPrompt{
			Needed: true,
			Target: "settings",
			Label:  "Ollama",
			Button: "Open Settings",
			Detail: "Start Ollama on this Mac, then pick an Ollama model in Settings.",
		}
	default:
		return AuthPrompt{
			Needed: true,
			Target: "settings",
			Label:  "API key",
			Button: "Open Settings",
			Detail: "Add an API key in Settings, or switch to Codex / Claude / OpenCode / Antigravity.",
		}
	}
}

// ProviderConnected reports whether the given provider has usable auth.
func ProviderConnected(p config.Provider) bool {
	switch p {
	case config.ProviderCodex:
		return codexauth.LoggedIn()
	case config.ProviderQuadCode:
		return claudeauth.LoggedIn()
	case config.ProviderOpenCode:
		return opencodeauth.LoggedIn()
	case config.ProviderAntigravity:
		return agyauth.LoggedIn()
	case config.ProviderOllama:
		return true
	default:
		return false
	}
}
