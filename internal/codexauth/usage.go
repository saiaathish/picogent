package codexauth

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"sync"
	"time"
)

var (
	ErrUsageLogin       = errors.New("sign in to Codex to see usage limits")
	ErrUsageUnavailable = errors.New("Codex usage limits are unavailable right now")
)

// Usage is account-wide subscription quota, not the current chat's token count.
type Usage struct {
	Plan      string       `json:"plan,omitempty"`
	Groups    []UsageGroup `json:"groups"`
	UpdatedAt time.Time    `json:"updated_at"`
}

type UsageGroup struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Allowed      *bool         `json:"allowed,omitempty"`
	LimitReached *bool         `json:"limit_reached,omitempty"`
	Windows      []UsageWindow `json:"windows"`
}

type UsageWindow struct {
	Name             string   `json:"name"`
	RemainingPercent *float64 `json:"remaining_percent"`
	WindowSeconds    *int64   `json:"window_seconds"`
	ResetsAt         *int64   `json:"resets_at"`
}

type quotaWindow struct {
	UsedPercent *float64 `json:"used_percent"`
	Seconds     *int64   `json:"limit_window_seconds"`
	ResetAt     *int64   `json:"reset_at"`
}

type quota struct {
	Allowed      *bool        `json:"allowed"`
	LimitReached *bool        `json:"limit_reached"`
	Primary      *quotaWindow `json:"primary_window"`
	Secondary    *quotaWindow `json:"secondary_window"`
}

type usagePayload struct {
	Plan       string `json:"plan_type"`
	Quota      *quota `json:"rate_limit"`
	Additional []struct {
		ID    string `json:"metered_feature"`
		Name  string `json:"limit_name"`
		Quota *quota `json:"rate_limit"`
	} `json:"additional_rate_limits"`
}

var usageHTTP = &http.Client{
	Timeout: 10 * time.Second,
	// Never forward the account bearer token to a redirected host.
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

var usageCache struct {
	sync.Mutex
	Identity [32]byte
	Until    time.Time
	Value    Usage
	Err      error
}

// ReadUsage uses the existing Codex login and a read-only quota request. It
// never starts inference or consumes reset credits. All tabs share a one-minute
// cache, keyed to the credentials so a changed login cannot reuse old limits.
func ReadUsage(ctx context.Context) (Usage, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	access, account, err := Token(ctx)
	if err != nil {
		return Usage{}, ErrUsageLogin
	}
	identity := sha256.Sum256([]byte(account + "\x00" + access))
	usageCache.Lock()
	defer usageCache.Unlock()
	if usageCache.Identity == identity && time.Now().Before(usageCache.Until) {
		return usageCache.Value, usageCache.Err
	}
	value, err := fetchUsage(ctx, usageHTTP, "https://chatgpt.com/backend-api/wham/usage", access, account)
	if ctx.Err() == nil {
		usageCache.Identity, usageCache.Value, usageCache.Err = identity, value, err
		usageCache.Until = time.Now().Add(time.Minute)
	}
	return value, err
}

func fetchUsage(ctx context.Context, client *http.Client, endpoint, access, account string) (Usage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Usage{}, ErrUsageUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("originator", Originator())
	if account != "" {
		req.Header.Set("ChatGPT-Account-ID", account)
	}
	res, err := client.Do(req)
	if err != nil {
		return Usage{}, ErrUsageUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return Usage{}, ErrUsageLogin
	}
	if res.StatusCode != http.StatusOK {
		return Usage{}, ErrUsageUnavailable
	}
	var payload usagePayload
	if json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&payload) != nil {
		return Usage{}, ErrUsageUnavailable
	}
	if payload.Plan == "" && payload.Quota == nil && len(payload.Additional) == 0 {
		return Usage{}, ErrUsageUnavailable
	}
	value := Usage{Plan: payload.Plan, UpdatedAt: time.Now().UTC(), Groups: []UsageGroup{usageGroup("codex", "Codex", payload.Quota)}}
	for i, extra := range payload.Additional {
		if i >= 32 {
			break
		}
		name := extra.Name
		if name == "" {
			name = extra.ID
		}
		if name == "" {
			name = "Additional allowance"
		}
		value.Groups = append(value.Groups, usageGroup(extra.ID, name, extra.Quota))
	}
	return value, nil
}

func usageGroup(id, name string, q *quota) UsageGroup {
	g := UsageGroup{ID: id, Name: name, Windows: []UsageWindow{}}
	if q == nil {
		return g
	}
	g.Allowed, g.LimitReached = q.Allowed, q.LimitReached
	for i, w := range []*quotaWindow{q.Primary, q.Secondary} {
		if w == nil {
			continue
		}
		names := []string{"Primary", "Secondary"}
		window := UsageWindow{Name: names[i], WindowSeconds: w.Seconds, ResetsAt: w.ResetAt}
		if w.UsedPercent != nil && !math.IsNaN(*w.UsedPercent) && !math.IsInf(*w.UsedPercent, 0) {
			remaining := math.Max(0, math.Min(100, 100-*w.UsedPercent))
			window.RemainingPercent = &remaining
		}
		g.Windows = append(g.Windows, window)
	}
	return g
}
