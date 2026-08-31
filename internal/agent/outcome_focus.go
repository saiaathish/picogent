package agent

import (
	"strings"

	"github.com/saiaathish/picogent/internal/outcome"
	"github.com/saiaathish/picogent/internal/projecthealth"
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

// outcomeFocusForTask turns the latest durable task snapshot into one bounded
// advisory without pretending that an old project-health observation is still
// current. The unknown health report makes that freshness boundary explicit;
// callers can replace it with a fresh report only when the read happened after
// the latest mutation.
func outcomeFocusForTask(task *taskstate.Task) string {
	if task == nil {
		return ""
	}
	return outcome.EngineInstruction(outcome.Build(task, projecthealth.Report{
		Schema: projecthealth.Schema,
		Status: projecthealth.StateUnknown,
	}))
}
