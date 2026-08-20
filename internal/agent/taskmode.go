package agent

import "strings"

// TaskMode is how the agent approaches a turn (Cursor-style, kept minimal).
type TaskMode string

const (
	TaskAgent TaskMode = "agent"
	TaskAsk   TaskMode = "ask"
	TaskPlan  TaskMode = "plan"
	TaskDebug TaskMode = "debug"
)

func ParseTaskMode(s string) TaskMode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "ask":
		return TaskAsk
	case "plan":
		return TaskPlan
	case "debug":
		return TaskDebug
	default:
		return TaskAgent
	}
}

func (m TaskMode) Valid() bool {
	switch m {
	case TaskAgent, TaskAsk, TaskPlan, TaskDebug:
		return true
	default:
		return false
	}
}

func (m TaskMode) Label() string {
	switch m {
	case TaskAsk:
		return "Ask"
	case TaskPlan:
		return "Plan"
	case TaskDebug:
		return "Debug"
	default:
		return "Agent"
	}
}

func (m TaskMode) ReadOnly() bool {
	return m == TaskAsk || m == TaskPlan
}

func (m TaskMode) Prompt() string {
	switch m {
	case TaskAsk:
		return `

ASK MODE (read-only):
- Answer and explore. Do not edit files or run shell.
- If they want changes, they can just say so — do not mention slash commands.`
	case TaskPlan:
		return `

PLAN MODE:
- Research first. Do not edit yet.
- Output a short plan: goal, steps, files, risks, todos.
- End with: "Say go ahead to build." Do not mention slash commands.
- Do not implement until they approve.`
	case TaskDebug:
		return `

DEBUG MODE:
- Hypothesize, gather evidence, then fix.
- Prefer small diffs. Call verify after the fix.
- End with what was wrong, what changed, and how it was proven.`
	default:
		return ""
	}
}

func (m TaskMode) BlockTool(name string) (blocked bool, reason string) {
	if !m.ReadOnly() {
		return false, ""
	}
	switch name {
	case "write_file", "edit_file", "bash", "verify", "mcp_manage":
		return true, name + " blocked in " + strings.ToLower(m.Label()) + " mode (read-only)"
	default:
		if strings.HasPrefix(name, "mcp_") && looksWriteMCP(name) {
			return true, "write MCP tools blocked in " + strings.ToLower(m.Label()) + " mode"
		}
	}
	return false, ""
}

func looksWriteMCP(name string) bool {
	lower := strings.ToLower(name)
	for _, w := range []string{"write", "edit", "create", "delete", "send", "post", "push", "commit"} {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}
