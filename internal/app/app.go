package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/mcpbridge"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/projectctx"
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
	if servers, err := mcpbridge.LoadServers(cfg.Workspace); err != nil {
		return nil, err
	} else if len(servers) > 0 {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		mgr, warns := mcpbridge.ConnectBestEffort(ctx, servers)
		cancel()
		for _, w := range warns {
			fmt.Fprintf(os.Stderr, "picogent: mcp: %s\n", w)
		}
		reg.AttachMCP(mgr)
	}
	gate := perm.New(cfg.Mode, cfg.Workspace, nil)
	a := agent.New(cfg, client, reg, gate)
	a.ProjectRules = projectctx.Load(cfg.Workspace)
	return a, nil
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
