package llm_test

import (
	"context"
	"testing"

	"github.com/saiaathish/picogent/internal/llm"
)

func TestAdvisorLightTask(t *testing.T) {
	cat := llm.InitCatalog(false)
	a := llm.NewAdvisor(cat, nil, "")
	dec := a.Decide(context.Background(), llm.RouteInput{
		Prompt:    "fix the typo in README",
		Ecosystem: llm.EcoCodex,
	})
	if dec.Tier != llm.TierLight {
		t.Fatalf("tier=%s want light", dec.Tier)
	}
	if dec.Model == "" {
		t.Fatal("empty model")
	}
}

func TestAdvisorHeavyTask(t *testing.T) {
	cat := llm.InitCatalog(false)
	a := llm.NewAdvisor(cat, nil, "")
	dec := a.Decide(context.Background(), llm.RouteInput{
		Prompt:    "Design the architecture for a distributed task router with planning and multi-file refactor",
		Ecosystem: llm.EcoCodex,
	})
	if dec.Tier != llm.TierHeavy {
		t.Fatalf("tier=%s want heavy", dec.Tier)
	}
}

func TestAdvisorEscalation(t *testing.T) {
	cat := llm.InitCatalog(false)
	a := llm.NewAdvisor(cat, nil, "")
	dec := a.Decide(context.Background(), llm.RouteInput{
		Prompt:    "add a button",
		ToolRound: 9,
		Escalate:  true,
		Ecosystem: llm.EcoQuadCode,
	})
	if dec.Tier != llm.TierHeavy {
		t.Fatalf("tier=%s want heavy on escalation", dec.Tier)
	}
}

func TestCatalogQuadCodeTiers(t *testing.T) {
	cat := llm.InitCatalog(false)
	models := cat.ForEcosystem(llm.EcoQuadCode)
	if len(models) < 3 {
		t.Fatalf("expected at least 3 quadcode tiers, got %d", len(models))
	}
	m, ok := cat.ModelForTier(llm.EcoQuadCode, llm.TierPremium, false)
	if ok && m.Gated {
		t.Log("premium gated as expected when not allowed")
	}
	m, ok = cat.ModelForTier(llm.EcoQuadCode, llm.TierStandard, false)
	if !ok || m.ID == "" {
		t.Fatal("missing standard quadcode model")
	}
}

func TestRouterOverridesModel(t *testing.T) {
	scripted := &llm.Scripted{
		Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "ok"}}},
	}
	cat := llm.InitCatalog(false)
	advisor := llm.NewAdvisor(cat, nil, "")
	r := llm.NewRouter(scripted, advisor, llm.EcoCodex, false, nil)
	_, err := r.Chat(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "format this file"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(scripted.Calls) != 1 {
		t.Fatalf("calls=%d", len(scripted.Calls))
	}
	if scripted.Calls[0].Model == "" {
		t.Fatal("router did not set model")
	}
	dec := r.LastDecision()
	if dec.Tier != llm.TierLight {
		t.Fatalf("expected light tier, got %s", dec.Tier)
	}
}
