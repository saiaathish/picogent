package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/agyauth"
)

const (
	agyDailyURL = "https://daily-cloudcode-pa.googleapis.com"
	agyProdURL  = "https://cloudcode-pa.googleapis.com"
	geminiOAURL = "https://generativelanguage.googleapis.com/v1beta/openai"
)

// Antigravity talks to Google's Cloud Code PA gateway (subscription) or
// Gemini OpenAI-compat when GEMINI_API_KEY is set.
type Antigravity struct {
	Model   string
	Timeout time.Duration
	HTTP    *http.Client
	BaseURL string // override for tests
	Project string
}

func NewAntigravity(model string, timeout time.Duration) *Antigravity {
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	if model == "" || IsAutoModel(model) {
		model = agyauth.DefaultModel()
	}
	return &Antigravity{
		Model:   model,
		Timeout: timeout,
		HTTP:    &http.Client{Timeout: timeout},
		BaseURL: agyDailyURL,
	}
}

func (c *Antigravity) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	model := req.Model
	if model == "" || IsAutoModel(model) {
		model = c.Model
	}
	if key := agyauth.GeminiAPIKey(); key != "" && agyauth.UsesGeminiAPIKey() {
		oa := NewOpenAI(geminiOAURL, key, normalizeGeminiModel(model), c.Timeout)
		oa.HTTP = c.HTTP
		req.Model = normalizeGeminiModel(model)
		return oa.Chat(ctx, req)
	}
	tok, err := agyauth.Token()
	if err != nil {
		return ChatResponse{}, fmt.Errorf("Antigravity: %w\nFix:     run `agy` to log in, or set GEMINI_API_KEY (see Antigravity CLI docs)", err)
	}
	project := c.Project
	if project == "" {
		project = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}
	if project == "" {
		project = "default"
	}
	body := map[string]any{
		"project":   project,
		"model":     model,
		"userAgent": "antigravity",
		"requestId": fmt.Sprintf("picogent-%d", time.Now().UnixNano()),
		"request": map[string]any{
			"contents":          toAgyContents(req.Messages),
			"systemInstruction": agySystem(req.Messages),
			"tools":             toAgyTools(req.Tools),
			"generationConfig": map[string]any{
				"maxOutputTokens": 8192,
				"temperature":     0.4,
			},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return ChatResponse{}, err
	}
	bases := []string{c.BaseURL, agyDailyURL, agyProdURL}
	var lastErr error
	for _, base := range bases {
		if base == "" {
			continue
		}
		out, err := c.generateOnce(ctx, strings.TrimRight(base, "/")+"/v1internal:generateContent", tok, raw)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if !isAgyRetryable(err) {
			return ChatResponse{}, err
		}
	}
	return ChatResponse{}, lastErr
}

func (c *Antigravity) generateOnce(ctx context.Context, url, tok string, raw []byte) (ChatResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+tok)
	httpReq.Header.Set("User-Agent", "antigravity/1.1.17 "+runtime.GOOS+"/"+runtime.GOARCH)
	httpReq.Header.Set("X-Goog-Api-Client", "google-cloud-sdk vscode_cloudshelleditor/0.1")
	meta, _ := json.Marshal(map[string]string{
		"ideType":    "ANTIGRAVITY",
		"platform":   strings.ToUpper(runtime.GOOS),
		"pluginType": "GEMINI",
	})
	httpReq.Header.Set("Client-Metadata", string(meta))

	res, err := c.HTTP.Do(httpReq)
	if err != nil {
		return ChatResponse{}, err
	}
	defer res.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if res.StatusCode >= 400 {
		return ChatResponse{}, fmt.Errorf("antigravity http %d: %s", res.StatusCode, truncate(string(payload), 500))
	}
	var parsed struct {
		Response struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text         string `json:"text"`
						FunctionCall *struct {
							Name string         `json:"name"`
							Args map[string]any `json:"args"`
							ID   string         `json:"id"`
						} `json:"functionCall"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
			UsageMetadata struct {
				PromptTokenCount     int `json:"promptTokenCount"`
				CandidatesTokenCount int `json:"candidatesTokenCount"`
			} `json:"usageMetadata"`
		} `json:"response"`
		Error *struct {
			Message string `json:"message"`
			Code    int    `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return ChatResponse{}, fmt.Errorf("antigravity: bad json: %s", truncate(string(payload), 300))
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return ChatResponse{}, fmt.Errorf("antigravity: %s", parsed.Error.Message)
	}
	msg := Message{Role: "assistant"}
	if len(parsed.Response.Candidates) > 0 {
		for _, p := range parsed.Response.Candidates[0].Content.Parts {
			if p.FunctionCall != nil {
				args, _ := json.Marshal(p.FunctionCall.Args)
				id := p.FunctionCall.ID
				if id == "" {
					id = "call_" + p.FunctionCall.Name
				}
				msg.ToolCalls = append(msg.ToolCalls, ToolCall{
					ID:        id,
					Name:      p.FunctionCall.Name,
					Arguments: string(args),
				})
				continue
			}
			if p.Text != "" {
				if msg.Content != "" {
					msg.Content += "\n"
				}
				msg.Content += p.Text
			}
		}
	}
	return ChatResponse{
		Message:          msg,
		PromptTokens:     parsed.Response.UsageMetadata.PromptTokenCount,
		CompletionTokens: parsed.Response.UsageMetadata.CandidatesTokenCount,
	}, nil
}

func isAgyRetryable(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "http 404") || strings.Contains(s, "http 503") || strings.Contains(s, "http 502")
}

func normalizeGeminiModel(id string) string {
	// agy slugs like gemini-3.5-flash-medium → gemini-2.0-flash for API key path when unknown.
	id = strings.TrimSpace(id)
	if strings.HasPrefix(id, "gemini-") {
		// Strip effort suffix for Google AI Studio ids when present.
		for _, suf := range []string{"-high", "-medium", "-low"} {
			if strings.HasSuffix(id, suf) {
				return strings.TrimSuffix(id, suf)
			}
		}
		return id
	}
	return id
}

func agySystem(msgs []Message) any {
	var b strings.Builder
	for _, m := range msgs {
		if m.Role == "system" && m.Content != "" {
			if b.Len() > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString(m.Content)
		}
	}
	if b.Len() == 0 {
		return nil
	}
	return map[string]any{
		"parts": []map[string]any{{"text": b.String()}},
	}
}

func toAgyContents(msgs []Message) []map[string]any {
	var out []map[string]any
	for _, m := range msgs {
		switch m.Role {
		case "system":
			continue
		case "user":
			out = append(out, map[string]any{
				"role":  "user",
				"parts": []map[string]any{{"text": m.Content}},
			})
		case "assistant":
			parts := []map[string]any{}
			if strings.TrimSpace(m.Content) != "" {
				parts = append(parts, map[string]any{"text": m.Content})
			}
			for _, tc := range m.ToolCalls {
				var args any
				_ = json.Unmarshal([]byte(tc.Arguments), &args)
				parts = append(parts, map[string]any{
					"functionCall": map[string]any{
						"name": tc.Name,
						"args": args,
						"id":   tc.ID,
					},
				})
			}
			if len(parts) == 0 {
				parts = append(parts, map[string]any{"text": ""})
			}
			out = append(out, map[string]any{"role": "model", "parts": parts})
		case "tool":
			out = append(out, map[string]any{
				"role": "user",
				"parts": []map[string]any{{
					"functionResponse": map[string]any{
						"name":     m.Name,
						"id":       m.ToolCallID,
						"response": map[string]any{"result": m.Content},
					},
				}},
			})
		}
	}
	return out
}

func toAgyTools(tools []ToolSpec) []map[string]any {
	if len(tools) == 0 {
		return nil
	}
	decls := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		params := t.Parameters
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		decls = append(decls, map[string]any{
			"name":        sanitizeAgyToolName(t.Name),
			"description": t.Description,
			"parameters":   params,
		})
	}
	return []map[string]any{{"functionDeclarations": decls}}
}

func sanitizeAgyToolName(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	if name == "" {
		return "tool"
	}
	r := name[0]
	if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && r != '_' {
		return "t_" + name
	}
	return name
}
