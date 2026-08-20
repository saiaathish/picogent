package llm

import (
	"context"

	"github.com/saiaathish/picogent/internal/config"
)

// IsAutoModel reports whether the model field means "router picks for me".
func IsAutoModel(model string) bool {
	return model == "" || model == config.ModelAuto
}

// ResolveRequestModel maps auto/empty to a concrete model ID using the advisor.
// Used as a last-resort guard so backends never see the literal string "auto".
func ResolveRequestModel(cat Catalog, eco Ecosystem, allowFable bool, req ChatRequest, prompt string, toolRound int, escalate bool) string {
	if !IsAutoModel(req.Model) {
		return req.Model
	}
	a := NewAdvisor(cat, nil, "")
	dec := a.Decide(context.Background(), RouteInput{
		Prompt:       prompt,
		ToolRound:    toolRound,
		Escalate:     escalate,
		Ecosystem:    eco,
		AllowFable:   allowFable,
		HasImages:    MessageHasImages(req.Messages),
		HasFiles:     MessageHasFiles(req.Messages),
		TaskMode:     req.TaskMode,
		ReadOnly:     req.ReadOnly,
		LastToolKind: req.LastToolKind,
	})
	if dec.Model != "" {
		return dec.Model
	}
	if m, ok := cat.ModelForTier(eco, TierStandard, false); ok {
		return m.ID
	}
	return ""
}

func MessageHasImages(msgs []Message) bool {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != "user" {
			continue
		}
		for _, p := range msgs[i].Parts {
			if p.Type == "image" {
				return true
			}
		}
		return false
	}
	return false
}

func MessageHasFiles(msgs []Message) bool {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role != "user" {
			continue
		}
		for _, p := range msgs[i].Parts {
			if p.Type == "file" {
				return true
			}
		}
		return false
	}
	return false
}
