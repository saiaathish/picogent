// Package opencodeauth reads OpenCode CLI credentials (Zen + Go)
// from the same store as `opencode auth login`.
package opencodeauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	ProviderZen = "opencode"
	ProviderGo  = "opencode-go"

	ZenBaseURL = "https://opencode.ai/zen/v1"
	GoBaseURL  = "https://opencode.ai/zen/go/v1"
)

// Cred is one OpenCode provider API key entry.
type Cred struct {
	Type string `json:"type"`
	Key  string `json:"key"`
}

// HomeDir returns the OpenCode data directory (auth.json parent).
func HomeDir() string {
	if v := os.Getenv("PICOGENT_OPENCODE_HOME"); v != "" {
		return v
	}
	if v := os.Getenv("OPENCODE_DATA_DIR"); v != "" {
		return v
	}
	// OpenCode XDG default: ~/.local/share/opencode
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".local", "share", "opencode")
	}
	return ""
}

// AuthPath is ~/.local/share/opencode/auth.json (or override).
func AuthPath() string {
	return filepath.Join(HomeDir(), "auth.json")
}

// LoadAll returns provider → cred from the OpenCode CLI auth file.
func LoadAll() (map[string]Cred, error) {
	b, err := os.ReadFile(AuthPath())
	if err != nil {
		return nil, err
	}
	raw := map[string]Cred{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := map[string]Cred{}
	for k, v := range raw {
		if strings.TrimSpace(v.Key) == "" {
			continue
		}
		out[k] = v
	}
	return out, nil
}

// Key returns the API key for a provider id (opencode or opencode-go).
func Key(provider string) (string, error) {
	all, err := LoadAll()
	if err != nil {
		return "", err
	}
	c, ok := all[provider]
	if !ok || c.Key == "" {
		return "", errors.New("OpenCode provider " + provider + " is not logged in")
	}
	return c.Key, nil
}

// LoggedIn reports whether at least one of Zen or Go has an API key.
func LoggedIn() bool {
	return ZenLoggedIn() || GoLoggedIn()
}

// ZenLoggedIn reports OpenCode Zen credentials.
func ZenLoggedIn() bool {
	k, err := Key(ProviderZen)
	return err == nil && k != ""
}

// GoLoggedIn reports OpenCode Go credentials.
func GoLoggedIn() bool {
	k, err := Key(ProviderGo)
	return err == nil && k != ""
}

// KeyForModel picks Zen vs Go from a model id like "opencode-go/kimi-k3" or "kimi-k3".
// Explicit prefixes never fall back to the other plan (Zen free models are not on Go).
func KeyForModel(model string) (provider, baseURL, apiKey, bareModel string, err error) {
	model = strings.TrimSpace(model)
	explicit := false
	provider = ProviderGo
	baseURL = GoBaseURL
	bareModel = model
	switch {
	case strings.HasPrefix(model, "opencode-go/"):
		bareModel = strings.TrimPrefix(model, "opencode-go/")
		provider = ProviderGo
		baseURL = GoBaseURL
		explicit = true
	case strings.HasPrefix(model, "opencode/"):
		bareModel = strings.TrimPrefix(model, "opencode/")
		provider = ProviderZen
		baseURL = ZenBaseURL
		explicit = true
	case ZenLoggedIn() && !GoLoggedIn():
		provider = ProviderZen
		baseURL = ZenBaseURL
	}
	apiKey, err = Key(provider)
	if err != nil {
		plan := "Zen"
		loginHint := "`opencode auth login` and select OpenCode Zen"
		if provider == ProviderGo {
			plan = "Go"
			loginHint = "`opencode auth login` and select OpenCode Go"
		}
		if explicit {
			return "", "", "", "", fmt.Errorf("model %s needs OpenCode %s — you are not logged into %s.\nFix:     run %s (or pick an %s model)", model, plan, plan, loginHint, otherPlan(plan))
		}
		// Unprefixed model: try the other plan only when that is the only login.
		alt := ProviderZen
		altBase := ZenBaseURL
		if provider == ProviderZen {
			alt = ProviderGo
			altBase = GoBaseURL
		}
		if k2, e2 := Key(alt); e2 == nil {
			return alt, altBase, k2, bareModel, nil
		}
		return "", "", "", "", fmt.Errorf("OpenCode is not logged in.\nFix:     run `opencode auth login` (Zen or Go)")
	}
	return provider, baseURL, apiKey, bareModel, nil
}

func otherPlan(plan string) string {
	if plan == "Zen" {
		return "Go"
	}
	return "Zen"
}

// DefaultModel prefers a Go coding model, else a Zen free/default model.
func DefaultModel() string {
	if GoLoggedIn() {
		return "opencode-go/kimi-k2.6"
	}
	if ZenLoggedIn() {
		return "opencode/big-pickle"
	}
	return "opencode-go/kimi-k2.6"
}
