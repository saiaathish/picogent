package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/saiaathish/picogent/internal/agyauth"
	"github.com/saiaathish/picogent/internal/opencodeauth"
)

// Dynamic (CLI-discovered) models, separate from the router catalog tiers.
var (
	cliMu     sync.RWMutex
	cliByEco  = map[Ecosystem][]ModelEntry{}
	cliFetched time.Time
)

const (
	EcoOpenCode     Ecosystem = "opencode"
	EcoOpenCodeGo   Ecosystem = "opencode-go"
	EcoAntigravity Ecosystem = "antigravity"
)

// CLIModels returns last discovered models for an ecosystem.
func CLIModels(eco Ecosystem) []ModelEntry {
	cliMu.RLock()
	defer cliMu.RUnlock()
	return append([]ModelEntry(nil), cliByEco[eco]...)
}

// SetCLIModels replaces the dynamic list for an ecosystem (tests / refresh).
func SetCLIModels(eco Ecosystem, models []ModelEntry) {
	cliMu.Lock()
	defer cliMu.Unlock()
	cliByEco[eco] = append([]ModelEntry(nil), models...)
	cliFetched = time.Now().UTC()
}

// RefreshCLIModels pulls live model lists from installed CLIs and public Zen/Go catalogs.
// Safe to call on every model-picker load; skips if refreshed within ttl.
func RefreshCLIModels(force bool) {
	const ttl = 60 * time.Second
	cliMu.RLock()
	fresh := !force && time.Since(cliFetched) < ttl && len(cliByEco) > 0
	cliMu.RUnlock()
	if fresh {
		return
	}

	var wg sync.WaitGroup
	type result struct {
		eco Ecosystem
		ms  []ModelEntry
	}
	ch := make(chan result, 4)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if ms := discoverOpenCodeModels(opencodeauth.ProviderZen, EcoOpenCode); len(ms) > 0 {
			ch <- result{EcoOpenCode, ms}
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		if ms := discoverOpenCodeModels(opencodeauth.ProviderGo, EcoOpenCodeGo); len(ms) > 0 {
			ch <- result{EcoOpenCodeGo, ms}
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		if ms := discoverAntigravityModels(); len(ms) > 0 {
			ch <- result{EcoAntigravity, ms}
		}
	}()
	go func() {
		wg.Wait()
		close(ch)
	}()

	cliMu.Lock()
	defer cliMu.Unlock()
	for r := range ch {
		cliByEco[r.eco] = r.ms
	}
	cliFetched = time.Now().UTC()
}

func discoverOpenCodeModels(provider string, eco Ecosystem) []ModelEntry {
	// Prefer `opencode models <provider>` when CLI is present.
	if bin, err := exec.LookPath("opencode"); err == nil {
		cmd := exec.Command(bin, "models", provider)
		out, err := cmd.CombinedOutput()
		if err == nil {
			var ms []ModelEntry
			for _, line := range strings.Split(string(out), "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				id := line
				display := line
				if i := strings.IndexByte(line, '/'); i >= 0 {
					id = line // keep provider/model as value
					display = prettyModelName(line[i+1:])
				}
				ms = append(ms, ModelEntry{
					ID:          id,
					Display:     display,
					Ecosystem:   eco,
					Tier:        guessTier(line),
					Description: "via OpenCode CLI",
				})
			}
			if len(ms) > 0 {
				return ms
			}
		}
	}
	// Fallback: public models endpoint (no auth required for listing).
	url := opencodeauth.ZenBaseURL + "/models"
	if provider == opencodeauth.ProviderGo {
		url = opencodeauth.GoBaseURL + "/models"
	}
	client := &httpClient{timeout: 8 * time.Second}
	b, err := client.get(url)
	if err != nil {
		return nil
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.Unmarshal(b, &payload) != nil {
		return nil
	}
	prefix := provider + "/"
	var ms []ModelEntry
	for _, m := range payload.Data {
		if m.ID == "" {
			continue
		}
		ms = append(ms, ModelEntry{
			ID:          prefix + m.ID,
			Display:     prettyModelName(m.ID),
			Ecosystem:   eco,
			Tier:        guessTier(m.ID),
			Description: "via OpenCode " + provider,
		})
	}
	return ms
}

func discoverAntigravityModels() []ModelEntry {
	models, err := agyauth.ListModels()
	if err != nil {
		// Static fallback from docs when CLI missing / offline.
		return []ModelEntry{
			{ID: "gemini-3.7-flash-high", Display: "Gemini 3.7 Flash (High)", Ecosystem: EcoAntigravity, Tier: TierLight},
			{ID: "gemini-3.5-flash-medium", Display: "Gemini 3.5 Flash (Medium)", Ecosystem: EcoAntigravity, Tier: TierStandard},
			{ID: "gemini-3.1-pro-high", Display: "Gemini 3.1 Pro (High)", Ecosystem: EcoAntigravity, Tier: TierHeavy},
			{ID: "claude-sonnet-4-6", Display: "Claude Sonnet 4.6", Ecosystem: EcoAntigravity, Tier: TierStandard},
			{ID: "claude-opus-4-6-thinking", Display: "Claude Opus 4.6 Thinking", Ecosystem: EcoAntigravity, Tier: TierHeavy},
			{ID: "gpt-oss-120b-medium", Display: "GPT-OSS 120B", Ecosystem: EcoAntigravity, Tier: TierStandard},
		}
	}
	var ms []ModelEntry
	for _, m := range models {
		label := m.Label
		if label == "" {
			label = prettyModelName(m.ID)
		}
		ms = append(ms, ModelEntry{
			ID:          m.ID,
			Display:     label,
			Ecosystem:   EcoAntigravity,
			Tier:        guessTier(m.ID),
			Description: "via Antigravity CLI",
		})
	}
	return ms
}

func prettyModelName(id string) string {
	id = strings.TrimSpace(id)
	parts := strings.Split(id, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

func guessTier(id string) Tier {
	s := strings.ToLower(id)
	switch {
	case strings.Contains(s, "flash") && (strings.Contains(s, "lite") || strings.Contains(s, "low") || strings.Contains(s, "nano") || strings.Contains(s, "luna") || strings.Contains(s, "haiku") || strings.Contains(s, "free")):
		return TierLight
	case strings.Contains(s, "opus") || strings.Contains(s, "sol") || strings.Contains(s, "pro-high") || strings.Contains(s, "fable") || strings.Contains(s, "ultra"):
		return TierHeavy
	case strings.Contains(s, "flash") || strings.Contains(s, "mini") || strings.Contains(s, "haiku") || strings.Contains(s, "luna"):
		return TierLight
	default:
		return TierStandard
	}
}

// OpenCode is a Zen/Go gateway client. It routes to chat/completions, Messages, or Responses.
type OpenCode struct {
	Timeout time.Duration
	HTTP    *http.Client
}

func NewOpenCode(timeout time.Duration) *OpenCode {
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	return &OpenCode{
		Timeout: timeout,
		HTTP:    &http.Client{Timeout: timeout},
	}
}

func (c *OpenCode) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	model := req.Model
	if model == "" || IsAutoModel(model) {
		model = opencodeauth.DefaultModel()
	}
	_, base, key, bare, err := opencodeauth.KeyForModel(model)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("OpenCode: %w\nFix:     run `opencode auth login` (Zen or Go), or picogent login opencode", err)
	}
	req.Model = bare
	switch openCodeAPIStyle(bare) {
	case "messages":
		return c.chatMessages(ctx, base, key, req)
	case "responses":
		return c.chatResponses(ctx, base, key, req)
	default:
		oa := NewOpenAI(base, key, bare, c.Timeout)
		oa.HTTP = c.HTTP
		return oa.Chat(ctx, req)
	}
}

func openCodeAPIStyle(model string) string {
	m := strings.ToLower(model)
	switch {
	case strings.HasPrefix(m, "claude-"), strings.HasPrefix(m, "qwen"), strings.HasPrefix(m, "minimax"):
		return "messages"
	case strings.HasPrefix(m, "gpt-"), strings.HasPrefix(m, "grok-"), strings.HasPrefix(m, "muse-"):
		return "responses"
	default:
		return "chat"
	}
}

func (c *OpenCode) chatMessages(ctx context.Context, base, key string, req ChatRequest) (ChatResponse, error) {
	a := NewAnthropic("", req.Model, c.Timeout)
	a.BaseURL = strings.TrimRight(base, "/") + "/messages"
	a.HTTP = c.HTTP
	a.BearerKey = key
	return a.Chat(ctx, req)
}

func (c *OpenCode) chatResponses(ctx context.Context, base, key string, req ChatRequest) (ChatResponse, error) {
	return openAIResponsesChat(ctx, c.HTTP, strings.TrimRight(base, "/")+"/responses", key, req)
}
