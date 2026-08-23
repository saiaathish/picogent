package llm

import (
	"context"
	"sync"
)

// RouteHook is called when the router picks a model (for UI/logging).
type RouteHook func(dec RouteDecision)

// Router implements Client and auto-selects models by task complexity.
type Router struct {
	Backend    Client
	Advisor    *Advisor
	Ecosystem  Ecosystem
	AllowFable bool
	Enabled    bool
	OnRoute    RouteHook
	mu         sync.Mutex
	last       RouteDecision
	failures   int
	userPrompt string
}

func NewRouter(backend Client, advisor *Advisor, eco Ecosystem, allowFable bool, onRoute RouteHook) *Router {
	return &Router{
		Backend:    backend,
		Advisor:    advisor,
		Ecosystem:  eco,
		AllowFable: allowFable,
		Enabled:    true,
		OnRoute:    onRoute,
	}
}

func (r *Router) LastDecision() RouteDecision {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.last
}

func (r *Router) SetUserPrompt(p string) {
	r.mu.Lock()
	r.userPrompt = p
	r.mu.Unlock()
}

// SetOnRoute swaps the UI hook without racing a route that is about to call
// the previous hook.
func (r *Router) SetOnRoute(hook RouteHook) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.OnRoute = hook
	r.mu.Unlock()
}

func (r *Router) LastRouteHook() RouteHook {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.OnRoute
}

func (r *Router) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	mustRoute := IsAutoModel(req.Model)
	if (!r.Enabled && !mustRoute) || r.Advisor == nil {
		req.Model = r.guardModel(req)
		return r.Backend.Chat(ctx, req)
	}

	prompt := LatestUserPrompt(req.Messages)
	r.mu.Lock()
	if prompt == "" {
		prompt = r.userPrompt
	}
	toolRound := req.ToolRound
	escalate := req.Escalate || r.failures >= 2
	r.mu.Unlock()

	dec := r.Advisor.Decide(ctx, RouteInput{
		Prompt:       prompt,
		ToolRound:    toolRound,
		Escalate:     escalate,
		Ecosystem:    r.Ecosystem,
		AllowFable:   r.AllowFable,
		HasImages:    MessageHasImages(req.Messages),
		HasFiles:     MessageHasFiles(req.Messages),
		TaskMode:     req.TaskMode,
		ReadOnly:     req.ReadOnly,
		LastToolKind: req.LastToolKind,
	})

	req.Model = dec.Model
	if dec.Reasoning != "" {
		req.Reasoning = dec.Reasoning
	}
	if req.Model == "" {
		req.Model = r.guardModel(req)
	}

	r.mu.Lock()
	r.last = dec
	hook := r.OnRoute
	r.mu.Unlock()
	if hook != nil {
		hook(dec)
	}

	out, err := r.Backend.Chat(ctx, req)
	if err != nil {
		r.mu.Lock()
		r.failures++
		r.mu.Unlock()
		return out, err
	}
	r.mu.Lock()
	r.failures = 0
	r.mu.Unlock()
	return out, nil
}

func (r *Router) guardModel(req ChatRequest) string {
	if !IsAutoModel(req.Model) && req.Model != "" {
		return req.Model
	}
	cat := InitCatalog(false)
	prompt := LatestUserPrompt(req.Messages)
	r.mu.Lock()
	if prompt == "" {
		prompt = r.userPrompt
	}
	r.mu.Unlock()
	if id := ResolveRequestModel(cat, r.Ecosystem, r.AllowFable, req, prompt, req.ToolRound, req.Escalate); id != "" {
		return id
	}
	if m, ok := cat.ModelForTier(r.Ecosystem, TierStandard, false); ok {
		return m.ID
	}
	return req.Model
}
