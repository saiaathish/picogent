package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
)

type TodoItem struct {
	Content string `json:"content"`
	Status  string `json:"status"`
}

type todoWrite struct{}

func (todoWrite) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Name:        "todo_write",
		Description: "Update the session task list (Claude Code TodoWrite). Use to track multi-step work. Status: pending, in_progress, completed.",
		Parameters: schema(map[string]any{
			"todos": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"content": map[string]any{"type": "string"},
						"status":  map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed"}},
					},
					"required": []string{"content", "status"},
				},
			},
		}, []string{"todos"}),
	}
}

func (todoWrite) Permission(_ string, _ Context) perm.Request {
	return perm.Request{Tool: "todo_write", Summary: "update task list"}
}

func (todoWrite) Run(_ context.Context, args string, c Context) (string, error) {
	var in struct {
		Todos []TodoItem `json:"todos"`
	}
	if err := parseJSON(args, &in); err != nil {
		return "", err
	}
	c.Todos = in.Todos
	var b strings.Builder
	b.WriteString("Todos updated:\n")
	for _, t := range in.Todos {
		mark := "[ ]"
		switch t.Status {
		case "completed":
			mark = "[x]"
		case "in_progress":
			mark = "[~]"
		}
		b.WriteString(fmt.Sprintf("%s %s\n", mark, t.Content))
	}
	return b.String(), nil
}

func FormatTodos(todos []TodoItem) string {
	if len(todos) == 0 {
		return ""
	}
	data, _ := json.MarshalIndent(todos, "", "  ")
	return string(data)
}
