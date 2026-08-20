package llm

import (
	"context"
	"sync"
)

// RouteHook is called when the router picks a model (for UI/logging).
type RouteHook func(dec RouteDecision)

// Router implements Client and auto-selects models by task complexity.
type Router struct {
	Backend      Client
	Advisor      *Advisor
	Ecosystem    Ecosystem
	AllowFable   bool
	Enabled      bool
	OnRoute      RouteHook
	mu           sync.Mutex
	last         RouteDecision
	failures     int
	userPrompt   string
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

func (r *Router) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	if !r.Enabled || r.Advisor == nil {
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
		Prompt:     prompt,
		ToolRound:  toolRound,
		Escalate:   escalate,
		Ecosystem:  r.Ecosystem,
		AllowFable: r.AllowFable,
	})

	req.Model = dec.Model

	r.mu.Lock()
	r.last = dec
	r.mu.Unlock()
	if r.OnRoute != nil {
		r.OnRoute(dec)
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
