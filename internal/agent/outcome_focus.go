package agent

import (
	"strings"

	"github.com/saiaathish/picogent/internal/outcome"
	"github.com/saiaathish/picogent/internal/taskstate"
)

// outcomeFocusForTool turns only a successful project_health result into one
// bounded advisory built from the durable Outcome Engine contract. Invalid,
// non-health, or failed tool output is ignored so a repository-controlled
// string cannot become an agent instruction.
func outcomeFocusForTool(task *taskstate.Task, toolName, toolOutput string) string {
	if toolName != "project_health" || strings.TrimSpace(toolOutput) == "" {
		return ""
	}
	contract, ok := outcome.FromJSON(task, toolOutput)
	if !ok {
		return ""
	}
	return outcome.EngineInstruction(contract)
}
