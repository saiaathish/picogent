package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// openAIResponsesChat calls an OpenAI Responses API endpoint with a Bearer key
// (OpenCode Zen/Go GPT/Grok models).
func openAIResponsesChat(ctx context.Context, httpClient *http.Client, url, bearer string, req ChatRequest) (ChatResponse, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	model := req.Model
	instructions, input := toResponsesInput(req.Messages)
	body := responsesReq{
		Model:        model,
		Instructions: instructions,
		Input:        input,
		Tools:        toResponsesTools(req.Tools),
		Store:        false,
		Stream:       true,
	}
	if req.Reasoning != "" {
		body.Reasoning = &responsesReasoning{Effort: string(req.Reasoning)}
	}
	if len(body.Tools) > 0 {
		body.ToolChoice = "auto"
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+bearer)
	res, err := httpClient.Do(httpReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("opencode responses failed: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 400 {
		payload, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		return ChatResponse{}, fmt.Errorf("opencode responses http %d: %s", res.StatusCode, truncate(string(payload), 400))
	}
	msg, err := readResponsesStream(res.Body, req.OnDelta)
	if err != nil {
		return ChatResponse{}, err
	}
	return ChatResponse{Message: msg}, nil
}
