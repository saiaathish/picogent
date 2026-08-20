package llm_test

import (
	"os"
	"testing"

	"github.com/saiaathish/picogent/internal/llm"
)

func TestModelChoicesIncludesCLIModels(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PICOGENT_OPENCODE_HOME", dir)
	// Both plans logged in so Zen + Go lists are visible.
	_ = os.WriteFile(dir+"/auth.json", []byte(`{"opencode":{"type":"api","key":"z"},"opencode-go":{"type":"api","key":"g"}}`), 0o600)

	llm.SetCLIModels(llm.EcoOpenCode, []llm.ModelEntry{
		{ID: "opencode/hy3-free", Display: "Hy3 Free", Ecosystem: llm.EcoOpenCode},
	})
	llm.SetCLIModels(llm.EcoOpenCodeGo, []llm.ModelEntry{
		{ID: "opencode-go/kimi-k3", Display: "Kimi K3", Ecosystem: llm.EcoOpenCodeGo},
	})
	llm.SetCLIModels(llm.EcoAntigravity, []llm.ModelEntry{
		{ID: "gemini-3.7-flash-high", Display: "Gemini 3.7 Flash", Ecosystem: llm.EcoAntigravity},
	})

	oc := llm.ModelChoices(llm.EcoOpenCode, false)
	foundZen, foundGo, foundAuto := false, false, false
	for _, c := range oc {
		if c.Value == "auto" {
			foundAuto = true
		}
		if c.Value == "opencode/hy3-free" {
			foundZen = true
		}
		if c.Value == "opencode-go/kimi-k3" {
			foundGo = true
		}
	}
	if foundAuto {
		t.Fatal("opencode must not offer Auto router")
	}
	if !foundZen || !foundGo {
		t.Fatalf("expected zen+go in opencode choices: %+v", oc)
	}

	agy := llm.ModelChoices(llm.EcoAntigravity, false)
	found, foundAgyAuto := false, false
	for _, c := range agy {
		if c.Value == "auto" {
			foundAgyAuto = true
		}
		if c.Value == "gemini-3.7-flash-high" {
			found = true
		}
	}
	if foundAgyAuto {
		t.Fatal("antigravity must not offer Auto router")
	}
	if !found {
		t.Fatalf("expected antigravity cli model: %+v", agy)
	}

	codex := llm.ModelChoices(llm.EcoCodex, false)
	if len(codex) == 0 || codex[0].Value != "auto" {
		t.Fatalf("codex should lead with Auto: %+v", codex)
	}
}

func TestEcosystemForProvider(t *testing.T) {
	if llm.EcosystemForProvider("opencode") != llm.EcoOpenCode {
		t.Fatal("opencode")
	}
	if llm.EcosystemForProvider("antigravity") != llm.EcoAntigravity {
		t.Fatal("antigravity")
	}
}

func TestRefreshCLIModelsLive(t *testing.T) {
	llm.RefreshCLIModels(true)
	oc := llm.CLIModels(llm.EcoOpenCode)
	goModels := llm.CLIModels(llm.EcoOpenCodeGo)
	agy := llm.CLIModels(llm.EcoAntigravity)
	t.Logf("opencode=%d opencode-go=%d antigravity=%d", len(oc), len(goModels), len(agy))
	if len(oc)+len(goModels) == 0 && len(agy) == 0 {
		t.Fatal("expected at least some CLI/API models")
	}
}
