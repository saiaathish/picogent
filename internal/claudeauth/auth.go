// Package claudeauth reads Claude Code CLI subscription credentials
// (same store as `claude /login`) so Picogent can use Claude without an API key.
package claudeauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const keychainService = "Claude Code-credentials"

// Cred holds a Claude Code OAuth credential.
type Cred struct {
	AccessToken      string
	RefreshToken     string
	ExpiresAt        int64 // ms since epoch; 0 unknown
	SubscriptionType string
}

// LoggedIn reports whether Claude Code CLI credentials are available.
func LoggedIn() bool {
	c, err := Load()
	return err == nil && c.AccessToken != "" && !c.Expired()
}

// Expired reports whether the access token is past expiresAt.
func (c Cred) Expired() bool {
	if c.ExpiresAt <= 0 {
		return false
	}
	return time.Now().UnixMilli() >= c.ExpiresAt
}

// Load reads Claude Code OAuth from macOS Keychain or ~/.claude/.credentials.json.
func Load() (Cred, error) {
	// Explicit config dir (tests / CLAUDE_CONFIG_DIR) always wins.
	if os.Getenv("CLAUDE_CONFIG_DIR") != "" {
		return loadFile()
	}
	if runtime.GOOS == "darwin" {
		if c, err := loadKeychain(); err == nil && c.AccessToken != "" {
			return c, nil
		}
	}
	return loadFile()
}

// Token returns a usable access token, or an error if not logged in / expired.
func Token() (string, error) {
	c, err := Load()
	if err != nil {
		return "", err
	}
	if c.AccessToken == "" {
		return "", errors.New("Claude Code is not logged in")
	}
	if c.Expired() {
		return "", fmt.Errorf("Claude Code session expired.\nFix:     run `claude auth login` (or picogent setup → Log in to Claude)")
	}
	return c.AccessToken, nil
}

func loadKeychain() (Cred, error) {
	out, err := exec.Command("security", "find-generic-password", "-s", keychainService, "-w").Output()
	if err != nil {
		return Cred{}, err
	}
	return parseCredJSON(strings.TrimSpace(string(out)))
}

func loadFile() (Cred, error) {
	path, err := credentialsPath()
	if err != nil {
		return Cred{}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Cred{}, err
	}
	return parseCredJSON(string(b))
}

func credentialsPath() (string, error) {
	if v := os.Getenv("CLAUDE_CONFIG_DIR"); v != "" {
		return filepath.Join(v, ".credentials.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", ".credentials.json"), nil
}

func parseCredJSON(raw string) (Cred, error) {
	var outer struct {
		ClaudeAiOauth *struct {
			AccessToken      string `json:"accessToken"`
			RefreshToken     string `json:"refreshToken"`
			ExpiresAt        int64  `json:"expiresAt"`
			SubscriptionType string `json:"subscriptionType"`
		} `json:"claudeAiOauth"`
		// Flat fallback shapes seen in older dumps.
		AccessToken  string `json:"accessToken"`
		AccessToken2 string `json:"access_token"`
	}
	if err := json.Unmarshal([]byte(raw), &outer); err != nil {
		return Cred{}, err
	}
	if outer.ClaudeAiOauth != nil && outer.ClaudeAiOauth.AccessToken != "" {
		return Cred{
			AccessToken:      outer.ClaudeAiOauth.AccessToken,
			RefreshToken:     outer.ClaudeAiOauth.RefreshToken,
			ExpiresAt:        outer.ClaudeAiOauth.ExpiresAt,
			SubscriptionType: outer.ClaudeAiOauth.SubscriptionType,
		}, nil
	}
	tok := outer.AccessToken
	if tok == "" {
		tok = outer.AccessToken2
	}
	if tok == "" {
		return Cred{}, errors.New("no Claude Code access token in credentials")
	}
	return Cred{AccessToken: tok}, nil
}
