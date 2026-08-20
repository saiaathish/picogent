package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/extensions"
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
	gate.SetAlwaysAllowed(cfg.Extensions.AlwaysAllowTools)
	a := agent.New(cfg, client, reg, gate)
	a.ProjectRules = projectctx.Load(cfg.Workspace)
	skills := extensions.LoadDeveloperExtensions(cfg.Extensions.InstalledSkills)
	a.SkillRules = extensions.SkillsPrompt(skills)
	return a, nil
}

// RoutePersist saves the last routing decision into config (best-effort).
func RoutePersist(cfg *config.Config, dec llm.RouteDecision) {
	cfg.Router.LastTier = string(dec.Tier)
	cfg.Router.LastModel = dec.Model
	cfg.Router.LastReason = dec.Reason
	cfg.Model = dec.Model
	_ = config.Save(*cfg)
}

func NewClient(cfg config.Config) (llm.Client, error) {
	backend, err := newBackend(cfg)
	if err != nil {
		return nil, err
	}
	if !cfg.AutoRouter() {
		return backend, nil
	}
	if cfg.Provider != config.ProviderCodex && cfg.Provider != config.ProviderQuadCode {
		return backend, nil
	}

	cat := llm.InitCatalog(false)
	eco := llm.EcosystemForProvider(string(cfg.Provider))

	var advisorBackend llm.Client
	if cfg.Router.UseLLMAdvisor {
		advisorBackend = backend
	}
	advisorModel := cfg.Router.AdvisorModel
	if advisorModel == "" {
		if m, ok := cat.ModelForTier(eco, llm.TierLight, false); ok {
			advisorModel = m.ID
		}
	}
	advisor := llm.NewAdvisor(cat, advisorBackend, advisorModel)
	if !cfg.Router.UseLLMAdvisor {
		advisor.UseLLMAdvisor = false
	}

	return llm.NewRouter(backend, advisor, eco, cfg.FableAllowed(), nil), nil
}

func newBackend(cfg config.Config) (llm.Client, error) {
	timeout := time.Duration(cfg.LLMTimeoutSec) * time.Second
	switch cfg.Provider {
	case config.ProviderCodex:
		return llm.NewCodex(cfg.BackendModel()), nil
	case config.ProviderQuadCode:
		key := cfg.AnthropicKeyResolved()
		if key == "" {
			return nil, fmt.Errorf("Claude Code requires an Anthropic API key (Settings → API key or ANTHROPIC_API_KEY)")
		}
		return llm.NewAnthropic(key, cfg.BackendModel(), timeout), nil
	case config.ProviderOllama, config.ProviderOpenAI:
		return llm.NewOpenAI(cfg.ChatBaseURL(), cfg.APIKeyResolved(), cfg.Model, timeout), nil
	default:
		return nil, fmt.Errorf("unknown provider %q (codex, claude-code, openai, ollama)", cfg.Provider)
	}
}
