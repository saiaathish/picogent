package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/claudeauth"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/evolve"
	"github.com/saiaathish/picogent/internal/extensions"
	"github.com/saiaathish/picogent/internal/goal"
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
	syncMemory(a, cfg.Workspace)
	syncGoal(a, cfg.Workspace)
	wireRuntime(a)
	return a, nil
}

func syncMemory(a *agent.Agent, workspace string) {
	if a == nil {
		return
	}
	store, err := evolve.Load(workspace)
	if err != nil {
		return
	}
	a.Memory = store
}

// RefreshMemory reloads learned habits/playbooks into the live agent.
func RefreshMemory(a *agent.Agent, workspace string) {
	syncMemory(a, workspace)
}

func syncGoal(a *agent.Agent, workspace string) {
	if a == nil {
		return
	}
	g, _ := goal.Load(workspace)
	a.Goal = g
}

// RoutePersist saves the last routing decision into config (best-effort).
func RoutePersist(cfg *config.Config, dec llm.RouteDecision) {
	cfg.Router.LastTier = string(dec.Tier)
	cfg.Router.LastModel = dec.Model
	cfg.Router.LastReason = dec.Reason
	cfg.Router.LastReasoning = string(dec.Reasoning)
	cfg.Router.LastTaskKind = string(dec.TaskKind)
	cfg.Router.LastRouteMode = string(dec.RouteMode)
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

	r := llm.NewRouter(backend, advisor, eco, cfg.FableAllowed(), nil)
	r.Enabled = cfg.Router.Enabled || cfg.AutoRouter()
	return r, nil
}

func newBackend(cfg config.Config) (llm.Client, error) {
	timeout := time.Duration(cfg.LLMTimeoutSec) * time.Second
	switch cfg.Provider {
	case config.ProviderCodex:
		return llm.NewCodex(cfg.BackendModel()), nil
	case config.ProviderQuadCode:
		if key := cfg.AnthropicKeyResolved(); key != "" {
			return llm.NewAnthropic(key, cfg.BackendModel(), timeout), nil
		}
		if claudeauth.LoggedIn() {
			return llm.NewClaudeCode(cfg.BackendModel(), timeout), nil
		}
		return nil, fmt.Errorf("Claude Code is not logged in (run `claude auth login` or picogent setup), and no Anthropic API key is set")
	case config.ProviderOllama, config.ProviderOpenAI:
		return llm.NewOpenAI(cfg.ChatBaseURL(), cfg.APIKeyResolved(), cfg.Model, timeout), nil
	default:
		return nil, fmt.Errorf("unknown provider %q (codex, claude-code, openai, ollama)", cfg.Provider)
	}
}
