package opencodeauth_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/saiaathish/picogent/internal/opencodeauth"
)

func TestKeyForModel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PICOGENT_OPENCODE_HOME", dir)
	auth := map[string]any{
		"opencode-go": map[string]any{"type": "api", "key": "sk-go"},
		"opencode":    map[string]any{"type": "api", "key": "sk-zen"},
	}
	b, _ := json.Marshal(auth)
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	if !opencodeauth.LoggedIn() || !opencodeauth.GoLoggedIn() || !opencodeauth.ZenLoggedIn() {
		t.Fatal("expected logged in")
	}
	p, base, key, bare, err := opencodeauth.KeyForModel("opencode-go/kimi-k3")
	if err != nil || p != "opencode-go" || key != "sk-go" || bare != "kimi-k3" || base != opencodeauth.GoBaseURL {
		t.Fatalf("go: %s %s %s %s %v", p, base, key, bare, err)
	}
	p, base, key, bare, err = opencodeauth.KeyForModel("opencode/gpt-5.5")
	if err != nil || p != "opencode" || key != "sk-zen" || bare != "gpt-5.5" || base != opencodeauth.ZenBaseURL {
		t.Fatalf("zen: %s %s %s %s %v", p, base, key, bare, err)
	}
}

func TestKeyForModelNoCrossPlanFallback(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PICOGENT_OPENCODE_HOME", dir)
	auth := map[string]any{
		"opencode-go": map[string]any{"type": "api", "key": "sk-go"},
	}
	b, _ := json.Marshal(auth)
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, _, _, err := opencodeauth.KeyForModel("opencode/x-preview-f-free")
	if err == nil {
		t.Fatal("expected error when Zen model requested with only Go login")
	}
}
