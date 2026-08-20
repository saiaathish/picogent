package codexauth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoggedInAndDefaultModel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PICOGENT_CODEX_HOME", dir)
	if LoggedIn() {
		t.Fatal("expected logged out")
	}
	auth := map[string]any{
		"auth_mode": "chatgpt",
		"tokens": map[string]string{
			"access_token":  "aaa",
			"refresh_token": "bbb",
			"account_id":    "acct",
		},
	}
	b, _ := json.Marshal(auth)
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	if !LoggedIn() {
		t.Fatal("expected logged in")
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("model = \"gpt-5.6-luna\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if DefaultModel() != "gpt-5.6-luna" {
		t.Fatalf("%s", DefaultModel())
	}
}

func TestJwtExp(t *testing.T) {
	// header.payload.sig — payload {"exp": 2000000000}
	tok := "eyJhbGciOiJub25lIn0.eyJleHAiOjIwMDAwMDAwMDB9.x"
	got := jwtExp(tok)
	if got.Unix() != 2000000000 {
		t.Fatalf("%v", got)
	}
	if expiring("not-a-jwt") {
		t.Fatal("non-jwt should not look expired")
	}
	_ = time.Now()
}
