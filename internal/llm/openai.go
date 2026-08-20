package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OpenAI struct {
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
	HTTP    *http.Client
}

func NewOpenAI(baseURL, apiKey, model string, timeout time.Duration) *OpenAI {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &OpenAI{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Model:   model,
		Timeout: timeout,
		HTTP:    &http.Client{Timeout: timeout},
	}
}

type apiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type apiMessage struct {
	Role       string        `json:"role"`
	Content    any           `json:"content"`
	ToolCalls  []apiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	Name       string        `json:"name,omitempty"`
}

type apiRequest struct {
	Model    string       `json:"model"`
	Messages []apiMessage `json:"messages"`
	Tools    []apiTool    `json:"tools,omitempty"`
	Stream   bool         `json:"stream"`
}

type apiTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

type apiResponse struct {
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
	Choices []struct {
		Message apiMessage `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func (c *OpenAI) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = c.Model
	}
	body := apiRequest{
		Model:    model,
		Messages: toAPIMessages(req.Messages),
		Tools:    toAPITools(req.Tools),
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	res, err := c.HTTP.Do(httpReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("llm request failed: %w", err)
	}
	defer res.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return ChatResponse{}, err
	}
	var parsed apiResponse
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return ChatResponse{}, fmt.Errorf("llm returned non-json (status %d): %s", res.StatusCode, truncate(string(payload), 400))
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return ChatResponse{}, fmt.Errorf("%s", parsed.Error.Message)
	}
	if res.StatusCode >= 400 {
		return ChatResponse{}, fmt.Errorf("llm http %d: %s", res.StatusCode, truncate(string(payload), 400))
	}
	if len(parsed.Choices) == 0 {
		return ChatResponse{}, fmt.Errorf("llm returned no choices")
	}
	msg := fromAPIMessage(parsed.Choices[0].Message)
	return ChatResponse{
		Message:          msg,
		PromptTokens:     parsed.Usage.PromptTokens,
		CompletionTokens: parsed.Usage.CompletionTokens,
	}, nil
}

func toAPIMessages(in []Message) []apiMessage {
	out := make([]apiMessage, 0, len(in))
	for _, m := range in {
		am := apiMessage{
			Role:       m.Role,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		}
		if m.Content != "" || len(m.ToolCalls) == 0 {
			am.Content = m.Content
		}
		for _, tc := range m.ToolCalls {
			at := apiToolCall{ID: tc.ID, Type: "function"}
			at.Function.Name = tc.Name
			at.Function.Arguments = tc.Arguments
			am.ToolCalls = append(am.ToolCalls, at)
		}
		out = append(out, am)
	}
	return out
}

func toAPITools(in []ToolSpec) []apiTool {
	if len(in) == 0 {
		return nil
	}
	out := make([]apiTool, 0, len(in))
	for _, t := range in {
		at := apiTool{Type: "function"}
		at.Function.Name = t.Name
		at.Function.Description = t.Description
		params := t.Parameters
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		at.Function.Parameters = params
		out = append(out, at)
	}
	return out
}

func fromAPIMessage(m apiMessage) Message {
	out := Message{Role: m.Role, ToolCallID: m.ToolCallID, Name: m.Name}
	switch c := m.Content.(type) {
	case string:
		out.Content = c
	case []any:
		var b strings.Builder
		for _, part := range c {
			obj, _ := part.(map[string]any)
			if obj == nil {
				continue
			}
			if t, _ := obj["text"].(string); t != "" {
				b.WriteString(t)
			}
		}
		out.Content = b.String()
	}
	for _, tc := range m.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
