package codexauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Public Codex CLI OAuth client. Same id OpenClaw / kestrel / sagent use.
const ClientID = "app_EMoamEEZ73f0CkXaXp7hrann"

const (
	tokenURL       = "https://auth.openai.com/oauth/token"
	refreshBuffer  = 5 * time.Minute
	defaultModel   = "gpt-5.6-luna"
	originatorName = "codex_cli_rs"
)

var (
	mu     sync.Mutex
	httpDo = http.DefaultClient
)

func HomeDir() string {
	if v := os.Getenv("PICOGENT_CODEX_HOME"); v != "" {
		return v
	}
	if v := os.Getenv("CODEX_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex")
}

func AuthPath() string {
	return filepath.Join(HomeDir(), "auth.json")
}

func LoggedIn() bool {
	c, err := loadFile(AuthPath())
	if err != nil {
		return false
	}
	return c.AccessToken != "" || c.RefreshToken != ""
}

func DefaultModel() string {
	b, err := os.ReadFile(filepath.Join(HomeDir(), "config.toml"))
	if err == nil {
		if m := regexp.MustCompile(`(?m)^\s*model\s*=\s*"([^"]+)"`).FindSubmatch(b); len(m) == 2 {
			id := string(m[1])
			if strings.EqualFold(id, "auto") {
				return defaultModel
			}
			return id
		}
	}
	return defaultModel
}

func Originator() string { return originatorName }

type creds struct {
	AccessToken  string
	RefreshToken string
	AccountID    string
	IDToken      string
	raw          map[string]any
}

func Token(ctx context.Context) (access, account string, err error) {
	mu.Lock()
	defer mu.Unlock()
	return tokenLocked(ctx, false)
}

func ForceRefresh(ctx context.Context) error {
	mu.Lock()
	defer mu.Unlock()
	_, _, err := tokenLocked(ctx, true)
	return err
}

func tokenLocked(ctx context.Context, force bool) (string, string, error) {
	path := AuthPath()
	unlock, err := lockAuth(path)
	if err != nil {
		// Fall back to process-local locking when the file lock is unavailable
		// (sandbox, read-only home, etc.). Concurrent writers across processes
		// are still best-effort protected by mu above.
		unlock = func() {}
	}
	defer unlock()

	c, err := loadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("no Codex login at %s (run: picogent login)", path)
	}
	if !force && !expiring(c.AccessToken) && c.AccessToken != "" {
		return c.AccessToken, c.AccountID, nil
	}
	if c.RefreshToken == "" {
		if c.AccessToken != "" {
			return c.AccessToken, c.AccountID, nil
		}
		return "", "", fmt.Errorf("Codex auth.json has no tokens. Run: picogent login")
	}
	next, err := refresh(ctx, c)
	if err != nil {
		if c.AccessToken != "" && !force {
			return c.AccessToken, c.AccountID, nil
		}
		return "", "", err
	}
	if err := saveFile(path, next); err != nil {
		return "", "", err
	}
	return next.AccessToken, next.AccountID, nil
}

func expiring(access string) bool {
	exp := jwtExp(access)
	if exp.IsZero() {
		return false
	}
	return time.Now().After(exp.Add(-refreshBuffer))
}

func jwtExp(tok string) time.Time {
	parts := strings.Split(tok, ".")
	if len(parts) < 2 {
		return time.Time{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return time.Time{}
		}
	}
	var p struct {
		Exp int64 `json:"exp"`
	}
	if json.Unmarshal(payload, &p) != nil || p.Exp == 0 {
		return time.Time{}
	}
	return time.Unix(p.Exp, 0)
}

func refresh(ctx context.Context, c creds) (creds, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {c.RefreshToken},
		"client_id":     {ClientID},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return creds{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := httpDo.Do(req)
	if err != nil {
		return creds{}, fmt.Errorf("Codex token refresh failed: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 400 {
		return creds{}, fmt.Errorf("Codex login expired (http %d). Run: picogent login", res.StatusCode)
	}
	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return creds{}, fmt.Errorf("Codex token refresh: bad json")
	}
	if out.AccessToken == "" {
		return creds{}, fmt.Errorf("Codex token refresh returned no access token")
	}
	c.AccessToken = out.AccessToken
	if out.RefreshToken != "" {
		c.RefreshToken = out.RefreshToken
	}
	if out.IDToken != "" {
		c.IDToken = out.IDToken
	}
	return c, nil
}

func loadFile(path string) (creds, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return creds{}, err
	}
	raw := map[string]any{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return creds{}, err
	}
	tokens, _ := raw["tokens"].(map[string]any)
	if tokens == nil {
		return creds{}, fmt.Errorf("auth.json missing tokens")
	}
	str := func(k string) string {
		v, _ := tokens[k].(string)
		return v
	}
	return creds{
		AccessToken:  str("access_token"),
		RefreshToken: str("refresh_token"),
		AccountID:    str("account_id"),
		IDToken:      str("id_token"),
		raw:          raw,
	}, nil
}

func saveFile(path string, c creds) error {
	raw := c.raw
	if raw == nil {
		raw = map[string]any{}
	}
	tokens, _ := raw["tokens"].(map[string]any)
	if tokens == nil {
		tokens = map[string]any{}
	}
	tokens["access_token"] = c.AccessToken
	tokens["refresh_token"] = c.RefreshToken
	tokens["account_id"] = c.AccountID
	if c.IDToken != "" {
		tokens["id_token"] = c.IDToken
	}
	raw["tokens"] = tokens
	raw["auth_mode"] = "chatgpt"
	raw["last_refresh"] = time.Now().UTC().Format(time.RFC3339)
	b, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
