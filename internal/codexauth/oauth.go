package codexauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	AuthorizeEndpoint = "https://auth.openai.com/oauth/authorize"
	RedirectURI       = "http://localhost:1455/auth/callback"
	CallbackPort      = "1455"
	OAuthScopes       = "openid profile email offline_access api.connectors.read api.connectors.invoke"
)

type pendingOAuth struct {
	verifier string
	state    string
	returnTo string
	srv      *http.Server
}

var (
	oauthMu sync.Mutex
	pending *pendingOAuth
)

func randomURLToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func pkcePair() (verifier, challenge string) {
	verifier = randomURLToken(32)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return verifier, challenge
}

func AuthorizeURLFor(challenge, state string) string {
	q := url.Values{
		"client_id":                  {ClientID},
		"response_type":              {"code"},
		"redirect_uri":               {RedirectURI},
		"scope":                      {OAuthScopes},
		"code_challenge":             {challenge},
		"code_challenge_method":      {"S256"},
		"state":                      {state},
		"codex_cli_simplified_flow":  {"true"},
		"id_token_add_organizations": {"true"},
		"originator":                 {originatorName},
	}
	return AuthorizeEndpoint + "?" + q.Encode()
}

// BeginBrowserLogin starts the Codex CLI callback on localhost:1455 and
// returns the ChatGPT authorize URL. After login, the browser is sent back to returnTo.
func BeginBrowserLogin(returnTo string) (string, error) {
	oauthMu.Lock()
	defer oauthMu.Unlock()
	stopPendingLocked()

	verifier, challenge := pkcePair()
	state := randomURLToken(24)
	ln, err := net.Listen("tcp", "127.0.0.1:"+CallbackPort)
	if err != nil {
		return "", fmt.Errorf("cannot listen on localhost:%s (another Codex login may be open): %w", CallbackPort, err)
	}
	p := &pendingOAuth{verifier: verifier, state: state, returnTo: returnTo}
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", p.handleCallback)
	p.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	pending = p
	go func() { _ = p.srv.Serve(ln) }()
	return AuthorizeURLFor(challenge, state), nil
}

func stopPendingLocked() {
	if pending == nil {
		return
	}
	if pending.srv != nil {
		go pending.srv.Close()
	}
	pending = nil
}

func (p *pendingOAuth) handleCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if errMsg := q.Get("error"); errMsg != "" {
		http.Redirect(w, r, bounce(p.returnTo, "error", errMsg), http.StatusFound)
		return
	}
	if q.Get("state") != p.state {
		http.Error(w, "bad oauth state", http.StatusBadRequest)
		return
	}
	code := q.Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}
	if err := ExchangeCode(r.Context(), code, p.verifier); err != nil {
		http.Redirect(w, r, bounce(p.returnTo, "error", err.Error()), http.StatusFound)
		return
	}
	http.Redirect(w, r, bounce(p.returnTo, "login", "ok"), http.StatusFound)
	go func() {
		time.Sleep(300 * time.Millisecond)
		oauthMu.Lock()
		stopPendingLocked()
		oauthMu.Unlock()
	}()
}

func bounce(returnTo, key, val string) string {
	if returnTo == "" {
		returnTo = "http://127.0.0.1:7420/setup.html"
	}
	u, err := url.Parse(returnTo)
	if err != nil {
		return returnTo
	}
	q := u.Query()
	q.Set(key, val)
	u.RawQuery = q.Encode()
	return u.String()
}

func ExchangeCode(ctx context.Context, code, verifier string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {RedirectURI},
		"client_id":     {ClientID},
		"code_verifier": {verifier},
	}.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := httpDo.Do(req)
	if err != nil {
		return fmt.Errorf("token exchange failed: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 400 {
		return fmt.Errorf("token exchange http %d: %s", res.StatusCode, truncate(string(body), 240))
	}
	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("token exchange: bad json")
	}
	if out.AccessToken == "" {
		return fmt.Errorf("token exchange returned no access token")
	}
	return SaveTokens(out.AccessToken, out.RefreshToken, out.IDToken, AccountIDFromAccess(out.AccessToken))
}

func AccountIDFromAccess(access string) string {
	parts := strings.Split(access, ".")
	if len(parts) < 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return ""
		}
	}
	var claims map[string]any
	if json.Unmarshal(payload, &claims) != nil {
		return ""
	}
	auth, _ := claims["https://api.openai.com/auth"].(map[string]any)
	if auth == nil {
		return ""
	}
	id, _ := auth["chatgpt_account_id"].(string)
	return id
}

func SaveTokens(access, refresh, idToken, accountID string) error {
	dir := HomeDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := AuthPath()
	existing, _ := loadFile(path)
	if existing.raw == nil {
		existing.raw = map[string]any{}
	}
	existing.AccessToken = access
	if refresh != "" {
		existing.RefreshToken = refresh
	}
	if idToken != "" {
		existing.IDToken = idToken
	}
	if accountID != "" {
		existing.AccountID = accountID
	}
	return saveFile(path, existing)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
