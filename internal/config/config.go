package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/saiaathish/picogent/internal/codexauth"
	"gopkg.in/yaml.v3"
)

type Mode string

const (
	ModeSafe Mode = "safe"
	ModeFast Mode = "fast"
)

type Provider string

const (
	ProviderCodex    Provider = "codex"
	ProviderQuadCode Provider = "quadcode"
	ProviderOpenAI   Provider = "openai"
	ProviderOllama   Provider = "ollama"
)

// ModelAuto enables the auto model router (default for new users).
const ModelAuto = "auto"

// RouterConfig controls automatic model tier selection (Codex Luna/Terra/Sol or Quad Code Haiku/Sonnet/Opus).
type RouterConfig struct {
	Enabled         bool   `yaml:"enabled"`
	UseLLMAdvisor   bool   `yaml:"use_llm_advisor"`
	AllowFable      bool   `yaml:"allow_fable"`
	FableConfirmed  bool   `yaml:"fable_confirmed"`
	AdvisorModel    string `yaml:"advisor_model"`
	LastTier        string `yaml:"last_tier,omitempty"`
	LastModel       string `yaml:"last_model,omitempty"`
	LastReason      string `yaml:"last_reason,omitempty"`
}

// ExtensionsConfig tracks installed extensions and permission preferences.
type ExtensionsConfig struct {
	AlwaysAllowTools []string `yaml:"always_allow_tools"`
	Dismissed        []string `yaml:"dismissed"`
	InstalledSkills  []string `yaml:"installed_skills"`
	InstalledPlugins []string `yaml:"installed_plugins"`
	EssentialPlugins []string `yaml:"essential_plugins"`
	ActiveTransient  []string `yaml:"active_transient"`
}

type Config struct {
	Workspace      string           `yaml:"workspace"`
	Mode           Mode             `yaml:"mode"`
	Provider       Provider         `yaml:"provider"`
	BaseURL        string           `yaml:"base_url"`
	APIKey         string           `yaml:"api_key"`
	AnthropicKey   string           `yaml:"anthropic_api_key"`
	Model          string           `yaml:"model"`
	OllamaURL      string           `yaml:"ollama_url"`
	MaxToolRounds  int              `yaml:"max_tool_rounds"`
	LLMTimeoutSec  int              `yaml:"llm_timeout_sec"`
	BashTimeoutSec int              `yaml:"bash_timeout_sec"`
	SetupComplete  bool             `yaml:"setup_complete"`
	Router         RouterConfig     `yaml:"router"`
	Extensions     ExtensionsConfig `yaml:"extensions"`
}

func Default() Config {
	return Config{
		Workspace:      ".",
		Mode:           ModeSafe,
		Provider:       ProviderCodex,
		BaseURL:        "https://api.openai.com/v1",
		Model:          ModelAuto,
		OllamaURL:      "http://127.0.0.1:11434",
		MaxToolRounds:  25,
		LLMTimeoutSec:  180,
		BashTimeoutSec: 60,
		Router: RouterConfig{
			Enabled:       true,
			UseLLMAdvisor: true,
		},
	}
}

func Dir() (string, error) {
	if v := os.Getenv("PICOGENT_HOME"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".picogent"), nil
}

func (c Config) MissingAuth() error {
	switch c.Provider {
	case ProviderOllama:
		return nil
	case ProviderCodex:
		if codexauth.LoggedIn() {
			return nil
		}
		return fmt.Errorf("Problem: Codex is not logged in.\nCause:   no tokens in ~/.codex/auth.json.\nFix:     picogent login   (uses your ChatGPT Codex subscription)")
	case ProviderQuadCode:
		if c.AnthropicKeyResolved() != "" {
			return nil
		}
		return fmt.Errorf("Problem: Claude Code needs an Anthropic API key.\nCause:   anthropic_api_key / ANTHROPIC_API_KEY are empty.\nFix:     add your key in Settings or export ANTHROPIC_API_KEY=sk-ant-...")
	default:
		if c.APIKeyResolved() != "" {
			return nil
		}
		if codexauth.LoggedIn() {
			return fmt.Errorf("Problem: no API key for provider %s.\nCause:   api_key / PICOGENT_API_KEY / OPENAI_API_KEY are empty.\nFix:     use Codex (the default) or export PICOGENT_API_KEY=sk-...", c.Provider)
		}
		return fmt.Errorf("Problem: no API key.\nCause:   api_key, PICOGENT_API_KEY, and OPENAI_API_KEY are all empty.\nFix:     picogent login  (Codex)  or  export PICOGENT_API_KEY=sk-...  or  picogent init --ollama")
	}
}

func NeedsSetup() bool {
	path, err := Path()
	if err != nil {
		return true
	}
	if _, err := os.Stat(path); err != nil {
		return true
	}
	cfg, err := Load()
	if err != nil {
		return true
	}
	return !cfg.SetupComplete
}

func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

func Load() (Config, error) {
	cfg := Default()
	path, err := Path()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return overlayEnv(preferCodex(normalizeModel(overlayProject(cfg)))), nil
		}
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.MaxToolRounds <= 0 {
		cfg.MaxToolRounds = 25
	}
	if cfg.LLMTimeoutSec <= 0 {
		cfg.LLMTimeoutSec = 180
	}
	if cfg.BashTimeoutSec <= 0 {
		cfg.BashTimeoutSec = 60
	}
	if cfg.Provider == "" {
		cfg.Provider = ProviderCodex
	}
	return overlayEnv(preferCodex(normalizeModel(overlayProject(cfg)))), nil
}

func Save(cfg Config) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	path, err := Path()
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (c Config) APIKeyResolved() string {
	if v := os.Getenv("PICOGENT_API_KEY"); v != "" {
		return v
	}
	if c.APIKey != "" {
		return c.APIKey
	}
	return os.Getenv("OPENAI_API_KEY")
}

func (c Config) AnthropicKeyResolved() string {
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		return v
	}
	if c.AnthropicKey != "" {
		return c.AnthropicKey
	}
	return os.Getenv("PICOGENT_ANTHROPIC_API_KEY")
}

func (c Config) RouterEcosystem() string {
	switch c.Provider {
	case ProviderQuadCode:
		return "quadcode"
	default:
		return "codex"
	}
}

func (c Config) FableAllowed() bool {
	return c.Router.AllowFable && c.Router.FableConfirmed && c.AnthropicKeyResolved() != ""
}

// AutoRouter reports whether the auto model router should run.
func (c Config) AutoRouter() bool {
	if c.Model == ModelAuto {
		return true
	}
	return c.Router.Enabled && c.Model == ""
}

// DisplayModel is what the UI should show for the current model setting.
func (c Config) DisplayModel() string {
	if c.AutoRouter() {
		return ModelAuto
	}
	return c.Model
}

// BackendModel returns the fixed model name for non-router backends.
func (c Config) BackendModel() string {
	if c.Model != "" && c.Model != ModelAuto {
		return c.Model
	}
	switch c.Provider {
	case ProviderQuadCode:
		return "claude-sonnet-5"
	default:
		return codexauth.DefaultModel()
	}
}

func normalizeModel(cfg Config) Config {
	if cfg.Model == "" {
		cfg.Model = ModelAuto
	}
	if cfg.Model == ModelAuto {
		cfg.Router.Enabled = true
	}
	return cfg
}

func (c Config) ChatBaseURL() string {
	if c.Provider == ProviderOllama {
		base := c.OllamaURL
		if base == "" {
			base = "http://127.0.0.1:11434"
		}
		return trimSlash(base) + "/v1"
	}
	if c.BaseURL != "" {
		return trimSlash(c.BaseURL)
	}
	return "https://api.openai.com/v1"
}

func preferCodex(cfg Config) Config {
	if cfg.Provider == ProviderOpenAI && cfg.APIKeyResolved() == "" && codexauth.LoggedIn() {
		cfg.Provider = ProviderCodex
		if cfg.Model == "" || cfg.Model == "gpt-4.1-mini" {
			cfg.Model = codexauth.DefaultModel()
		}
	}
	if cfg.Provider == ProviderCodex && (cfg.Model == "" || cfg.Model == "gpt-4.1-mini") {
		cfg.Model = ModelAuto
	}
	return cfg
}

func overlayEnv(cfg Config) Config {
	if v := os.Getenv("PICOGENT_MODEL"); v != "" {
		cfg.Model = v
	}
	if v := os.Getenv("PICOGENT_BASE_URL"); v != "" {
		cfg.BaseURL = v
		cfg.Provider = ProviderOpenAI
	}
	if v := os.Getenv("PICOGENT_MODE"); v == string(ModeSafe) || v == string(ModeFast) {
		cfg.Mode = Mode(v)
	}
	switch strings.ToLower(os.Getenv("PICOGENT_PROVIDER")) {
	case "codex":
		cfg.Provider = ProviderCodex
	case "quadcode", "claude", "claude-code":
		cfg.Provider = ProviderQuadCode
	case "openai":
		cfg.Provider = ProviderOpenAI
	case "ollama":
		cfg.Provider = ProviderOllama
	}
	if v := os.Getenv("PICOGENT_ROUTER"); v == "0" || v == "false" || v == "off" {
		cfg.Router.Enabled = false
	}
	if v := os.Getenv("PICOGENT_ROUTER"); v == "1" || v == "true" || v == "on" {
		cfg.Router.Enabled = true
	}
	return cfg
}

func overlayProject(cfg Config) Config {
	data, err := os.ReadFile(".picogent.yaml")
	if err != nil {
		return cfg
	}
	var over struct {
		Mode  Mode   `yaml:"mode"`
		Model string `yaml:"model"`
	}
	if err := yaml.Unmarshal(data, &over); err != nil {
		return cfg
	}
	if over.Mode == ModeSafe || over.Mode == ModeFast {
		cfg.Mode = over.Mode
	}
	if over.Model != "" {
		cfg.Model = over.Model
	}
	return cfg
}

func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

func (m Mode) Valid() bool {
	return m == ModeSafe || m == ModeFast
}
