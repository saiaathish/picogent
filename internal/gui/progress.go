package gui

import (
	"github.com/saiaathish/picogent/internal/llm"
	"path/filepath"
)

func progressFor(call llm.ToolCall) string {
	switch call.Name {
	case "repo_map", "glob", "list_dir", "grep":
		return "Exploring the project"
	case "read_file":
		return "Reading " + filepath.Base(parseToolPath(call.Arguments))
	case "write_file", "edit_file":
		return "Editing " + filepath.Base(parseToolPath(call.Arguments))
	case "verify":
		return "Running project checks"
	case "bash":
		return "Running a command"
	case "git":
		return "Checking file changes"
	case "todo_write":
		return "Planning the next steps"
	default:
		return "Using a connected tool"
	}
}
