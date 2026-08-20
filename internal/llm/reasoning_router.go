package llm

import "strings"

// DecideReasoning picks reasoning effort for a routed call.
// Inspired by sol-advisor: orchestration→High, bounded work→Max on Luna,
// judgment-heavy→High on Terra, review→High on Sol.
func DecideReasoning(eco Ecosystem, tier Tier, kind TaskKind, toolRound int, escalate bool, score int) ReasoningLevel {
	profile := ProfileFor(eco, tier)
	level := baseReasoningForKind(eco, tier, kind, profile)

	// Vary effort across agent loop rounds — save tokens early, spend late.
	switch {
	case escalate || toolRound >= 8:
		level = bump(level, 2)
	case toolRound >= 5:
		level = bump(level, 1)
	case toolRound <= 1 && kind == TaskExplore:
		level = drop(level, 1)
	case toolRound == 0 && kind == TaskOrchestrate:
		level = bump(level, 0) // keep orchestration baseline
	}

	// Score nudges for ambiguous/complex prompts.
	switch {
	case score >= 9:
		level = bump(level, 2)
	case score >= 7:
		level = bump(level, 1)
	case score <= 2 && kind != TaskOrchestrate:
		level = drop(level, 1)
	}

	return clampReasoning(level, profile)
}

func baseReasoningForKind(eco Ecosystem, tier Tier, kind TaskKind, profile ReasoningProfile) ReasoningLevel {
	switch kind {
	case TaskOrchestrate:
		if tier == TierHeavy || tier == TierPremium {
			return ReasonHigh
		}
		return ReasonMedium
	case TaskReview:
		return ReasonHigh
	case TaskExplore:
		if tier == TierLight {
			return ReasonLow
		}
		return ReasonMedium
	case TaskSimple:
		if tier == TierLight {
			// sol-advisor uses Luna/Max for bounded implementation; we use max only when implementing.
			return ReasonNone
		}
		return ReasonLow
	case TaskImplement:
		if eco == EcoCodex {
			switch tier {
			case TierLight:
				return ReasonMax // bounded routine work — Luna/Max
			case TierStandard:
				return ReasonHigh // judgment-heavy — Terra/High
			case TierHeavy, TierPremium:
				return ReasonHigh
			}
		}
		if tier == TierLight {
			return ReasonMedium
		}
		return ReasonHigh
	default:
		return profile.Default
	}
}

// InferTaskKind guesses the subtask phase from routing hints.
func InferTaskKind(in RouteInput) TaskKind {
	if in.TaskKind != "" {
		return in.TaskKind
	}
	p := strings.ToLower(strings.TrimSpace(in.Prompt))

	if in.TaskMode == "plan" || strings.Contains(p, "architect") || strings.Contains(p, "roadmap") {
		return TaskOrchestrate
	}
	if in.TaskMode == "ask" || in.ReadOnly {
		return TaskExplore
	}
	if in.TaskMode == "debug" {
		return TaskImplement
	}

	// Tool-round phase detection inside agent loops.
	switch {
	case in.ToolRound == 0:
		if scorePromptQuick(p) >= 6 {
			return TaskOrchestrate
		}
		return TaskExplore
	case in.ToolRound <= 2:
		return TaskExplore
	case in.LastToolKind == "write":
		return TaskImplement
	case in.LastToolKind == "read":
		if in.ToolRound >= 6 {
			return TaskReview
		}
		return TaskExplore
	default:
		if in.ToolRound >= 7 {
			return TaskReview
		}
		return TaskImplement
	}
}

// DecideRouteMode picks sol-advisor-style selective routing mode.
func DecideRouteMode(tier Tier, kind TaskKind, toolRound int, score int, escalate bool) RouteMode {
	if escalate || score >= 9 {
		return RouteFull
	}
	if kind == TaskReview || (kind == TaskOrchestrate && toolRound == 0 && score >= 7) {
		if tier == TierHeavy || tier == TierPremium {
			return RouteAudit
		}
	}
	if kind == TaskSimple || kind == TaskExplore {
		if tier == TierLight && toolRound > 0 {
			return RouteDelegate
		}
	}
	if kind == TaskImplement && tier != TierHeavy && toolRound >= 3 && score >= 5 {
		return RouteDelegate
	}
	return RouteSolo
}

// AdjustTierForRoute may downgrade tier for delegate mode to save usage.
func AdjustTierForRoute(eco Ecosystem, tier Tier, mode RouteMode, kind TaskKind, allowPremium bool) Tier {
	if mode != RouteDelegate {
		return tier
	}
	switch kind {
	case TaskSimple, TaskExplore:
		if tierRank(tier) > tierRank(TierLight) {
			return TierLight
		}
	case TaskImplement:
		if tier == TierHeavy {
			return TierStandard
		}
		if tier == TierPremium {
			if allowPremium {
				return TierHeavy
			}
			return TierStandard
		}
	}
	return tier
}

func bump(level ReasoningLevel, steps int) ReasoningLevel {
	order := []ReasoningLevel{ReasonNone, ReasonMinimal, ReasonLow, ReasonMedium, ReasonHigh, ReasonXHigh, ReasonMax}
	idx := 0
	for i, l := range order {
		if l == level {
			idx = i
			break
		}
	}
	idx += steps
	if idx >= len(order) {
		idx = len(order) - 1
	}
	return order[idx]
}

func drop(level ReasoningLevel, steps int) ReasoningLevel {
	order := []ReasoningLevel{ReasonNone, ReasonMinimal, ReasonLow, ReasonMedium, ReasonHigh, ReasonXHigh, ReasonMax}
	idx := 0
	for i, l := range order {
		if l == level {
			idx = i
			break
		}
	}
	idx -= steps
	if idx < 0 {
		idx = 0
	}
	return order[idx]
}

func scorePromptQuick(p string) int {
	if p == "" {
		return 3
	}
	score := 0
	words := len(strings.Fields(p))
	if words > 25 {
		score += 2
	}
	for _, k := range []string{"architect", "plan", "design", "refactor", "migrate", "complex"} {
		if strings.Contains(p, k) {
			score += 2
		}
	}
	return score
}
