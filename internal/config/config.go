package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Mode string

const (
	ModeSafe Mode = "safe"
	ModeFast Mode = "fast"
)

type Provider string

const (
	ProviderOpenAI Provider = "openai"
	ProviderOllama Provider = "ollama"
)

type Config struct {
	Workspace      string   `yaml:"workspace"`
	Mode           Mode     `yaml:"mode"`
	Provider       Provider `yaml:"provider"`
	BaseURL        string   `yaml:"base_url"`
	APIKey         string   `yaml:"api_key"`
	Model          string   `yaml:"model"`
	OllamaURL      string   `yaml:"ollama_url"`
	MaxToolRounds  int      `yaml:"max_tool_rounds"`
	LLMTimeoutSec  int      `yaml:"llm_timeout_sec"`
	BashTimeoutSec int      `yaml:"bash_timeout_sec"`
}

func Default() Config {
	return Config{
		Workspace:      ".",
		Mode:           ModeSafe,
		Provider:       ProviderOpenAI,
		BaseURL:        "https://api.openai.com/v1",
		Model:          "gpt-4.1-mini",
		OllamaURL:      "http://127.0.0.1:11434",
		MaxToolRounds:  25,
		LLMTimeoutSec:  120,
		BashTimeoutSec: 60,
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
	if c.Provider == ProviderOllama {
		return nil
	}
	if c.APIKeyResolved() != "" {
		return nil
	}
	return fmt.Errorf("Problem: no API key.\nCause:   api_key, PICOGENT_API_KEY, and OPENAI_API_KEY are all empty.\nFix:     picogent init, then export PICOGENT_API_KEY=sk-...  Or: picogent init --ollama")
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
			return overlayEnv(overlayProject(cfg)), nil
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
		cfg.LLMTimeoutSec = 120
	}
	if cfg.BashTimeoutSec <= 0 {
		cfg.BashTimeoutSec = 60
	}
	return overlayEnv(overlayProject(cfg)), nil
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
