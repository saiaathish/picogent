package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/claudeauth"
)

const defaultAnthropicURL = "https://api.anthropic.com/v1/messages"

// Claude Code OAuth requires this exact first system block + beta headers.
const claudeCodeIdentity = "You are Claude Code, Anthropic's official CLI for Claude."
const claudeCodeOAuthBeta = "claude-code-20250219,oauth-2025-04-20"
const claudeCodeUserAgent = "claude-cli/1.0.0 (external, picogent)"

// Anthropic is a Quad Code / Claude API client (API key or Claude Code CLI OAuth).
type Anthropic struct {
	BaseURL string
	APIKey  string // x-api-key (Console key); empty when using Claude Code OAuth
	UseOAuth bool  // Authorization: Bearer from Claude Code CLI login
	Model   string
	Timeout time.Duration
	HTTP    *http.Client
}

func NewAnthropic(apiKey, model string, timeout time.Duration) *Anthropic {
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	if model == "" {
		model = "claude-sonnet-5"
	}
	return &Anthropic{
		BaseURL: defaultAnthropicURL,
		APIKey:  apiKey,
		Model:   model,
		Timeout: timeout,
		HTTP:    &http.Client{Timeout: timeout},
	}
}

// NewClaudeCode builds a client that uses Claude Code CLI subscription auth
// (same credentials as `claude /login`) — no Anthropic API key required.
func NewClaudeCode(model string, timeout time.Duration) *Anthropic {
	c := NewAnthropic("", model, timeout)
	c.UseOAuth = true
	return c
}

type anthropicReq struct {
	Model        string                 `json:"model"`
	MaxTokens    int                    `json:"max_tokens"`
	System       any                    `json:"system,omitempty"`
	Messages     []anthropicMsg         `json:"messages"`
	Tools        []anthropicTool        `json:"tools,omitempty"`
	OutputConfig *anthropicOutputConfig `json:"output_config,omitempty"`
}

type anthropicOutputConfig struct {
	Effort string `json:"effort"`
}

type anthropicMsg struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type anthropicResp struct {
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
	Content []struct {
		Type  string `json:"type"`
		Text  string `json:"text"`
		ID    string `json:"id"`
		Name  string `json:"name"`
		Input any    `json:"input"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (c *Anthropic) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	model := req.Model
	if IsAutoModel(model) {
		model = c.Model
	}
	if model == "" {
		model = c.Model
	}

	var systemText strings.Builder
	var msgs []anthropicMsg
	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			if systemText.Len() > 0 {
				systemText.WriteString("\n\n")
			}
			systemText.WriteString(m.Content)
		case "user":
			if len(m.Parts) > 0 {
				msgs = append(msgs, anthropicMsg{Role: "user", Content: anthropicUserParts(m.Content, m.Parts)})
			} else {
				msgs = append(msgs, anthropicMsg{Role: "user", Content: m.Content})
			}
		case "assistant":
			if len(m.ToolCalls) > 0 {
				var blocks []map[string]any
				if t := strings.TrimSpace(m.Content); t != "" {
					blocks = append(blocks, map[string]any{"type": "text", "text": t})
				}
				for _, tc := range m.ToolCalls {
					var input any
					_ = json.Unmarshal([]byte(tc.Arguments), &input)
					blocks = append(blocks, map[string]any{
						"type":  "tool_use",
						"id":    tc.ID,
						"name":  tc.Name,
						"input": input,
					})
				}
				msgs = append(msgs, anthropicMsg{Role: "assistant", Content: blocks})
			} else {
				msgs = append(msgs, anthropicMsg{Role: "assistant", Content: m.Content})
			}
		case "tool":
			msgs = append(msgs, anthropicMsg{
				Role: "user",
				Content: []map[string]any{{
					"type":        "tool_result",
					"tool_use_id": m.ToolCallID,
					"content":     m.Content,
				}},
			})
		}
	}

	body := anthropicReq{
		Model:     model,
		MaxTokens: 8192,
		Messages:  msgs,
	}
	if c.UseOAuth {
		// OAuth path requires the Claude Code identity as the first system block.
		blocks := []map[string]any{{"type": "text", "text": claudeCodeIdentity}}
		if t := strings.TrimSpace(systemText.String()); t != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": t})
		}
		body.System = blocks
	} else if t := strings.TrimSpace(systemText.String()); t != "" {
		body.System = t
	}
	if req.Reasoning != "" && req.Reasoning != ReasonNone {
		body.OutputConfig = &anthropicOutputConfig{Effort: anthropicEffort(req.Reasoning)}
	}
	for _, t := range req.Tools {
		body.Tools = append(body.Tools, anthropicTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		})
	}

	b, err := json.Marshal(body)
	if err != nil {
		return ChatResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL, bytes.NewReader(b))
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	if c.UseOAuth {
		tok, err := claudeauth.Token()
		if err != nil {
			return ChatResponse{}, err
		}
		httpReq.Header.Set("Authorization", "Bearer "+tok)
		httpReq.Header.Set("anthropic-beta", claudeCodeOAuthBeta)
		httpReq.Header.Set("User-Agent", claudeCodeUserAgent)
		httpReq.Header.Set("x-app", "cli")
	} else {
		if c.APIKey == "" {
			return ChatResponse{}, fmt.Errorf("anthropic: missing API key")
		}
		httpReq.Header.Set("x-api-key", c.APIKey)
	}

	res, err := c.HTTP.Do(httpReq)
	if err != nil {
		return ChatResponse{}, err
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if res.StatusCode >= 400 {
		return ChatResponse{}, fmt.Errorf("anthropic http %d: %s", res.StatusCode, strings.TrimSpace(string(raw)))
	}

	var out anthropicResp
	if err := json.Unmarshal(raw, &out); err != nil {
		return ChatResponse{}, err
	}
	if out.Error != nil {
		return ChatResponse{}, fmt.Errorf("anthropic: %s", out.Error.Message)
	}

	msg := Message{Role: "assistant"}
	for _, block := range out.Content {
		switch block.Type {
		case "text":
			if msg.Content != "" {
				msg.Content += "\n"
			}
			msg.Content += block.Text
		case "tool_use":
			args, _ := json.Marshal(block.Input)
			msg.ToolCalls = append(msg.ToolCalls, ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: string(args),
			})
		}
	}

	return ChatResponse{
		Message:          msg,
		PromptTokens:     out.Usage.InputTokens,
		CompletionTokens: out.Usage.OutputTokens,
	}, nil
}

// anthropicEffort maps Picogent's scale onto Anthropic output_config.effort.
func anthropicEffort(r ReasoningLevel) string {
	switch r {
	case ReasonNone, ReasonLow, "minimal":
		return "low"
	case ReasonMedium:
		return "medium"
	case ReasonHigh, ReasonXHigh, ReasonMax, ReasonUltra:
		return "high"
	default:
		return "medium"
	}
}

func anthropicUserParts(text string, parts []Part) []map[string]any {
	var blocks []map[string]any
	if t := strings.TrimSpace(text); t != "" {
		blocks = append(blocks, map[string]any{"type": "text", "text": t})
	}
	for _, p := range parts {
		switch p.Type {
		case "image":
			blocks = append(blocks, map[string]any{
				"type": "image",
				"source": map[string]any{
					"type":       "base64",
					"media_type": p.MIME,
					"data":       base64.StdEncoding.EncodeToString(p.Data),
				},
			})
		case "file":
			if p.MIME == "application/pdf" || strings.HasSuffix(strings.ToLower(p.Name), ".pdf") {
				blocks = append(blocks, map[string]any{
					"type": "document",
					"source": map[string]any{
						"type":       "base64",
						"media_type": "application/pdf",
						"data":       base64.StdEncoding.EncodeToString(p.Data),
					},
				})
			} else {
				body := string(p.Data)
				if p.Text != "" {
					body = p.Text
				}
				blocks = append(blocks, map[string]any{
					"type": "text",
					"text": fmt.Sprintf("--- %s ---\n%s", p.Name, body),
				})
			}
		case "text":
			if p.Text != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": p.Text})
			}
		}
	}
	if len(blocks) == 0 {
		return []map[string]any{{"type": "text", "text": text}}
	}
	return blocks
}
