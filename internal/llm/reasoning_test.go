package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
)

func TestReasoningOrchestrationSolHigh(t *testing.T) {
	cat := llm.InitCatalog(false)
	a := llm.NewAdvisor(cat, nil, "")
	dec := a.Decide(context.Background(), llm.RouteInput{
		Prompt:    "Design the architecture for a distributed task router with planning",
		Ecosystem: llm.EcoCodex,
		ToolRound: 0,
	})
	if dec.Tier != llm.TierHeavy {
		t.Fatalf("tier=%s want heavy", dec.Tier)
	}
	if dec.Reasoning != llm.ReasonHigh && dec.Reasoning != llm.ReasonMedium {
		t.Fatalf("reasoning=%s want medium/high for orchestration", dec.Reasoning)
	}
	if dec.TaskKind != llm.TaskOrchestrate {
		t.Fatalf("task=%s want orchestrate", dec.TaskKind)
	}
	if dec.TokenSaveX < 1 {
		t.Fatalf("token_save_x=%v", dec.TokenSaveX)
	}
}

func TestReasoningLunaLowForBoundedImplement(t *testing.T) {
	level := llm.DecideReasoning(llm.EcoCodex, llm.TierLight, llm.TaskImplement, 4, false, 4)
	if level != llm.ReasonLow && level != llm.ReasonNone {
		t.Fatalf("luna implement effort=%s want low/none (token-efficient)", level)
	}
}

func TestReasoningTerraMediumForJudgment(t *testing.T) {
	level := llm.DecideReasoning(llm.EcoCodex, llm.TierStandard, llm.TaskImplement, 4, false, 5)
	if level != llm.ReasonMedium && level != llm.ReasonLow {
		t.Fatalf("terra implement effort=%s want medium/low", level)
	}
}

func TestReasoningEscalatesOnLateRounds(t *testing.T) {
	early := llm.DecideReasoning(llm.EcoCodex, llm.TierStandard, llm.TaskImplement, 2, false, 4)
	late := llm.DecideReasoning(llm.EcoCodex, llm.TierStandard, llm.TaskImplement, 10, true, 4)
	if llm.ReasoningRank(late) <= llm.ReasoningRank(early) {
		t.Fatalf("late=%s should exceed early=%s", late, early)
	}
}

func TestReasoningExploreUsesNoneOnLuna(t *testing.T) {
	level := llm.DecideReasoning(llm.EcoCodex, llm.TierLight, llm.TaskExplore, 1, false, 2)
	if level != llm.ReasonNone && level != llm.ReasonLow {
		t.Fatalf("explore effort=%s want none/low", level)
	}
}

func TestReasoningQuadCodeProfiles(t *testing.T) {
	p := llm.ProfileFor(llm.EcoQuadCode, llm.TierHeavy)
	if len(p.Supported) < 4 {
		t.Fatalf("opus should support 4+ levels, got %v", p.Supported)
	}
	level := llm.DecideReasoning(llm.EcoQuadCode, llm.TierHeavy, llm.TaskOrchestrate, 0, false, 8)
	if level != llm.ReasonHigh && level != llm.ReasonMedium {
		t.Fatalf("opus orchestrate=%s", level)
	}
}

func TestRouteModeDelegateForExplore(t *testing.T) {
	mode := llm.DecideRouteMode(llm.TierStandard, llm.TaskExplore, 2, 3, false)
	if mode != llm.RouteDelegate {
		t.Fatalf("mode=%s want delegate", mode)
	}
}

func TestAdjustTierDelegateDowngrades(t *testing.T) {
	tier := llm.AdjustTierForRoute(llm.EcoCodex, llm.TierHeavy, llm.RouteDelegate, llm.TaskExplore, false)
	if tier != llm.TierLight {
		t.Fatalf("delegate explore tier=%s want light", tier)
	}
}

func TestTokenSaveXExploreIsHuge(t *testing.T) {
	// Luna + none + mid-loop context ≈ 100×+ vs Sol@high plain.
	x := llm.EstimateTokenSaveX(llm.TierLight, llm.ReasonNone, 5)
	if x < 100 {
		t.Fatalf("saveX=%.1f want >=100 for explore/delegate", x)
	}
}

func TestRouterSetsReasoningOnRequest(t *testing.T) {
	scripted := &llm.Scripted{
		Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "ok"}}},
	}
	cat := llm.InitCatalog(false)
	advisor := llm.NewAdvisor(cat, nil, "")
	r := llm.NewRouter(scripted, advisor, llm.EcoCodex, false, nil)
	_, err := r.Chat(context.Background(), llm.ChatRequest{
		Model:     config.ModelAuto,
		TaskMode:  "plan",
		ToolRound: 0,
		Messages:  []llm.Message{{Role: "user", Content: "plan a refactor across the codebase"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(scripted.Calls) != 1 {
		t.Fatalf("calls=%d", len(scripted.Calls))
	}
	if scripted.Calls[0].Reasoning == "" {
		t.Fatal("router did not set reasoning effort")
	}
	dec := r.LastDecision()
	if dec.Reasoning == "" {
		t.Fatal("decision missing reasoning")
	}
	if dec.Label == "" {
		t.Fatal("decision missing label")
	}
	if dec.TokenSaveX < 1 {
		t.Fatalf("missing token save estimate: %v", dec.TokenSaveX)
	}
}

func TestCodexForwardsReasoning(t *testing.T) {
	var captured struct {
		Reasoning *struct {
			Effort string `json:"effort"`
		} `json:"reasoning"`
	}
	stream := strings.Join([]string{
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}",
		"",
	}, "\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(stream))
	}))
	defer srv.Close()

	c := llm.NewCodex("gpt-5.6-terra")
	c.BaseURL = srv.URL
	c.Tokens = codexFakeTok{access: "tok", account: "acct"}
	c.HTTP = srv.Client()

	_, err := c.Chat(context.Background(), llm.ChatRequest{
		Model:     "gpt-5.6-terra",
		Reasoning: llm.ReasonHigh,
		Messages:  []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if captured.Reasoning == nil || captured.Reasoning.Effort != "high" {
		t.Fatalf("reasoning not forwarded: %+v", captured.Reasoning)
	}
}

func TestAnthropicForwardsReasoning(t *testing.T) {
	var captured struct {
		OutputConfig *struct {
			Effort string `json:"effort"`
		} `json:"output_config"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"ok"}]}`))
	}))
	defer srv.Close()

	c := llm.NewAnthropic("sk-test", "claude-sonnet-5", 0)
	c.BaseURL = srv.URL
	c.HTTP = srv.Client()

	_, err := c.Chat(context.Background(), llm.ChatRequest{
		Model:     "claude-sonnet-5",
		Reasoning: llm.ReasonMedium,
		Messages:  []llm.Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if captured.OutputConfig == nil || captured.OutputConfig.Effort != "medium" {
		t.Fatalf("effort not forwarded: %+v", captured.OutputConfig)
	}
}

func TestReasoningScaleForCodex(t *testing.T) {
	scale := llm.ReasoningScaleFor(llm.EcoCodex)
	if len(scale) < 3 {
		t.Fatalf("scale len=%d", len(scale))
	}
	if scale[0].Display != "Luna" {
		t.Fatalf("first=%s", scale[0].Display)
	}
	for _, s := range scale[0].Supported {
		if s == "minimal" {
			t.Fatal("Luna must not include minimal")
		}
	}
	terra := scale[1].Supported
	sol := scale[2].Supported
	hasUltra := func(levels []llm.ReasoningLevel) bool {
		for _, l := range levels {
			if l == llm.ReasonUltra {
				return true
			}
		}
		return false
	}
	if hasUltra(scale[0].Supported) {
		t.Fatal("Luna should not include ultra")
	}
	if !hasUltra(terra) || !hasUltra(sol) {
		t.Fatalf("Terra/Sol must include ultra; terra=%v sol=%v", terra, sol)
	}
	wantOrder := []llm.ReasoningLevel{llm.ReasonNone, llm.ReasonLow, llm.ReasonMedium, llm.ReasonHigh, llm.ReasonXHigh, llm.ReasonMax}
	for i, w := range wantOrder {
		if terra[i] != w {
			t.Fatalf("terra[%d]=%s want %s", i, terra[i], w)
		}
	}
}

func TestCatalogEmbedsReasoningProfiles(t *testing.T) {
	cat := llm.InitCatalog(false)
	m, ok := cat.ModelForTier(llm.EcoCodex, llm.TierLight, false)
	if !ok {
		t.Fatal("missing luna")
	}
	if m.Reasoning == nil || len(m.Reasoning.Supported) == 0 {
		t.Fatal("missing reasoning profile on catalog entry")
	}
}

type codexFakeTok struct{ access, account string }

func (f codexFakeTok) Token(context.Context) (string, string, error) { return f.access, f.account, nil }
func (f codexFakeTok) ForceRefresh(context.Context) error            { return nil }
