// Package agyauth reads Google Antigravity CLI credentials and model lists
// (same store as `agy` / `agy models`).
package agyauth

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	keychainService = "gemini"
	keychainAccount = "antigravity"
	defaultModel    = "gemini-3.5-flash-medium"
)

// Cred holds an Antigravity OAuth token set.
type Cred struct {
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
	AuthMethod   string
}

// HomeDir is ~/.gemini/antigravity-cli (or override).
func HomeDir() string {
	if v := os.Getenv("PICOGENT_AGY_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".gemini", "antigravity-cli")
}

// TokenPath is the SSH/file-based OAuth token path used by `agy`.
func TokenPath() string {
	return filepath.Join(HomeDir(), "antigravity-oauth-token")
}

// SettingsPath is ~/.gemini/antigravity-cli/settings.json.
func SettingsPath() string {
	return filepath.Join(HomeDir(), "settings.json")
}

// GeminiAPIKey returns GEMINI_API_KEY when Antigravity is configured for Gemini API key mode.
func GeminiAPIKey() string {
	if v := os.Getenv("GEMINI_API_KEY"); v != "" {
		return v
	}
	if v := os.Getenv("PICOGENT_GEMINI_API_KEY"); v != "" {
		return v
	}
	return ""
}

// UsesGeminiAPIKey reports settings.json modelProvider=gemini + a key present.
func UsesGeminiAPIKey() bool {
	if GeminiAPIKey() == "" {
		return false
	}
	b, err := os.ReadFile(SettingsPath())
	if err != nil {
		// Key alone is enough for Picogent's Gemini OpenAI-compat path.
		return true
	}
	var s struct {
		ModelProvider string `json:"modelProvider"`
	}
	_ = json.Unmarshal(b, &s)
	return s.ModelProvider == "" || s.ModelProvider == "gemini"
}

// LoggedIn reports Antigravity CLI session or Gemini API key availability.
func LoggedIn() bool {
	if UsesGeminiAPIKey() && GeminiAPIKey() != "" {
		return true
	}
	c, err := Load()
	return err == nil && c.AccessToken != "" && !c.Expired()
}

// Expired reports whether the access token is past expiry (refresh may still work).
func (c Cred) Expired() bool {
	if c.Expiry.IsZero() {
		return false
	}
	return time.Now().After(c.Expiry.Add(-2 * time.Minute))
}

// Load reads Antigravity OAuth from the CLI token file or macOS Keychain.
func Load() (Cred, error) {
	if c, err := loadFile(TokenPath()); err == nil && c.AccessToken != "" {
		return c, nil
	}
	if runtime.GOOS == "darwin" {
		if c, err := loadKeychain(); err == nil && c.AccessToken != "" {
			return c, nil
		}
	}
	return Cred{}, errors.New("Antigravity is not logged in")
}

// Token returns a usable access token (may be expired if only refresh is available).
func Token() (string, error) {
	c, err := Load()
	if err != nil {
		return "", err
	}
	if c.AccessToken == "" {
		return "", errors.New("Antigravity is not logged in")
	}
	return c.AccessToken, nil
}

// RefreshToken returns the durable refresh token when present.
func RefreshToken() (string, error) {
	c, err := Load()
	if err != nil {
		return "", err
	}
	if c.RefreshToken == "" {
		return "", errors.New("Antigravity refresh token missing — run `agy` to log in")
	}
	return c.RefreshToken, nil
}

// DefaultModel is a sensible Antigravity default.
func DefaultModel() string {
	if models, err := ListModels(); err == nil {
		for _, m := range models {
			if m.ID == defaultModel {
				return m.ID
			}
		}
		if len(models) > 0 {
			return models[0].ID
		}
	}
	return defaultModel
}

// ModelInfo is one line from `agy models`.
type ModelInfo struct {
	ID    string
	Label string
}

// ListModels runs `agy models` when the CLI is installed.
func ListModels() ([]ModelInfo, error) {
	bin, err := exec.LookPath("agy")
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, "models")
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}
	var models []ModelInfo
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Fetching") {
			continue
		}
		// Format: slug<TAB>Display Name  or  slug  Display Name
		id, label, ok := splitModelLine(line)
		if !ok {
			continue
		}
		models = append(models, ModelInfo{ID: id, Label: label})
	}
	if len(models) == 0 {
		return nil, errors.New("agy models returned no entries")
	}
	return models, nil
}

func splitModelLine(line string) (id, label string, ok bool) {
	if i := strings.IndexByte(line, '\t'); i >= 0 {
		id = strings.TrimSpace(line[:i])
		label = strings.TrimSpace(line[i+1:])
		return id, label, id != ""
	}
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return "", "", false
	}
	id = parts[0]
	if len(parts) == 1 {
		return id, id, true
	}
	return id, strings.Join(parts[1:], " "), true
}

func loadFile(path string) (Cred, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Cred{}, err
	}
	var outer struct {
		Token *struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
			Expiry       string `json:"expiry"`
			TokenType    string `json:"token_type"`
		} `json:"token"`
		AuthMethod string `json:"auth_method"`
		// Flat shapes
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Expiry       string `json:"expiry"`
	}
	if err := json.Unmarshal(b, &outer); err != nil {
		return Cred{}, err
	}
	c := Cred{AuthMethod: outer.AuthMethod}
	if outer.Token != nil {
		c.AccessToken = outer.Token.AccessToken
		c.RefreshToken = outer.Token.RefreshToken
		c.Expiry = parseExpiry(outer.Token.Expiry)
	}
	if c.AccessToken == "" {
		c.AccessToken = outer.AccessToken
		c.RefreshToken = outer.RefreshToken
		c.Expiry = parseExpiry(outer.Expiry)
	}
	if c.AccessToken == "" && c.RefreshToken == "" {
		return Cred{}, errors.New("no Antigravity tokens in " + path)
	}
	return c, nil
}

func parseExpiry(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.000000000Z",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func loadKeychain() (Cred, error) {
	out, err := exec.Command("security", "find-generic-password", "-s", keychainService, "-a", keychainAccount, "-w").Output()
	if err != nil {
		// Try service-only lookup.
		out, err = exec.Command("security", "find-generic-password", "-s", keychainService, "-w").Output()
		if err != nil {
			return Cred{}, err
		}
	}
	raw := strings.TrimSpace(string(out))
	// Keychain may store raw JSON or a nested blob.
	if strings.HasPrefix(raw, "{") {
		tmp := filepath.Join(os.TempDir(), "agy-kc.json")
		_ = os.WriteFile(tmp, []byte(raw), 0o600)
		defer os.Remove(tmp)
		return loadFile(tmp)
	}
	return Cred{}, errors.New("keychain item is not JSON")
}

// CLIInstalled reports whether `agy` is on PATH.
func CLIInstalled() bool {
	_, err := exec.LookPath("agy")
	return err == nil
}
