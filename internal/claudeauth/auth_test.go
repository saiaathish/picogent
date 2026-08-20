package claudeauth_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/saiaathish/picogent/internal/claudeauth"
)

func TestLoadFromCredentialsFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	path := filepath.Join(dir, ".credentials.json")
	blob, _ := json.Marshal(map[string]any{
		"claudeAiOauth": map[string]any{
			"accessToken":      "tok-test",
			"refreshToken":     "ref-test",
			"expiresAt":        9999999999999,
			"subscriptionType": "pro",
		},
	})
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := claudeauth.Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.AccessToken != "tok-test" {
		t.Fatalf("token=%q", c.AccessToken)
	}
	if !claudeauth.LoggedIn() {
		t.Fatal("expected logged in")
	}
	tok, err := claudeauth.Token()
	if err != nil || tok != "tok-test" {
		t.Fatalf("Token()=%q err=%v", tok, err)
	}
}

func TestExpiredCredential(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	path := filepath.Join(dir, ".credentials.json")
	blob, _ := json.Marshal(map[string]any{
		"claudeAiOauth": map[string]any{
			"accessToken": "tok",
			"expiresAt":   1,
		},
	})
	_ = os.WriteFile(path, blob, 0o600)
	c, err := claudeauth.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !c.Expired() {
		t.Fatal("expected expired")
	}
	if claudeauth.LoggedIn() {
		t.Fatal("expired should not count as logged in")
	}
}
