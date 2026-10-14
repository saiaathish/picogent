package llm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCodexPickerIncludesAstraAndAutoUsesLatestFamilies(t *testing.T) {
	previous := CatalogSnapshot()
	t.Cleanup(func() {
		catalogMu.Lock()
		catalog = previous
		catalogMu.Unlock()
	})
	home := t.TempDir()
	t.Setenv("PICOGENT_CODEX_HOME", home)
	t.Setenv("PICOGENT_HOME", t.TempDir())
	cache := `{"models":[
		{"slug":"gpt-6-luna","display_name":"Luna 6","visibility":"list","supported_in_api":true},
		{"slug":"gpt-5.6-terra","display_name":"Terra 5.6","visibility":"list","supported_in_api":true},
		{"slug":"gpt-6.1-sol","display_name":"Sol 6.1","visibility":"list","supported_in_api":true},
		{"slug":"gpt-6-astra","display_name":"Astra 6","visibility":"list","supported_in_api":true}
	]}`
	if err := os.WriteFile(filepath.Join(home, "models_cache.json"), []byte(cache), 0o600); err != nil {
		t.Fatal(err)
	}
	choices := ModelChoices(EcoCodex, false)
	if len(choices) != 5 || choices[0].Value != "auto" {
		t.Fatalf("Codex choices = %+v", choices)
	}
	foundAstra := false
	for _, choice := range choices {
		if choice.Value == "gpt-6-astra" {
			foundAstra = true
		}
	}
	if !foundAstra {
		t.Fatal("Astra absent from Codex picker")
	}
	cat := InitCatalog(false)
	for tier, want := range map[Tier]string{TierLight: "gpt-6-luna", TierStandard: "gpt-5.6-terra", TierHeavy: "gpt-6.1-sol"} {
		found := false
		for _, model := range cat.Models {
			if model.Ecosystem == EcoCodex && model.Tier == tier {
				found = true
				if model.ID != want {
					t.Errorf("%s route = %q, want %q", tier, model.ID, want)
				}
			}
		}
		if !found {
			t.Errorf("missing Codex route for %s", tier)
		}
	}
}
