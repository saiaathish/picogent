package llm

import "strings"

// DecideReasoning picks reasoning effort for a routed call.
// Token-first (Cursor auto / RouteLLM style): spend effort only when the
// phase actually needs it. Explore/simple stay at none/low. Flagship high+
// is reserved for orchestration, review, and explicit escalation.
func DecideReasoning(eco Ecosystem, tier Tier, kind TaskKind, toolRound int, escalate bool, score int) ReasoningLevel {
	profile := ProfileFor(eco, tier)
	level := baseReasoningForKind(eco, tier, kind, profile)

	// Vary effort across agent loop rounds — save tokens early, spend late.
	switch {
	case escalate || toolRound >= 10:
		level = bump(level, 2)
	case toolRound >= 7:
		level = bump(level, 1)
	case toolRound <= 2 && (kind == TaskExplore || kind == TaskSimple):
		level = drop(level, 1)
	case toolRound <= 1 && kind == TaskImplement && score <= 4:
		level = drop(level, 1)
	}

	// Score nudges — keep low for routine work; only spike on hard prompts.
	// Orchestration already starts at medium/high; don't auto-jump to xhigh/max.
	switch {
	case score >= 9 && kind != TaskOrchestrate:
		level = bump(level, 1)
	case score <= 3 && kind != TaskOrchestrate && kind != TaskReview:
		level = drop(level, 1)
	}

	return clampReasoning(level, profile)
}

func baseReasoningForKind(eco Ecosystem, tier Tier, kind TaskKind, profile ReasoningProfile) ReasoningLevel {
	switch kind {
	case TaskOrchestrate:
		// Plan once with medium/high — not ultra. Escalation can bump later.
		if tier == TierHeavy || tier == TierPremium {
			return ReasonHigh
		}
		if tier == TierStandard {
			return ReasonMedium
		}
		return ReasonLow
	case TaskReview:
		if tier == TierHeavy || tier == TierPremium {
			return ReasonHigh
		}
		return ReasonMedium
	case TaskExplore:
		// Cheap reads/searches — almost never need deep reasoning.
		if tier == TierLight {
			return ReasonNone
		}
		return ReasonLow
	case TaskSimple:
		return ReasonNone
	case TaskImplement:
		// Bounded implementation: prefer low/medium. Luna/Max was quality-first
		// and burned huge reasoning budgets on "light" work — inverted here.
		switch tier {
		case TierLight:
			return ReasonLow
		case TierStandard:
			return ReasonMedium
		case TierHeavy, TierPremium:
			return ReasonHigh
		}
		return ReasonMedium
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
	if looksSimple(p) {
		return TaskSimple
	}
	if in.TaskMode == "debug" {
		return TaskImplement
	}

	// Tool-round phase detection inside agent loops — stay explore longer.
	switch {
	case in.ToolRound == 0:
		if scorePromptQuick(p) >= 7 {
			return TaskOrchestrate
		}
		if scorePromptQuick(p) <= 2 {
			return TaskSimple
		}
		return TaskExplore
	case in.ToolRound <= 3:
		return TaskExplore
	case in.LastToolKind == "write":
		return TaskImplement
	case in.LastToolKind == "read":
		if in.ToolRound >= 8 {
			return TaskReview
		}
		return TaskExplore
	default:
		if in.ToolRound >= 9 {
			return TaskReview
		}
		return TaskImplement
	}
}

func looksSimple(p string) bool {
	if p == "" {
		return false
	}
	words := len(strings.Fields(p))
	if words > 20 {
		return false
	}
	for _, k := range []string{
		"typo", "lint", "format", "rename", "comment", "docstring",
		"whitespace", "import ", "one-liner", "quick fix", "small fix",
	} {
		if strings.Contains(p, k) {
			return true
		}
	}
	return false
}

// DecideRouteMode picks RouteLLM-style selective routing.
// Prefer delegate (weak model) unless risk/complexity demands solo/audit/full.
func DecideRouteMode(tier Tier, kind TaskKind, toolRound int, score int, escalate bool) RouteMode {
	if escalate || score >= 9 {
		return RouteFull
	}
	if kind == TaskReview && (tier == TierHeavy || tier == TierPremium) && score >= 7 {
		return RouteAudit
	}
	if kind == TaskOrchestrate && toolRound == 0 && score >= 8 && (tier == TierHeavy || tier == TierPremium) {
		return RouteAudit
	}
	// Default bias: delegate bounded phases to the light tier.
	switch kind {
	case TaskSimple, TaskExplore:
		return RouteDelegate
	case TaskImplement:
		if tier != TierHeavy && tier != TierPremium && score <= 6 {
			return RouteDelegate
		}
		if toolRound >= 2 && score <= 5 {
			return RouteDelegate
		}
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
		return TierLight
	case TaskImplement:
		if tierRank(tier) > tierRank(TierStandard) {
			return TierStandard
		}
	}
	return tier
}

func bump(level ReasoningLevel, steps int) ReasoningLevel {
	order := []ReasoningLevel{ReasonNone, ReasonLow, ReasonMedium, ReasonHigh, ReasonXHigh, ReasonMax, ReasonUltra}
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
	order := []ReasoningLevel{ReasonNone, ReasonLow, ReasonMedium, ReasonHigh, ReasonXHigh, ReasonMax, ReasonUltra}
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
