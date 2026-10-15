package tools

import (
	"encoding/json"
	"fmt"
	"github.com/saiaathish/picogent/internal/llm"
	"strings"
)

// NativeShellCalls recognizes a tiny compatibility grammar. It never executes
// shell syntax. Unsupported commands retain their original approval boundary.
func NativeShellCalls(call llm.ToolCall) []llm.ToolCall {
	if call.Name != "bash" {
		return []llm.ToolCall{call}
	}
	var input struct {
		Command string `json:"command"`
	}
	if json.Unmarshal([]byte(call.Arguments), &input) != nil {
		return []llm.ToolCall{call}
	}
	command := strings.TrimSpace(input.Command)
	if strings.ContainsAny(command, "\n\r|<>`$\\\"'(){}") {
		return []llm.ToolCall{call}
	}
	command = strings.ReplaceAll(command, "&&", ";")
	if strings.Contains(command, "&") {
		return []llm.ToolCall{call}
	}
	var calls []llm.ToolCall
	add := func(name string, args any) {
		raw, _ := json.Marshal(args)
		calls = append(calls, llm.ToolCall{ID: fmt.Sprintf("%s-native-%d", call.ID, len(calls)+1), Name: name, Arguments: string(raw)})
	}
	for _, part := range strings.Split(command, ";") {
		fields := strings.Fields(part)
		if len(fields) == 0 {
			return []llm.ToolCall{call}
		}
		switch {
		case fields[0] == "cat" && len(fields) > 1:
			for _, path := range fields[1:] {
				if strings.HasPrefix(path, "-") || strings.ContainsAny(path, "*?[]") {
					return []llm.ToolCall{call}
				}
				add("read_file", map[string]string{"path": path})
			}
		case strings.Join(fields, " ") == "git status --short" || strings.Join(fields, " ") == "git status":
			add("git", map[string]string{"action": "status"})
		case strings.Join(fields, " ") == "npm test --silent" || strings.Join(fields, " ") == "npm test" || strings.Join(fields, " ") == "python3 -m unittest discover" || strings.Join(fields, " ") == "python -m unittest discover":
			add("verify", map[string]any{})
		default:
			return []llm.ToolCall{call}
		}
	}
	if len(calls) > 16 {
		return []llm.ToolCall{call}
	}
	return calls
}
