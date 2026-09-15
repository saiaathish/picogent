package agent

import (
	"context"
	"reflect"
	"strings"

	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
)

const (
	projectHealthAdmissionCallID           = "admission-project-health"
	projectHealthAdmissionAlreadyAttempted = "The bounded read-only admission was already attempted once; continue with the available evidence and permission rules."
)

type projectHealthAdmission struct {
	attempted bool
	focus     string
}

type projectHealthAdmissionSnapshot struct {
	workspace         string
	registryWorkspace string
	goal              string
	goalRevision      uint64
	sessionID         string
	sessionGeneration uint64
	registry          *tools.Registry
	task              *taskstate.Task
}

// admitProjectHealth optionally performs one invisible, read-only diagnosis
// before the first model request. The returned focus is always bounded engine
// guidance; raw project-health output never enters events or conversation
// history. An attempted admission suppresses later health calls for the turn,
// including failed, malformed, or stale observations.
func (a *Agent) admitProjectHealth(ctx context.Context, prompt string, mode TaskMode, scopeBoundary string, state RuntimeState, regCtx tools.Context, gate *perm.Gate) projectHealthAdmission {
	result := projectHealthAdmission{focus: outcomeFocusForTask(a.TaskSnapshot())}
	if !eligibleForProjectHealthAdmission(prompt, mode, scopeBoundary) {
		return result
	}
	result.attempted = true
	fallback := func() projectHealthAdmission {
		result.focus = outcomeFocusForTask(a.TaskSnapshot())
		return result
	}
	if state.Tools == nil || gate == nil {
		return fallback()
	}

	tool, ok := state.Tools.Get("project_health")
	if !ok {
		return fallback()
	}
	baseline := projectHealthAdmissionSnapshot{
		workspace:         state.CFG.Workspace,
		registryWorkspace: regCtx.Workspace,
		goal:              state.Goal,
		goalRevision:      state.GoalRevision,
		registry:          state.Tools,
		task:              a.TaskSnapshot(),
	}
	baseline.sessionID, baseline.sessionGeneration = a.taskSessionSnapshot()

	call := llm.ToolCall{
		ID:        projectHealthAdmissionCallID,
		Name:      "project_health",
		Arguments: "{}",
	}
	req := tool.Permission(call.Arguments, regCtx)
	req.Hint = perm.EnrichHint(req, call.Arguments)
	decision, _, err := gate.CheckWithProvenance(ctx, req)
	if err != nil || decision == perm.Deny {
		return fallback()
	}
	if err := req.ValidateWorkspaceIdentity(); err != nil || !a.projectHealthAdmissionFresh(baseline) {
		return fallback()
	}

	toolCtx := regCtx
	toolCtx.WorkspaceIdentity = req.WorkspaceIdentity()
	var output string
	var runErr error
	if a.runTool != nil {
		output, runErr = a.runTool(ctx, call, tool, toolCtx)
	} else {
		output, runErr, _ = tools.RunWithEvidence(ctx, tool, call.Arguments, toolCtx)
	}
	if runErr != nil || req.ValidateWorkspaceIdentity() != nil || !a.projectHealthAdmissionFresh(baseline) {
		return fallback()
	}
	if focus := outcomeFocusForTool(a.TaskSnapshot(), call.Name, output); focus != "" {
		result.focus = focus
	}
	return result
}

func eligibleForProjectHealthAdmission(prompt string, mode TaskMode, scopeBoundary string) bool {
	if mode.ReadOnly() || strings.TrimSpace(scopeBoundary) != "" {
		return false
	}
	inferred := taskstate.Infer(prompt)
	if !inferred.TaskLike || inferred.Intent == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(inferred.Intent.Class), "readiness") &&
		strings.EqualFold(strings.TrimSpace(inferred.Intent.Completeness), "full")
}

func (a *Agent) projectHealthAdmissionFresh(snapshot projectHealthAdmissionSnapshot) bool {
	current := a.RuntimeSnapshot()
	if strings.TrimSpace(current.CFG.Workspace) != strings.TrimSpace(snapshot.workspace) ||
		current.Goal != snapshot.goal ||
		current.GoalRevision != snapshot.goalRevision ||
		current.Tools != snapshot.registry {
		return false
	}
	if current.Tools != nil && current.Tools.ContextSnapshot().Workspace != snapshot.registryWorkspace {
		return false
	}
	sessionID, sessionGeneration := a.taskSessionSnapshot()
	if sessionID != snapshot.sessionID || sessionGeneration != snapshot.sessionGeneration {
		return false
	}
	return reflect.DeepEqual(snapshot.task, a.TaskSnapshot())
}

func withoutProjectHealth(specs []llm.ToolSpec) []llm.ToolSpec {
	filtered := make([]llm.ToolSpec, 0, len(specs))
	for _, spec := range specs {
		if spec.Name != "project_health" {
			filtered = append(filtered, spec)
		}
	}
	return filtered
}
