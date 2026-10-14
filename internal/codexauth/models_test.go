package codexauth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAvailableModelsAndLatestFamilies(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PICOGENT_CODEX_HOME", home)
	cache := `{"models":[
		{"slug":"gpt-5.6-luna","display_name":"Luna 5.6","visibility":"list","supported_in_api":true,"priority":5},
		{"slug":"gpt-6-luna","display_name":"Luna 6","visibility":"list","supported_in_api":true,"priority":3},
		{"slug":"gpt-5.6-terra","display_name":"Terra 5.6","visibility":"list","supported_in_api":true,"priority":4},
		{"slug":"gpt-6.1-sol","display_name":"Sol 6.1","visibility":"list","supported_in_api":true,"priority":1},
		{"slug":"gpt-6-astra","display_name":"Astra 6","visibility":"list","supported_in_api":true,"priority":2},
		{"slug":"gpt-9-sol","visibility":"hide","supported_in_api":true},
		{"slug":"gpt-9-terra","visibility":"list","supported_in_api":false},
		{"slug":"gpt-6-astra","visibility":"list","supported_in_api":true,"priority":9}
	]}`
	if err := os.WriteFile(filepath.Join(home, "models_cache.json"), []byte(cache), 0o600); err != nil {
		t.Fatal(err)
	}
	models := AvailableModels()
	if len(models) != 5 || models[1].Slug != "gpt-6-astra" {
		t.Fatalf("pickable models = %+v", models)
	}
	for family, want := range map[string]string{"luna": "gpt-6-luna", "terra": "gpt-5.6-terra", "sol": "gpt-6.1-sol"} {
		if got := LatestFamily(family); got != want {
			t.Errorf("latest %s = %q, want %q", family, got, want)
		}
	}
	if got := DefaultModel(); got != "gpt-6-luna" {
		t.Fatalf("default model = %q", got)
	}
}
