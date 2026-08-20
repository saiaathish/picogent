package llm

import "context"

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

type ToolCall struct {
	ID        string `json:"id"`
	ItemID    string `json:"item_id,omitempty"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolSpec struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type ChatRequest struct {
	Model    string
	Messages []Message
	Tools    []ToolSpec
	// Routing hints — used by auto router inside agent loops.
	ToolRound int
	Escalate  bool
}

type ChatResponse struct {
	Message          Message
	PromptTokens     int
	CompletionTokens int
}

type Client interface {
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
}

// Scripted is a test double. Each Chat() call returns the next response.
type Scripted struct {
	Responses []ChatResponse
	Calls     []ChatRequest
}

func (s *Scripted) Chat(_ context.Context, req ChatRequest) (ChatResponse, error) {
	s.Calls = append(s.Calls, req)
	if len(s.Responses) == 0 {
		return ChatResponse{Message: Message{Role: "assistant", Content: "done"}}, nil
	}
	r := s.Responses[0]
	s.Responses = s.Responses[1:]
	return r, nil
}
