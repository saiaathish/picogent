package claudeauth

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/usage"
)

var usageHTTP = usage.NewHTTPClient()
var usageCache usage.Cache

// ReadUsage reads subscription windows using Claude Code OAuth, never an API key.
func ReadUsage(ctx context.Context) (usage.Snapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	cred, err := LoadContext(ctx)
	if err != nil || cred.AccessToken == "" || cred.Expired() {
		return usage.Snapshot{}, usage.ErrLogin
	}
	return usageCache.Read(ctx, cred.AccessToken+"\x00"+cred.SubscriptionType, func(ctx context.Context) (usage.Snapshot, error) {
		return fetchUsage(ctx, usageHTTP, "https://api.anthropic.com/api/oauth/usage", cred)
	})
}

type usageWindow struct {
	Utilization *float64 `json:"utilization"`
	ResetsAt    string   `json:"resets_at"`
}

func fetchUsage(ctx context.Context, client *http.Client, endpoint string, cred Cred) (usage.Snapshot, error) {
	var payload map[string]json.RawMessage
	if err := usage.JSON(ctx, client, http.MethodGet, endpoint, cred.AccessToken, map[string]string{
		"anthropic-beta": "oauth-2025-04-20", "anthropic-version": "2023-06-01", "User-Agent": "claude-cli/1.0.0 (external, picogent)",
	}, nil, &payload); err != nil {
		return usage.Snapshot{}, err
	}
	value := usage.Snapshot{Plan: cred.SubscriptionType, UpdatedAt: time.Now().UTC(), Groups: []usage.Group{}}
	main := usage.Group{ID: "claude", Name: "Claude Code", Windows: []usage.Window{}}
	window := func(raw json.RawMessage, name string, seconds int64) *usage.Window {
		var w *usageWindow
		if json.Unmarshal(raw, &w) != nil || w == nil {
			return nil
		}
		return &usage.Window{Name: name, RemainingPercent: usage.UsedPercent(w.Utilization), WindowSeconds: &seconds, ResetsAt: usage.ResetTime(w.ResetsAt)}
	}
	if w := window(payload["five_hour"], "Session", 18000); w != nil {
		main.Windows = append(main.Windows, *w)
	}
	if w := window(payload["seven_day"], "Weekly", 604800); w != nil {
		main.Windows = append(main.Windows, *w)
	}
	if len(main.Windows) > 0 {
		value.Groups = append(value.Groups, main)
	}
	keys := []string{}
	for key := range payload {
		if strings.HasPrefix(key, "seven_day_") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	known := map[string]bool{}
	for _, key := range keys {
		if len(value.Groups) >= 32 {
			break
		}
		label := strings.ReplaceAll(strings.TrimPrefix(key, "seven_day_"), "_", " ")
		if label == "" {
			continue
		}
		label = strings.ToUpper(label[:1]) + label[1:]
		if w := window(payload[key], "Weekly", 604800); w != nil {
			value.Groups = append(value.Groups, usage.Group{ID: key, Name: label, Windows: []usage.Window{*w}})
			known[strings.ToLower(label)] = true
		}
	}
	// Newer accounts can report model-scoped limits separately from the legacy
	// seven_day_* windows. Paid extra_usage billing data is deliberately omitted.
	var limits []struct {
		Percent  *float64 `json:"percent"`
		Active   *bool    `json:"is_active"`
		ResetsAt string   `json:"resets_at"`
		Scope    struct {
			Model struct {
				ID   string `json:"id"`
				Name string `json:"display_name"`
			} `json:"model"`
		} `json:"scope"`
	}
	_ = json.Unmarshal(payload["limits"], &limits)
	for _, l := range limits {
		if len(value.Groups) >= 32 {
			break
		}
		if l.Active != nil && !*l.Active {
			continue
		}
		label := l.Scope.Model.Name
		if label == "" {
			label = l.Scope.Model.ID
		}
		if label == "" || known[strings.ToLower(label)] {
			continue
		}
		known[strings.ToLower(label)] = true
		value.Groups = append(value.Groups, usage.Group{ID: l.Scope.Model.ID, Name: label, Windows: []usage.Window{{Name: "Model allowance", RemainingPercent: usage.UsedPercent(l.Percent), ResetsAt: usage.ResetTime(l.ResetsAt)}}})
	}
	if len(value.Groups) == 0 {
		return usage.Snapshot{}, usage.ErrUnavailable
	}
	return value, nil
}
