package codexauth

import (
	"strings"
	"testing"
)

func TestAuthorizeURLFor(t *testing.T) {
	u := AuthorizeURLFor("chal", "st")
	if !strings.Contains(u, "client_id="+ClientID) && !strings.Contains(u, "client_id="+urlQueryEscape(ClientID)) {
		if !strings.Contains(u, ClientID) {
			t.Fatalf("%s", u)
		}
	}
	if !strings.Contains(u, "code_challenge=chal") {
		t.Fatalf("%s", u)
	}
	if !strings.Contains(u, "localhost%3A1455") && !strings.Contains(u, "localhost:1455") {
		t.Fatalf("redirect %s", u)
	}
}

func urlQueryEscape(s string) string { return s }

func TestAccountIDFromAccess(t *testing.T) {
	// {"https://api.openai.com/auth":{"chatgpt_account_id":"acct_9"}}
	tok := "x.eyJodHRwczovL2FwaS5vcGVuYWkuY29tL2F1dGgiOnsiY2hhdGdwdF9hY2NvdW50X2lkIjoiYWNjdF85In19.y"
	if got := AccountIDFromAccess(tok); got != "acct_9" {
		t.Fatalf("%q", got)
	}
}

func TestSaveTokens(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PICOGENT_CODEX_HOME", dir)
	if err := SaveTokens("acc", "ref", "id", "acct"); err != nil {
		t.Fatal(err)
	}
	if !LoggedIn() {
		t.Fatal("expected logged in")
	}
}
