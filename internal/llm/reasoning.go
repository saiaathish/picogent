package llm

import "strings"

// ReasoningLevel is the scalar reasoning-effort scale shared by Codex and Claude Code.
// Codex maps this to reasoning.effort; Anthropic maps to output_config.effort.
type ReasoningLevel string

const (
	ReasonNone    ReasoningLevel = "none"
	ReasonMinimal ReasoningLevel = "minimal"
	ReasonLow     ReasoningLevel = "low"
	ReasonMedium  ReasoningLevel = "medium"
	ReasonHigh    ReasoningLevel = "high"
	ReasonXHigh   ReasoningLevel = "xhigh"
	ReasonMax     ReasoningLevel = "max"
)

// TaskKind is what the agent is doing in this LLM round.
type TaskKind string

const (
	TaskOrchestrate TaskKind = "orchestrate" // plan, decompose, route (Sol / High)
	TaskImplement   TaskKind = "implement"   // write code, apply fixes
	TaskExplore     TaskKind = "explore"     // read/search only
	TaskReview      TaskKind = "review"      // audit / verify
	TaskSimple      TaskKind = "simple"      // trivial edits
)

// RouteMode follows sol-advisor selective routing semantics.
type RouteMode string

const (
	RouteSolo     RouteMode = "solo"     // one model handles everything (default)
	RouteDelegate RouteMode = "delegate" // lighter tier for bounded sub-work
	RouteAudit    RouteMode = "audit"    // fresh review pass
	RouteFull     RouteMode = "full"     // delegate + review (explicit high-risk)
)

// ReasoningProfile lists supported effort levels for a model tier.
type ReasoningProfile struct {
	Supported []ReasoningLevel `json:"supported"`
	Default   ReasoningLevel   `json:"default"`
}

// ReasoningRank returns the scalar order of a reasoning level (higher = more effort).
func ReasoningRank(l ReasoningLevel) int {
	return reasoningRank(l)
}

func reasoningRank(l ReasoningLevel) int {
	switch l {
	case ReasonNone:
		return 0
	case ReasonMinimal:
		return 1
	case ReasonLow:
		return 2
	case ReasonMedium:
		return 3
	case ReasonHigh:
		return 4
	case ReasonXHigh:
		return 5
	case ReasonMax:
		return 6
	default:
		return 3
	}
}

func clampReasoning(level ReasoningLevel, profile ReasoningProfile) ReasoningLevel {
	if level == "" {
		level = profile.Default
	}
	if len(profile.Supported) == 0 {
		return level
	}
	want := reasoningRank(level)
	best := profile.Supported[0]
	bestRank := -1
	for _, s := range profile.Supported {
		r := reasoningRank(s)
		if r == want {
			return s
		}
		if r <= want && r > bestRank {
			best = s
			bestRank = r
		}
	}
	if bestRank >= 0 {
		return best
	}
	return profile.Supported[len(profile.Supported)-1]
}

// ProfileFor returns the reasoning profile for an ecosystem + tier pair.
func ProfileFor(eco Ecosystem, tier Tier) ReasoningProfile {
	switch eco {
	case EcoQuadCode:
		return quadReasoningProfile(tier)
	default:
		return codexReasoningProfile(tier)
	}
}

func codexReasoningProfile(tier Tier) ReasoningProfile {
	switch tier {
	case TierLight:
		return ReasoningProfile{
			Supported: []ReasoningLevel{ReasonNone, ReasonMinimal, ReasonLow, ReasonMedium, ReasonHigh, ReasonMax},
			Default:   ReasonLow,
		}
	case TierStandard:
		return ReasoningProfile{
			Supported: []ReasoningLevel{ReasonLow, ReasonMedium, ReasonHigh, ReasonXHigh},
			Default:   ReasonMedium,
		}
	case TierHeavy, TierPremium:
		return ReasoningProfile{
			Supported: []ReasoningLevel{ReasonMedium, ReasonHigh, ReasonXHigh, ReasonMax},
			Default:   ReasonHigh,
		}
	default:
		return ReasoningProfile{Supported: []ReasoningLevel{ReasonMedium}, Default: ReasonMedium}
	}
}

func quadReasoningProfile(tier Tier) ReasoningProfile {
	switch tier {
	case TierLight:
		return ReasoningProfile{
			Supported: []ReasoningLevel{ReasonLow, ReasonMedium},
			Default:   ReasonLow,
		}
	case TierStandard:
		return ReasoningProfile{
			Supported: []ReasoningLevel{ReasonLow, ReasonMedium, ReasonHigh},
			Default:   ReasonHigh,
		}
	case TierHeavy:
		return ReasoningProfile{
			Supported: []ReasoningLevel{ReasonLow, ReasonMedium, ReasonHigh, ReasonXHigh, ReasonMax},
			Default:   ReasonHigh,
		}
	case TierPremium:
		return ReasoningProfile{
			Supported: []ReasoningLevel{ReasonHigh, ReasonXHigh, ReasonMax},
			Default:   ReasonMax,
		}
	default:
		return ReasoningProfile{Supported: []ReasoningLevel{ReasonHigh}, Default: ReasonHigh}
	}
}

// ReasoningLabel formats effort for UI (e.g. "Sol / High", "Luna / Max").
func ReasoningLabel(modelLabel string, level ReasoningLevel) string {
	if modelLabel == "" {
		return string(level)
	}
	if level == "" || level == ReasonNone {
		return modelLabel
	}
	return modelLabel + " / " + capitalize(string(level))
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// ReasoningScaleEntry documents one tier's reasoning profile for the UI/API.
type ReasoningScaleEntry struct {
	Tier      Tier             `json:"tier"`
	Display   string           `json:"display"`
	Supported []ReasoningLevel `json:"supported"`
	Default   ReasoningLevel   `json:"default"`
}

// ReasoningScaleFor returns the scalar reasoning scale per tier for an ecosystem.
func ReasoningScaleFor(eco Ecosystem) []ReasoningScaleEntry {
	var tiers []Tier
	switch eco {
	case EcoQuadCode:
		tiers = []Tier{TierLight, TierStandard, TierHeavy, TierPremium}
	default:
		tiers = []Tier{TierLight, TierStandard, TierHeavy}
	}
	out := make([]ReasoningScaleEntry, 0, len(tiers))
	for _, t := range tiers {
		p := ProfileFor(eco, t)
		out = append(out, ReasoningScaleEntry{
			Tier:      t,
			Display:   TierLabel(eco, t),
			Supported: p.Supported,
			Default:   p.Default,
		})
	}
	return out
}
