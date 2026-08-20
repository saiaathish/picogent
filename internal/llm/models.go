package llm

import (
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/opencodeauth"
)

// ModelChoice is one option in the settings/setup model dropdown.
type ModelChoice struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Gated       bool   `json:"gated,omitempty"`
}

// SupportsAutoRouter is true only for Codex and Claude Code — curated catalogs
// where Auto + advisor/reasoning routing is the product default. OpenCode and
// Antigravity expose large model menus; users pick a concrete model.
func SupportsAutoRouter(eco Ecosystem) bool {
	return eco == EcoCodex || eco == EcoQuadCode
}

// ModelChoices returns eligible models for an ecosystem.
// Auto is prepended only for Codex and Claude Code.
// Includes CLI-discovered models (OpenCode Zen/Go, Antigravity) when present.
// For OpenCode, only lists Zen and/or Go models for plans the user is logged into.
func ModelChoices(eco Ecosystem, includeFable bool) []ModelChoice {
	var out []ModelChoice
	seen := map[string]bool{}
	if SupportsAutoRouter(eco) {
		out = append(out, ModelChoice{
			Value:       config.ModelAuto,
			Label:       "Auto",
			Description: autoBlurb(eco),
		})
		seen[config.ModelAuto] = true
	}

	add := func(m ModelEntry) {
		if m.ID == "" || seen[m.ID] {
			return
		}
		if m.Gated && !includeFable {
			return
		}
		seen[m.ID] = true
		label := m.Display
		if label == "" {
			label = m.ID
		}
		out = append(out, ModelChoice{
			Value:       m.ID,
			Label:       label,
			Description: m.Description,
			Gated:       m.Gated,
		})
	}

	includeZen := eco == EcoOpenCode && (opencodeauth.ZenLoggedIn() || !opencodeauth.LoggedIn())
	includeGo := (eco == EcoOpenCode || eco == EcoOpenCodeGo) && (opencodeauth.GoLoggedIn() || !opencodeauth.LoggedIn())
	// When at least one plan is logged in, hide the other plan's models.
	if opencodeauth.LoggedIn() {
		includeZen = eco == EcoOpenCode && opencodeauth.ZenLoggedIn()
		includeGo = (eco == EcoOpenCode || eco == EcoOpenCodeGo) && opencodeauth.GoLoggedIn()
	}

	switch eco {
	case EcoOpenCode:
		if includeZen {
			for _, m := range CatalogSnapshot().ForEcosystem(EcoOpenCode) {
				add(m)
			}
			for _, m := range CLIModels(EcoOpenCode) {
				add(m)
			}
		}
		if includeGo {
			for _, m := range CatalogSnapshot().ForEcosystem(EcoOpenCodeGo) {
				add(m)
			}
			for _, m := range CLIModels(EcoOpenCodeGo) {
				add(m)
			}
		}
	default:
		for _, m := range CatalogSnapshot().ForEcosystem(eco) {
			add(m)
		}
		for _, m := range CLIModels(eco) {
			add(m)
		}
	}
	return out
}

func autoBlurb(eco Ecosystem) string {
	switch eco {
	case EcoQuadCode:
		return "Picogent picks Haiku, Sonnet, or Opus per task"
	default:
		return "Picogent picks Luna, Terra, or Sol per task"
	}
}
