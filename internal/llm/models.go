package llm

import "github.com/saiaathish/picogent/internal/config"

// ModelChoice is one option in the settings/setup model dropdown.
type ModelChoice struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Gated       bool   `json:"gated,omitempty"`
}

// ModelChoices returns Auto plus eligible models for an ecosystem.
func ModelChoices(eco Ecosystem, includeFable bool) []ModelChoice {
	out := []ModelChoice{{
		Value:       config.ModelAuto,
		Label:       "Auto",
		Description: autoBlurb(eco),
	}}
	for _, m := range CatalogSnapshot().ForEcosystem(eco) {
		if m.Gated && !includeFable {
			continue
		}
		out = append(out, ModelChoice{
			Value:       m.ID,
			Label:       m.Display,
			Description: m.Description,
			Gated:       m.Gated,
		})
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
