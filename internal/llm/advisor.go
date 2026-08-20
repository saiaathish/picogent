package llm

import (
	"context"
	"fmt"
	"strings"
)

// RouteInput is what the advisor uses to pick a tier.
type RouteInput struct {
	Prompt       string
	ToolRound    int
	Escalate     bool
	Ecosystem    Ecosystem
	AllowFable   bool
	HasImages    bool
	HasFiles     bool
	TaskKind     TaskKind // optional; inferred when empty
	TaskMode     string   // agent | ask | plan | debug
	ReadOnly     bool
	LastToolKind string // read | write | shell | other — from prior tool round
}

// RouteDecision is the advisor's output.
type RouteDecision struct {
	Tier      Tier           `json:"tier"`
	Model     string         `json:"model"`
	Label     string         `json:"label"`
	Reason    string         `json:"reason"`
	Score     int            `json:"score"`
	Advisor   string         `json:"advisor"` // heuristic | llm
	Reasoning ReasoningLevel `json:"reasoning"`
	TaskKind  TaskKind       `json:"task_kind"`
	RouteMode RouteMode      `json:"route_mode"`
	// TokenSaveX ≈ how many times fewer effective tokens vs plain Sol/Opus@high.
	TokenSaveX float64 `json:"token_save_x,omitempty"`
	EstTokens  int     `json:"est_tokens,omitempty"`
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
	// RouteLLM weak-preferring cascade: stay on light as long as possible.
	// Only escalate on explicit failure or very late stuck rounds.
	if in.Escalate || in.ToolRound >= 12 {
		return a.finish(in, TierHeavy, "escalation", "Worker stuck — escalating to heavy tier", 9, "heuristic")
	}
	if in.ToolRound >= 8 {
		score := a.scorePrompt(in.Prompt) + 1
		tier := tierFromScore(score)
		if tierRank(tier) < tierRank(TierStandard) {
			tier = TierStandard
		}
		return a.finish(in, tier, "mid-run", "Long loop — bumping toward standard tier", score, "heuristic")
	}

	score := a.scorePrompt(in.Prompt)
	if in.HasImages {
		score += 2
		if score < 4 {
			score = 4 // images/PDFs → at least standard tier (Terra)
		}
	}
	if in.HasFiles && !in.HasImages {
		score += 1
	}
	// Mid-loop explore/implement rounds stay light unless the prompt is hard.
	if in.ToolRound >= 1 && in.ToolRound < 8 && score < 7 {
		score = maxInt(0, score-1)
	}
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
	taskKind := InferTaskKind(in)
	routeMode := DecideRouteMode(tier, taskKind, in.ToolRound, score, in.Escalate)
	tier = AdjustTierForRoute(in.Ecosystem, tier, routeMode, taskKind, in.AllowFable)
	reasoning := DecideReasoning(in.Ecosystem, tier, taskKind, in.ToolRound, in.Escalate, score)

	if in.AllowFable && tier == TierPremium {
		if m, ok := a.Catalog.ModelForTier(in.Ecosystem, TierPremium, true); ok {
			return a.buildDecision(in, TierPremium, m, reason, kind, score, advisor, taskKind, routeMode, reasoning)
		}
	}
	if m, ok := a.Catalog.ModelForTier(in.Ecosystem, tier, in.AllowFable); ok {
		return a.buildDecision(in, tier, m, reason, kind, score, advisor, taskKind, routeMode, reasoning)
	}
	// Last resort.
	m, _ := a.Catalog.ModelForTier(in.Ecosystem, TierStandard, false)
	return a.buildDecision(in, TierStandard, m, "fallback to standard tier", kind, score, advisor, taskKind, routeMode, reasoning)
}

func (a *Advisor) buildDecision(in RouteInput, tier Tier, m ModelEntry, reason, kind string, score int, advisor string, taskKind TaskKind, routeMode RouteMode, reasoning ReasoningLevel) RouteDecision {
	saveX := EstimateTokenSaveX(tier, reasoning, in.ToolRound)
	est := EstimateRoundTokens(tier, reasoning, in.ToolRound)
	fullReason := reason + " (" + kind + ")"
	if routeMode != RouteSolo {
		fullReason += "; route=" + string(routeMode)
	}
	fullReason += "; task=" + string(taskKind) + "; effort=" + string(reasoning)
	fullReason += "; ~" + formatSaveX(saveX) + "x vs plain flagship"
	return RouteDecision{
		Tier:       tier,
		Model:      m.ID,
		Label:      ReasoningLabel(m.Display, reasoning),
		Reason:     fullReason,
		Score:      score,
		Advisor:    advisor,
		Reasoning:  reasoning,
		TaskKind:   taskKind,
		RouteMode:  routeMode,
		TokenSaveX: saveX,
		EstTokens:  est,
	}
}

func formatSaveX(x float64) string {
	if x >= 100 {
		return "100+"
	}
	if x >= 10 {
		return fmt.Sprintf("%.0f", x)
	}
	return fmt.Sprintf("%.1f", x)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// tierFromScore is deliberately biased toward light (RouteLLM weak-preferring).
// Plain Claude Code / Codex defaults to heavy; we require clear complexity signal.
func tierFromScore(score int) Tier {
	switch {
	case score <= 4:
		return TierLight
	case score <= 7:
		return TierStandard
	default:
		return TierHeavy
	}
}

func ambiguous(score int) bool {
	return score >= 5 && score <= 7
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
	vision := []string{
		"this image", "the image", "screenshot", "what's in", "what is in",
		"describe the image", "look at this", "attached image", "see the photo",
	}
	heavy := []string{
		"design system", "refactor entire", "migrate",
		"roadmap", "strategy", "complex", "multi-file", "across the codebase",
		"review all", "security audit", "performance audit", "rewrite", "from scratch",
		"debug deeply", "root cause", "race condition", "concurrency",
	}
	// Architecture/planning — strong signal for heavy tier.
	if strings.Contains(p, "architecture") || strings.Contains(p, "architect") || strings.Contains(p, "system design") {
		score += 5
	}
	if strings.Contains(p, "planning") || strings.HasPrefix(p, "plan ") || strings.HasPrefix(p, "plan a") {
		score += 2
	}
	premium := []string{"fable", "maximum quality", "best possible", "highest tier"}

	for _, k := range light {
		if strings.Contains(p, k) {
			score -= 2
		}
	}
	for _, k := range vision {
		if strings.Contains(p, k) {
			score += 2
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
	case score <= 4:
		return "Simple/routine — routing to light tier (token-efficient)"
	case score <= 7:
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
