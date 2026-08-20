package llm

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Ecosystem is a model family Picogent can auto-route within.
type Ecosystem string

const (
	EcoCodex    Ecosystem = "codex"
	EcoQuadCode Ecosystem = "quadcode"
)

// Tier is a complexity band inside an ecosystem.
type Tier string

const (
	TierLight    Tier = "light"
	TierStandard Tier = "standard"
	TierHeavy    Tier = "heavy"
	TierPremium  Tier = "premium"
)

// ModelEntry is one routable model in a catalog.
type ModelEntry struct {
	ID          string    `json:"id"`
	Display     string    `json:"display"`
	Tier        Tier      `json:"tier"`
	Ecosystem   Ecosystem `json:"ecosystem"`
	Description string    `json:"description"`
	Patterns    []string  `json:"patterns,omitempty"`
	Gated       bool      `json:"gated,omitempty"`
}

// Catalog holds routable models for each ecosystem.
type Catalog struct {
	Version   string       `json:"version"`
	Updated   time.Time    `json:"updated"`
	Models    []ModelEntry `json:"models"`
	SourceURL string       `json:"source_url,omitempty"`
}

var (
	catalogMu sync.RWMutex
	catalog   = defaultCatalog()
)

const catalogRemote = "https://raw.githubusercontent.com/saiaathish/picogent/main/internal/llm/router-catalog.json"

//go:embed router-catalog.json
var embeddedCatalogJSON []byte

func defaultCatalog() Catalog {
	if len(embeddedCatalogJSON) > 0 {
		var c Catalog
		if json.Unmarshal(embeddedCatalogJSON, &c) == nil && len(c.Models) > 0 {
			if c.Updated.IsZero() {
				c.Updated = time.Now().UTC()
			}
			return c
		}
	}
	now := time.Now().UTC()
	return Catalog{
		Version: "2026.1",
		Updated: now,
		Models: []ModelEntry{
			// Codex GPT-5.6 family — patterns match future 5.7+ releases.
			{ID: "gpt-5.6-luna", Display: "Luna", Tier: TierLight, Ecosystem: EcoCodex, Description: "Linting, tests, docs, quick fixes", Patterns: []string{`gpt-5\.\d+-luna`, `gpt-\d+\.\d+-luna`}},
			{ID: "gpt-5.6-terra", Display: "Terra", Tier: TierStandard, Ecosystem: EcoCodex, Description: "Implementation, refactoring, daily coding", Patterns: []string{`gpt-5\.\d+-terra`, `gpt-\d+\.\d+-terra`}},
			{ID: "gpt-5.6-sol", Display: "Sol", Tier: TierHeavy, Ecosystem: EcoCodex, Description: "Planning, architecture, complex reasoning", Patterns: []string{`gpt-5\.\d+-sol`, `gpt-5\.\d+-soul`, `gpt-\d+\.\d+-sol`}},

			// Quad Code / Claude Code family.
			{ID: "claude-haiku-4-5", Display: "Haiku 4.5", Tier: TierLight, Ecosystem: EcoQuadCode, Description: "Fast loops — quick edits and simple tasks", Patterns: []string{`claude-.*haiku`, `haiku-\d`}},
			{ID: "claude-sonnet-5", Display: "Sonnet 5", Tier: TierStandard, Ecosystem: EcoQuadCode, Description: "Daily coding — best speed vs quality balance", Patterns: []string{`claude-sonnet-5`, `claude-.*sonnet-5`}},
			{ID: "claude-opus-5", Display: "Opus 5", Tier: TierHeavy, Ecosystem: EcoQuadCode, Description: "Complex agentic work and deep reasoning", Patterns: []string{`claude-opus-5`, `claude-.*opus-5`}},
			{ID: "claude-fable-5", Display: "Fable 5", Tier: TierPremium, Ecosystem: EcoQuadCode, Description: "Premium tier — API billing required", Patterns: []string{`claude-fable-5`, `claude-.*fable`}, Gated: true},
		},
	}
}

func CatalogSnapshot() Catalog {
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	return catalog
}

func EcosystemForProvider(provider string) Ecosystem {
	switch strings.ToLower(provider) {
	case "quadcode", "claude", "claude-code":
		return EcoQuadCode
	default:
		return EcoCodex
	}
}

func TierLabel(eco Ecosystem, tier Tier) string {
	switch eco {
	case EcoQuadCode:
		switch tier {
		case TierLight:
			return "Haiku"
		case TierStandard:
			return "Sonnet"
		case TierHeavy:
			return "Opus"
		case TierPremium:
			return "Fable"
		}
	case EcoCodex:
		switch tier {
		case TierLight:
			return "Luna"
		case TierStandard:
			return "Terra"
		case TierHeavy:
			return "Sol"
		case TierPremium:
			return "Sol"
		}
	}
	return string(tier)
}

func (c Catalog) ForEcosystem(eco Ecosystem) []ModelEntry {
	var out []ModelEntry
	for _, m := range c.Models {
		if m.Ecosystem == eco {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return tierRank(out[i].Tier) < tierRank(out[j].Tier)
	})
	return out
}

func (c Catalog) ModelForTier(eco Ecosystem, tier Tier, allowPremium bool) (ModelEntry, bool) {
	for _, m := range c.Models {
		if m.Ecosystem != eco || m.Tier != tier {
			continue
		}
		if m.Gated && !allowPremium {
			continue
		}
		return m, true
	}
	// Fall back to nearest lower tier.
	for _, t := range []Tier{TierHeavy, TierStandard, TierLight} {
		if tierRank(t) > tierRank(tier) {
			continue
		}
		if m, ok := c.ModelForTier(eco, t, allowPremium); ok {
			return m, true
		}
	}
	return ModelEntry{}, false
}

func (c Catalog) ResolveID(eco Ecosystem, modelID string) ModelEntry {
	modelID = strings.TrimSpace(modelID)
	for _, m := range c.Models {
		if m.Ecosystem == eco && strings.EqualFold(m.ID, modelID) {
			return m
		}
	}
	for _, m := range c.Models {
		if m.Ecosystem != eco {
			continue
		}
		for _, p := range m.Patterns {
			if p == "" {
				continue
			}
			re, err := regexp.Compile("(?i)" + p)
			if err == nil && re.MatchString(modelID) {
				return m
			}
		}
	}
	return ModelEntry{ID: modelID, Display: modelID, Ecosystem: eco, Tier: TierStandard}
}

func (c Catalog) RefreshBestMatch(eco Ecosystem) Catalog {
	// Bump embedded IDs to latest pattern matches (e.g. gpt-5.7-luna when released).
	next := c
	for i, m := range next.Models {
		if m.Ecosystem != eco {
			continue
		}
		if id := c.latestID(eco, m.Tier); id != "" && id != m.ID {
			next.Models[i].ID = id
		}
	}
	next.Updated = time.Now().UTC()
	return next
}

func (c Catalog) latestID(eco Ecosystem, tier Tier) string {
	// Scan ~/.codex/config.toml and known config for newer model strings.
	if eco == EcoCodex {
		if home, err := os.UserHomeDir(); err == nil {
			if b, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml")); err == nil {
				re := regexp.MustCompile(`(?m)^\s*model\s*=\s*"([^"]+)"`)
				if m := re.FindSubmatch(b); len(m) == 2 {
					id := string(m[1])
					entry := c.ResolveID(eco, id)
					if entry.Tier == tier {
						return id
					}
				}
			}
		}
	}
	for _, m := range c.Models {
		if m.Ecosystem == eco && m.Tier == tier {
			return m.ID
		}
	}
	return ""
}

func tierRank(t Tier) int {
	switch t {
	case TierLight:
		return 1
	case TierStandard:
		return 2
	case TierHeavy:
		return 3
	case TierPremium:
		return 4
	default:
		return 2
	}
}

func catalogPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".picogent")
	if v := os.Getenv("PICOGENT_HOME"); v != "" {
		dir = v
	}
	return filepath.Join(dir, "router-catalog.json"), nil
}

func LoadCachedCatalog() (Catalog, error) {
	path, err := catalogPath()
	if err != nil {
		return Catalog{}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Catalog{}, err
	}
	var c Catalog
	if err := json.Unmarshal(b, &c); err != nil {
		return Catalog{}, err
	}
	return c, nil
}

func SaveCachedCatalog(c Catalog) error {
	path, err := catalogPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// InitCatalog loads cache or uses defaults, optionally refreshing from remote.
func InitCatalog(refreshRemote bool) Catalog {
	c := defaultCatalog()
	if cached, err := LoadCachedCatalog(); err == nil && len(cached.Models) > 0 {
		c = mergeCatalog(c, cached)
	}
	c = c.RefreshBestMatch(EcoCodex)
	c = c.RefreshBestMatch(EcoQuadCode)
	if refreshRemote {
		if remote, err := fetchRemoteCatalog(); err == nil {
			c = mergeCatalog(c, remote)
			_ = SaveCachedCatalog(c)
		}
	}
	catalogMu.Lock()
	catalog = c
	catalogMu.Unlock()
	return c
}

func mergeCatalog(base, over Catalog) Catalog {
	byKey := map[string]ModelEntry{}
	for _, m := range base.Models {
		byKey[string(m.Ecosystem)+"/"+string(m.Tier)] = m
	}
	for _, m := range over.Models {
		key := string(m.Ecosystem) + "/" + string(m.Tier)
		if existing, ok := byKey[key]; ok {
			if m.ID != "" {
				existing.ID = m.ID
			}
			if m.Display != "" {
				existing.Display = m.Display
			}
			if m.Description != "" {
				existing.Description = m.Description
			}
			if len(m.Patterns) > 0 {
				existing.Patterns = m.Patterns
			}
			byKey[key] = existing
		} else {
			byKey[key] = m
		}
	}
	out := base
	out.Version = over.Version
	if !over.Updated.IsZero() {
		out.Updated = over.Updated
	}
	out.SourceURL = over.SourceURL
	out.Models = make([]ModelEntry, 0, len(byKey))
	for _, m := range byKey {
		out.Models = append(out.Models, m)
	}
	sort.Slice(out.Models, func(i, j int) bool {
		if out.Models[i].Ecosystem != out.Models[j].Ecosystem {
			return out.Models[i].Ecosystem < out.Models[j].Ecosystem
		}
		return tierRank(out.Models[i].Tier) < tierRank(out.Models[j].Tier)
	})
	return out
}

func fetchRemoteCatalog() (Catalog, error) {
	// Best-effort remote refresh; falls back silently when offline.
	client := &httpClient{timeout: 8 * time.Second}
	b, err := client.get(catalogRemote)
	if err != nil {
		return Catalog{}, err
	}
	var c Catalog
	if err := json.Unmarshal(b, &c); err != nil {
		return Catalog{}, fmt.Errorf("remote catalog: %w", err)
	}
	c.Updated = time.Now().UTC()
	return c, nil
}

type httpClient struct {
	timeout time.Duration
}

func (h *httpClient) get(url string) ([]byte, error) {
	req, err := newGET(url, h.timeout)
	if err != nil {
		return nil, err
	}
	return doGET(req)
}
