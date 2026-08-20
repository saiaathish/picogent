package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/codexauth"
)

const defaultCodexURL = "https://chatgpt.com/backend-api/codex"

type TokenSource interface {
	Token(ctx context.Context) (access, account string, err error)
	ForceRefresh(ctx context.Context) error
}

type liveTokens struct{}

func (liveTokens) Token(ctx context.Context) (string, string, error) {
	return codexauth.Token(ctx)
}
func (liveTokens) ForceRefresh(ctx context.Context) error {
	return codexauth.ForceRefresh(ctx)
}

type Codex struct {
	Model      string
	BaseURL    string
	Originator string
	Tokens     TokenSource
	HTTP       *http.Client
}

func NewCodex(model string) *Codex {
	if model == "" {
		model = codexauth.DefaultModel()
	}
	return &Codex{
		Model:      model,
		BaseURL:    defaultCodexURL,
		Originator: codexauth.Originator(),
		Tokens:     liveTokens{},
		HTTP: &http.Client{
			Timeout: 0,
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				ResponseHeaderTimeout: 90 * time.Second,
				IdleConnTimeout:       120 * time.Second,
			},
		},
	}
}

type responsesReq struct {
	Model        string          `json:"model"`
	Instructions string          `json:"instructions,omitempty"`
	Input        []any           `json:"input"`
	Tools        []responsesTool `json:"tools,omitempty"`
	ToolChoice   string          `json:"tool_choice,omitempty"`
	Store        bool            `json:"store"`
	Stream       bool            `json:"stream"`
}

type responsesTool struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

func (c *Codex) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	out, err := c.chatOnce(ctx, req, false)
	if err != nil && isAuthErr(err) {
		_ = c.Tokens.ForceRefresh(ctx)
		return c.chatOnce(ctx, req, true)
	}
	return out, err
}

func (c *Codex) chatOnce(ctx context.Context, req ChatRequest, retried bool) (ChatResponse, error) {
	_ = retried
	access, account, err := c.Tokens.Token(ctx)
	if err != nil {
		return ChatResponse{}, err
	}
	model := req.Model
	if model == "" {
		model = c.Model
	}
	instructions, input := toResponsesInput(req.Messages)
	body := responsesReq{
		Model:        model,
		Instructions: instructions,
		Input:        input,
		Tools:        toResponsesTools(req.Tools),
		Store:        false,
		Stream:       true,
	}
	if len(body.Tools) > 0 {
		body.ToolChoice = "auto"
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/responses", bytes.NewReader(raw))
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+access)
	httpReq.Header.Set("OpenAI-Beta", "responses=experimental")
	httpReq.Header.Set("originator", c.Originator)
	if account != "" {
		httpReq.Header.Set("chatgpt-account-id", account)
	}
	res, err := c.HTTP.Do(httpReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("codex request failed: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return ChatResponse{}, fmt.Errorf("codex http 401")
	}
	if res.StatusCode >= 400 {
		payload, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		return ChatResponse{}, fmt.Errorf("codex http %d: %s", res.StatusCode, truncate(string(payload), 400))
	}
	msg, err := readResponsesStream(res.Body)
	if err != nil {
		return ChatResponse{}, err
	}
	return ChatResponse{Message: msg}, nil
}

func isAuthErr(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "http 401") || strings.Contains(s, "unauthorized")
}

func toResponsesTools(in []ToolSpec) []responsesTool {
	if len(in) == 0 {
		return nil
	}
	out := make([]responsesTool, 0, len(in))
	for _, t := range in {
		params := t.Parameters
		if params == nil {
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, responsesTool{
			Type:        "function",
			Name:        t.Name,
			Description: t.Description,
			Parameters:  params,
		})
	}
	return out
}

func toResponsesInput(msgs []Message) (instructions string, input []any) {
	for _, m := range msgs {
		switch m.Role {
		case "system":
			if instructions != "" {
				instructions += "\n\n"
			}
			instructions += m.Content
		case "user":
			input = append(input, map[string]any{
				"type": "message",
				"role": "user",
				"content": []map[string]any{{
					"type": "input_text",
					"text": m.Content,
				}},
			})
		case "assistant":
			if strings.TrimSpace(m.Content) != "" {
				input = append(input, map[string]any{
					"type": "message",
					"role": "assistant",
					"content": []map[string]any{{
						"type": "output_text",
						"text": m.Content,
					}},
				})
			}
			for _, tc := range m.ToolCalls {
				item := map[string]any{
					"type":      "function_call",
					"call_id":   tc.ID,
					"name":      tc.Name,
					"arguments": tc.Arguments,
					"status":    "completed",
				}
				if tc.ItemID != "" {
					item["id"] = tc.ItemID
				}
				input = append(input, item)
			}
		case "tool":
			input = append(input, map[string]any{
				"type":    "function_call_output",
				"call_id": m.ToolCallID,
				"output":  m.Content,
			})
		}
	}
	return instructions, input
}

func readResponsesStream(r io.Reader) (Message, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
	var event, data string
	var text strings.Builder
	var calls []ToolCall
	flush := func() error {
		if strings.TrimSpace(data) == "" {
			event, data = "", ""
			return nil
		}
		if strings.TrimSpace(data) == "[DONE]" {
			event, data = "", ""
			return nil
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			event, data = "", ""
			return nil
		}
		typ, _ := payload["type"].(string)
		if typ == "" {
			typ = event
		}
		switch typ {
		case "response.output_text.delta":
			if d, _ := payload["delta"].(string); d != "" {
				text.WriteString(d)
			}
		case "response.failed", "error", "response.error":
			msg := "codex stream error"
			if e, _ := payload["error"].(map[string]any); e != nil {
				if m, _ := e["message"].(string); m != "" {
					msg = m
				}
			}
			if d, _ := payload["detail"].(string); d != "" {
				msg = d
			}
			return fmt.Errorf("%s", msg)
		case "response.output_item.done":
			item, _ := payload["item"].(map[string]any)
			if item == nil {
				break
			}
			if it, _ := item["type"].(string); it == "function_call" {
				callID, _ := item["call_id"].(string)
				name, _ := item["name"].(string)
				args, _ := item["arguments"].(string)
				id, _ := item["id"].(string)
				if callID == "" {
					callID = id
				}
				calls = append(calls, ToolCall{ID: callID, ItemID: id, Name: name, Arguments: args})
			}
			if it, _ := item["type"].(string); it == "message" && text.Len() == 0 {
				if t := extractItemText(item); t != "" {
					text.WriteString(t)
				}
			}
		}
		event, data = "", ""
		return nil
	}
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "" {
			if err := flush(); err != nil {
				return Message{}, err
			}
			continue
		}
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			chunk := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data != "" {
				data += "\n"
			}
			data += chunk
		}
	}
	if err := flush(); err != nil {
		return Message{}, err
	}
	if err := sc.Err(); err != nil {
		return Message{}, err
	}
	return Message{Role: "assistant", Content: text.String(), ToolCalls: calls}, nil
}

func extractItemText(item map[string]any) string {
	content, _ := item["content"].([]any)
	var b strings.Builder
	for _, part := range content {
		obj, _ := part.(map[string]any)
		if obj == nil {
			continue
		}
		if t, _ := obj["text"].(string); t != "" {
			b.WriteString(t)
		}
	}
	return b.String()
}
