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
)

const defaultAnthropicURL = "https://api.anthropic.com/v1/messages"

// Anthropic is a Quad Code / Claude API client.
type Anthropic struct {
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
	HTTP    *http.Client
}

func NewAnthropic(apiKey, model string, timeout time.Duration) *Anthropic {
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	if model == "" {
		model = "claude-sonnet-4-5"
	}
	return &Anthropic{
		BaseURL: defaultAnthropicURL,
		APIKey:  apiKey,
		Model:   model,
		Timeout: timeout,
		HTTP:    &http.Client{Timeout: timeout},
	}
}

type anthropicReq struct {
	Model        string                 `json:"model"`
	MaxTokens    int                    `json:"max_tokens"`
	System       string                 `json:"system,omitempty"`
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

	var system strings.Builder
	var msgs []anthropicMsg
	for _, m := range req.Messages {
		switch m.Role {
		case "system":
			if system.Len() > 0 {
				system.WriteString("\n\n")
			}
			system.WriteString(m.Content)
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
		System:    system.String(),
		Messages:  msgs,
	}
	if req.Reasoning != "" {
		body.OutputConfig = &anthropicOutputConfig{Effort: string(req.Reasoning)}
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
	httpReq.Header.Set("x-api-key", c.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

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
