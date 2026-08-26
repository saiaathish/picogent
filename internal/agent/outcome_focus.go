package agent

import (
	"strings"

	"github.com/saiaathish/picogent/internal/outcome"
	"github.com/saiaathish/picogent/internal/taskstate"
)

// outcomeFocusForTool turns only a successful project_health result into an
// internal prompt. Invalid, non-health, or failed tool output is ignored so a
// repository-controlled string cannot become an agent instruction.
func outcomeFocusForTool(task *taskstate.Task, toolName, toolOutput string) string {
	if toolName != "project_health" || strings.TrimSpace(toolOutput) == "" {
		return ""
	}
	decision, ok := outcome.SelectFromJSON(task, toolOutput)
	if !ok {
		return ""
	}
	return outcome.Instruction(decision)
}
