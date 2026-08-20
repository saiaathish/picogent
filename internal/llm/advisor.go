package llm

import (
	"context"
	"strings"
)

// RouteInput is what the advisor uses to pick a tier.
type RouteInput struct {
	Prompt     string
	ToolRound  int
	Escalate   bool
	Ecosystem  Ecosystem
	AllowFable bool
}

// RouteDecision is the advisor's output.
type RouteDecision struct {
	Tier    Tier   `json:"tier"`
	Model   string `json:"model"`
	Label   string `json:"label"`
	Reason  string `json:"reason"`
	Score   int    `json:"score"`
	Advisor string `json:"advisor"` // heuristic | llm
}

// Advisor classifies task complexity.
type Advisor struct {
	Catalog       Catalog
	LLM           Client // optional lightweight model for ambiguous cases
	AdvisorModel  string
	UseLLMAdvisor bool
}

func NewAdvisor(cat Catalog, llm Client, advisorModel string) *Advisor {
	return &Advisor{
		Catalog:       cat,
		LLM:           llm,
		AdvisorModel:  advisorModel,
		UseLLMAdvisor: llm != nil && advisorModel != "",
	}
}

func (a *Advisor) Decide(ctx context.Context, in RouteInput) RouteDecision {
	if in.Escalate || in.ToolRound >= 8 {
		return a.finish(in, TierHeavy, "escalation", "Worker stuck — escalating to heavy tier", 9, "heuristic")
	}
	if in.ToolRound >= 4 {
		score := a.scorePrompt(in.Prompt) + 2
		tier := tierFromScore(score)
		if tierRank(tier) < tierRank(TierStandard) {
			tier = TierStandard
		}
		return a.finish(in, tier, "mid-run", "Multi-step task — using standard tier or above", score, "heuristic")
	}

	score := a.scorePrompt(in.Prompt)
	tier := tierFromScore(score)

	if a.UseLLMAdvisor && ambiguous(score) {
		if llmTier, reason, ok := a.llmClassify(ctx, in); ok {
			return a.finish(in, llmTier, "llm-advisor", reason, score, "llm")
		}
	}

	reason := reasonForScore(score, in.Prompt)
	return a.finish(in, tier, "heuristic", reason, score, "heuristic")
}

func (a *Advisor) finish(in RouteInput, tier Tier, kind, reason string, score int, advisor string) RouteDecision {
	if in.AllowFable && tier == TierPremium {
		if m, ok := a.Catalog.ModelForTier(in.Ecosystem, TierPremium, true); ok {
			return RouteDecision{Tier: TierPremium, Model: m.ID, Label: m.Display, Reason: reason, Score: score, Advisor: advisor}
		}
	}
	if m, ok := a.Catalog.ModelForTier(in.Ecosystem, tier, in.AllowFable); ok {
		return RouteDecision{Tier: tier, Model: m.ID, Label: m.Display, Reason: reason + " (" + kind + ")", Score: score, Advisor: advisor}
	}
	// Last resort.
	m, _ := a.Catalog.ModelForTier(in.Ecosystem, TierStandard, false)
	return RouteDecision{Tier: TierStandard, Model: m.ID, Label: m.Display, Reason: "fallback to standard tier", Score: score, Advisor: advisor}
}

func tierFromScore(score int) Tier {
	switch {
	case score <= 2:
		return TierLight
	case score <= 5:
		return TierStandard
	default:
		return TierHeavy
	}
}

func ambiguous(score int) bool {
	return score >= 4 && score <= 6
}

func (a *Advisor) scorePrompt(prompt string) int {
	p := strings.ToLower(strings.TrimSpace(prompt))
	if p == "" {
		return 3
	}
	score := 0

	words := len(strings.Fields(p))
	switch {
	case words <= 8:
		score += 0
	case words <= 25:
		score += 1
	case words <= 60:
		score += 2
	default:
		score += 3
	}

	light := []string{
		"lint", "format", "typo", "spelling", "comment", "docstring", "readme",
		"test ", "unit test", "fix test", "rename", "whitespace", "import ",
		"what is", "explain this line", "quick", "small fix", "one-liner",
	}
	heavy := []string{
		"architect", "architecture", "design system", "refactor entire", "migrate",
		"plan", "roadmap", "strategy", "complex", "multi-file", "across the codebase",
		"review all", "security audit", "performance audit", "rewrite", "from scratch",
		"debug deeply", "root cause", "race condition", "concurrency",
	}
	premium := []string{"fable", "maximum quality", "best possible", "highest tier"}

	for _, k := range light {
		if strings.Contains(p, k) {
			score -= 2
		}
	}
	for _, k := range heavy {
		if strings.Contains(p, k) {
			score += 3
		}
	}
	for _, k := range premium {
		if strings.Contains(p, k) {
			score += 5
		}
	}

	if strings.Count(p, "```") >= 2 {
		score += 1
	}
	if strings.Count(p, "\n") > 8 {
		score += 1
	}

	if score < 0 {
		score = 0
	}
	if score > 10 {
		score = 10
	}
	return score
}

func reasonForScore(score int, prompt string) string {
	p := strings.ToLower(prompt)
	switch {
	case score <= 2:
		return "Simple task — routing to light tier"
	case score <= 5:
		return "Standard coding task — routing to standard tier"
	default:
		if strings.Contains(p, "architect") || strings.Contains(p, "plan") {
			return "Planning/architecture detected — routing to heavy tier"
		}
		return "Complex task — routing to heavy tier"
	}
}

func (a *Advisor) llmClassify(ctx context.Context, in RouteInput) (Tier, string, bool) {
	if a.LLM == nil {
		return TierStandard, "", false
	}
	sys := `You are a model router advisor. Reply with exactly one word: light, standard, heavy, or premium.
light = lint/docs/tests/typos/simple questions
standard = implementation/refactoring/bug fixes
heavy = architecture/planning/complex multi-file work
premium = only when user explicitly wants maximum quality`
	user := "Classify this coding task:\n" + clipPrompt(in.Prompt, 1200)
	out, err := a.LLM.Chat(ctx, ChatRequest{
		Model: a.AdvisorModel,
		Messages: []Message{
			{Role: "system", Content: sys},
			{Role: "user", Content: user},
		},
	})
	if err != nil {
		return TierStandard, "", false
	}
	word := strings.ToLower(strings.TrimSpace(out.Message.Content))
	word = strings.Trim(word, "`.\"'")
	switch {
	case strings.Contains(word, "premium"):
		if in.AllowFable {
			return TierPremium, "Advisor chose premium tier", true
		}
		return TierHeavy, "Advisor wanted premium but Fable is disabled — using heavy", true
	case strings.Contains(word, "heavy"):
		return TierHeavy, "Advisor chose heavy tier", true
	case strings.Contains(word, "light"):
		return TierLight, "Advisor chose light tier", true
	default:
		return TierStandard, "Advisor chose standard tier", true
	}
}

func clipPrompt(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// LatestUserPrompt extracts the last user message from a chat request.
func LatestUserPrompt(msgs []Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			return strings.TrimSpace(msgs[i].Content)
		}
	}
	return ""
}
