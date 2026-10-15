package agyauth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type ConnectionCheck struct {
	ID     string `json:"id"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}
type ConnectionReport struct {
	OK         bool              `json:"ok"`
	Checks     []ConnectionCheck `json:"checks"`
	ModelCount int               `json:"model_count"`
	CheckedAt  time.Time         `json:"checked_at"`
}

// CheckConnection checks subscription metadata only: no inference, onboarding,
// API keys, token refresh, or paid-credit fallback. Tokens never leave this adapter.
func CheckConnection(ctx context.Context) ConnectionReport {
	ctx, cancel := context.WithTimeout(ctx, 18*time.Second)
	defer cancel()
	report := ConnectionReport{CheckedAt: time.Now().UTC(), Checks: []ConnectionCheck{}}
	add := func(id string, ok bool, detail string) {
		report.Checks = append(report.Checks, ConnectionCheck{ID: id, OK: ok, Detail: detail})
	}
	if UsesGeminiAPIKey() {
		add("credentials", false, "Gemini API-key mode is selected. This checker only verifies an Antigravity subscription; no API request was made.")
		return report
	}
	cred, err := LoadContext(ctx)
	if err != nil {
		add("credentials", false, "No readable Antigravity login found. Use Log in, finish signing in, then check again.")
		return report
	}
	if cred.Expired() {
		add("credentials", false, "The saved Antigravity access token has expired. Reconnect in agy, then check again.")
		return report
	}
	add("credentials", true, "Current Antigravity credentials found.")
	checkHTTP := func(endpoint string, body any, out any) bool {
		code, err := connectionJSON(ctx, usageHTTP, "https://cloudcode-pa.googleapis.com/v1internal:"+endpoint, cred.AccessToken, body, out)
		if err == nil {
			return true
		}
		detail := "Google could not be reached. Check your network and try again."
		switch {
		case errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil:
			detail = "Connection check timed out. Try again."
		case code == 401:
			detail = "Google rejected the login (HTTP 401). Sign in again."
		case code == 403:
			detail = "Google denied this account access (HTTP 403). Check Antigravity account eligibility and permissions."
		case code == 429:
			detail = "Google is rate limiting account checks (HTTP 429). Wait and retry."
		case code >= 300:
			detail = fmt.Sprintf("Google's account/model endpoint returned HTTP %d. Try again later.", code)
		case code == 200:
			detail = "Google returned an unexpected account/model response. Connection could not be verified."
		}
		add(endpoint, false, detail)
		return false
	}
	var account struct {
		Project json.RawMessage `json:"cloudaicompanionProject"`
	}
	body := map[string]any{"metadata": map[string]string{"ideType": "ANTIGRAVITY", "platform": "PLATFORM_UNSPECIFIED", "pluginType": "GEMINI"}}
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if project != "" {
		body["cloudaicompanionProject"] = project
	}
	if !checkHTTP("loadCodeAssist", body, &account) {
		return report
	}
	add("account", true, "Google accepted the login and account-status request.")
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
	request := map[string]string{}
	if project != "" {
		request["project"] = project
	}
	var models struct {
		Models map[string]json.RawMessage `json:"models"`
	}
	if !checkHTTP("fetchAvailableModels", request, &models) {
		return report
	}
	if len(models.Models) == 0 {
		add("models", false, "Login accepted, but Google returned no available models. Check your account access in agy.")
		return report
	}
	report.ModelCount = len(models.Models)
	add("models", true, "Model discovery succeeded. No content was generated or AI credits consumed.")
	report.OK = true
	return report
}

func connectionJSON(ctx context.Context, client *http.Client, endpoint, token string, body, out any) (int, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "antigravity")
	response, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return response.StatusCode, errors.New("account check rejected")
	}
	err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(out)
	return response.StatusCode, err
}
