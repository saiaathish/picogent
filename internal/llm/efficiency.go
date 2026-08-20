package llm

// Efficiency math inspired by LMSYS RouteLLM (weak-preferring cascade) and
// Cursor-style auto routing: most agent rounds are explore/delegate work that
// does not need flagship + high reasoning.
//
// Plain Codex / Claude Code baseline ≈ TierHeavy × ReasonHigh on every turn
// with unbounded tool-output growth. Picogent multiplies three levers:
//   1) tier (Luna/Haiku vs Sol/Opus)
//   2) reasoning effort (none/low vs high/max)
//   3) context compression (TokenTamer + Headroom soft-budget: 256k ceiling, small working set)
//
// Product of those levers routinely lands in the 50–200× range vs the baseline
// for typical coding sessions (explore-heavy loops). Hard tasks still escalate.

// BaselineTokensPerRound is a planning estimate for Sol/Opus @ high effort
// with a mid-session context (~12k prompt + ~8k reasoning + ~1k visible).
const BaselineTokensPerRound = 21_000

// EffortTokenWeight approximates relative reasoning+completion tokens vs high.
func EffortTokenWeight(level ReasoningLevel) float64 {
	switch level {
	case ReasonNone:
		return 0.04
	case ReasonLow, "minimal":
		return 0.12
	case ReasonMedium:
		return 0.35
	case ReasonHigh:
		return 1.0
	case ReasonXHigh:
		return 2.2
	case ReasonMax:
		return 3.5
	case ReasonUltra:
		return 5.0
	default:
		return 0.35
	}
}

// TierTokenWeight approximates relative total cost/tokens vs heavy tier.
// Prices (Sol $5/$30, Terra $2.50/$15, Luna $1/$6) are folded into a single
// weight so "tokens" here means effective flagship-equivalent tokens.
func TierTokenWeight(tier Tier) float64 {
	switch tier {
	case TierLight:
		return 0.08 // ~12× cheaper than Sol; prefer for explore/delegate
	case TierStandard:
		return 0.35
	case TierHeavy:
		return 1.0
	case TierPremium:
		return 1.4
	default:
		return 0.35
	}
}

// ContextSaveFactor is the residual context size after TokenTamer-style
// compaction across a multi-round agent loop (vs dumping every tool result).
// Early rounds ~0.9; long loops approach ~0.15–0.25.
func ContextSaveFactor(toolRound int) float64 {
	switch {
	case toolRound <= 1:
		return 0.85
	case toolRound <= 3:
		return 0.55
	case toolRound <= 6:
		return 0.30
	default:
		return 0.18
	}
}

// EstimateTokenSaveX returns how many times fewer effective tokens this decision
// uses vs plain flagship@high (Sol/Opus high, no compaction). Floor at 1.
func EstimateTokenSaveX(tier Tier, level ReasoningLevel, toolRound int) float64 {
	w := TierTokenWeight(tier) * EffortTokenWeight(level) * ContextSaveFactor(toolRound)
	if w <= 0 {
		return 1
	}
	x := 1.0 / w
	if x < 1 {
		return 1
	}
	if x > 500 {
		return 500 // sanity cap for UI
	}
	return x
}

// EstimateRoundTokens is a rough absolute token estimate for one routed call.
func EstimateRoundTokens(tier Tier, level ReasoningLevel, toolRound int) int {
	n := float64(BaselineTokensPerRound) * TierTokenWeight(tier) * EffortTokenWeight(level) * ContextSaveFactor(toolRound)
	if n < 200 {
		return 200
	}
	return int(n)
}
