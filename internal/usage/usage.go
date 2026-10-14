// Package usage contains provider-independent, read-only account quota data.
package usage

import (
	"bytes"
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

var ErrLogin = errors.New("provider login is missing or expired")
var ErrUnavailable = errors.New("usage limits are unavailable")

type Snapshot struct {
	Plan      string    `json:"plan,omitempty"`
	Groups    []Group   `json:"groups"`
	UpdatedAt time.Time `json:"updated_at"`
}
type Group struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Allowed      *bool    `json:"allowed,omitempty"`
	LimitReached *bool    `json:"limit_reached,omitempty"`
	Windows      []Window `json:"windows"`
}
type Window struct {
	Name             string   `json:"name"`
	RemainingPercent *float64 `json:"remaining_percent"`
	WindowSeconds    *int64   `json:"window_seconds"`
	ResetsAt         *int64   `json:"resets_at"`
}

func Percent(v *float64) *float64 {
	if v == nil || math.IsNaN(*v) || math.IsInf(*v, 0) {
		return nil
	}
	n := math.Max(0, math.Min(100, *v))
	return &n
}
func UsedPercent(v *float64) *float64 {
	if v == nil {
		return nil
	}
	n := 100 - *v
	return Percent(&n)
}
func ResetTime(s string) *int64 {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil || t.Unix() <= 0 {
		return nil
	}
	n := t.Unix()
	return &n
}

func NewHTTPClient() *http.Client {
	return &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// JSON issues only the adapter's fixed quota/status requests. No provider body,
// bearer token, or transport error is returned to the UI.
func JSON(ctx context.Context, client *http.Client, method, endpoint, token string, headers map[string]string, body any, out any) error {
	var b io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return ErrUnavailable
		}
		b = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, b)
	if err != nil {
		return ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := client.Do(req)
	if err != nil {
		return ErrUnavailable
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return ErrLogin
	}
	if res.StatusCode != http.StatusOK {
		return ErrUnavailable
	}
	if json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(out) != nil {
		return ErrUnavailable
	}
	return nil
}

type Cache struct {
	mu       sync.Mutex
	identity [32]byte
	until    time.Time
	value    Snapshot
	err      error
}

func (c *Cache) Read(ctx context.Context, identity string, fetch func(context.Context) (Snapshot, error)) (Snapshot, error) {
	key := sha256.Sum256([]byte(identity))
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.identity == key && time.Now().Before(c.until) {
		return c.value, c.err
	}
	value, err := fetch(ctx)
	if ctx.Err() == nil {
		c.identity, c.value, c.err, c.until = key, value, err, time.Now().Add(time.Minute)
	}
	return value, err
}
