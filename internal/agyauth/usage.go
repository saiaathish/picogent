package agyauth

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/usage"
)

var usageHTTP = usage.NewHTTPClient()
var usageCache usage.Cache

// ReadUsage asks only for account status and available-model quotas. It never
// onboards a project, generates content, or enables/consumes AI credits.
func ReadUsage(ctx context.Context) (usage.Snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	if UsesGeminiAPIKey() {
		return usage.Snapshot{}, usage.ErrUnavailable
	}
	cred, err := LoadContext(ctx)
	if err != nil || cred.AccessToken == "" || cred.Expired() {
		return usage.Snapshot{}, usage.ErrLogin
	}
	project := strings.TrimSpace(os.Getenv("GOOGLE_CLOUD_PROJECT"))
	return usageCache.Read(ctx, cred.AccessToken+"\x00"+project, func(ctx context.Context) (usage.Snapshot, error) {
		return fetchUsage(ctx, usageHTTP, "https://cloudcode-pa.googleapis.com", cred.AccessToken, project)
	})
}

func fetchUsage(ctx context.Context, client *http.Client, base, token, project string) (usage.Snapshot, error) {
	headers := map[string]string{"User-Agent": "antigravity"}
	var account struct {
		Project     json.RawMessage `json:"cloudaicompanionProject"`
		CurrentTier struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"currentTier"`
		PaidTier struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"paidTier"`
	}
	body := map[string]any{"metadata": map[string]string{"ideType": "ANTIGRAVITY", "platform": "PLATFORM_UNSPECIFIED", "pluginType": "GEMINI"}}
	if project != "" {
		body["cloudaicompanionProject"] = project
	}
	if err := usage.JSON(ctx, client, http.MethodPost, base+"/v1internal:loadCodeAssist", token, headers, body, &account); err != nil {
		return usage.Snapshot{}, err
	}
	if project == "" {
		_ = json.Unmarshal(account.Project, &project)
		if project == "" {
			var p struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(account.Project, &p)
			project = p.ID
		}
	}
	var payload struct {
		Models map[string]struct {
			Name  string `json:"displayName"`
			Quota *struct {
				Remaining *float64 `json:"remainingFraction"`
				Reset     string   `json:"resetTime"`
				Exhausted *bool    `json:"isExhausted"`
			} `json:"quotaInfo"`
		} `json:"models"`
	}
	request := map[string]string{}
	if project != "" {
		request["project"] = project
	}
	if err := usage.JSON(ctx, client, http.MethodPost, base+"/v1internal:fetchAvailableModels", token, headers, request, &payload); err != nil {
		return usage.Snapshot{}, err
	}
	plan := account.PaidTier.Name
	if plan == "" {
		plan = account.CurrentTier.Name
	}
	if plan == "" {
		plan = account.CurrentTier.ID
	}
	value := usage.Snapshot{Plan: plan, UpdatedAt: time.Now().UTC(), Groups: []usage.Group{}}
	ids := []string{}
	for id := range payload.Models {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if len(value.Groups) >= 64 {
			break
		}
		model := payload.Models[id]
		if model.Quota == nil {
			continue
		}
		name := model.Name
		if name == "" {
			name = id
		}
		var remaining *float64
		if model.Quota.Remaining != nil {
			percent := 100 * *model.Quota.Remaining
			remaining = usage.Percent(&percent)
		}
		// The model API supplies a reset time, but not the quota window length.
		// Do not guess five-hour/weekly labels from the user's tier.
		value.Groups = append(value.Groups, usage.Group{ID: id, Name: name, LimitReached: model.Quota.Exhausted, Windows: []usage.Window{{Name: "Model quota", RemainingPercent: remaining, ResetsAt: usage.ResetTime(model.Quota.Reset)}}})
	}
	if len(value.Groups) == 0 {
		return usage.Snapshot{}, usage.ErrUnavailable
	}
	return value, nil
}
