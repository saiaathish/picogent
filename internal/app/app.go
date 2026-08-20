package app

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/tools"
)

func Load(dir string) (config.Config, *agent.Agent, error) {
	cfg, err := config.Load()
	if err != nil {
		return cfg, nil, err
	}
	if dir == "" {
		dir = cfg.Workspace
	}
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return cfg, nil, err
	}
	cfg.Workspace = abs
	a, err := Build(cfg)
	return cfg, a, err
}

func Build(cfg config.Config) (*agent.Agent, error) {
	client, err := NewClient(cfg)
	if err != nil {
		return nil, err
	}
	reg := tools.NewRegistry(tools.Context{
		Workspace:   cfg.Workspace,
		BashTimeout: time.Duration(cfg.BashTimeoutSec) * time.Second,
	})
	gate := perm.New(cfg.Mode, cfg.Workspace, nil)
	return agent.New(cfg, client, reg, gate), nil
}

func NewClient(cfg config.Config) (llm.Client, error) {
	switch cfg.Provider {
	case config.ProviderCodex:
		return llm.NewCodex(cfg.Model), nil
	case config.ProviderOllama, config.ProviderOpenAI:
		return llm.NewOpenAI(cfg.ChatBaseURL(), cfg.APIKeyResolved(), cfg.Model, time.Duration(cfg.LLMTimeoutSec)*time.Second), nil
	default:
		return nil, fmt.Errorf("unknown provider %q (codex, openai, ollama)", cfg.Provider)
	}
}
