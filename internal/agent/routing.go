package agent

import "strings"

// classifyToolKind maps a tool name to a routing hint for the reasoning router.
func classifyToolKind(name string) string {
	switch name {
	case "read_file", "list_dir", "glob", "grep", "web_fetch":
		return "read"
	case "write_file", "edit_file":
		return "write"
	case "bash", "git":
		return "shell"
	default:
		lower := strings.ToLower(name)
		if strings.HasPrefix(lower, "mcp_") {
			if strings.Contains(lower, "write") || strings.Contains(lower, "edit") {
				return "write"
			}
			if strings.Contains(lower, "read") || strings.Contains(lower, "grep") || strings.Contains(lower, "snapshot") {
				return "read"
			}
		}
		return "other"
	}
}
