package agent

import (
	"strings"

	"github.com/saiaathish/picogent/internal/outcome"
	"github.com/saiaathish/picogent/internal/taskstate"
)

// CompletionProof returns the durable criterion/evidence proof used by every
// surface that presents task progress. It deliberately exposes the existing
// outcome predicate without adding turn-specific retirement context.
func CompletionProof(task *taskstate.Task) taskstate.CompletionCheck {
	return outcome.EvaluateCompletion(task)
}

// CompletionProjection is the bounded result-level completion decision shared
// by the agent and every execution surface. Proof is the authoritative
// criterion/evidence check; Ready also accounts for the explicit completion
// marker and scoped-turn boundary that a surface must honor before retiring a
// goal.
type CompletionProjection struct {
	Proof    outcome.CompletionCheck `json:"proof"`
	Required bool                    `json:"required"`
	Ready    bool                    `json:"ready"`
	Marker   bool                    `json:"marker"`
	Reason   string                  `json:"reason"`
}

// Explanation returns the same bounded reason to all callers. Reasons are
// selected from the durable completion contract and never contain raw tool
// output or model instructions.
func (p CompletionProjection) Explanation() string {
	if reason := strings.TrimSpace(p.Reason); reason != "" {
		return reason
	}
	if reason := strings.TrimSpace(p.Proof.Reason); reason != "" {
		return reason
	}
	return "completion proof is unavailable"
}

// completionProjection joins the durable proof predicate with the small
// amount of turn context needed by a caller that may retire a goal. A missing
// durable task keeps the pre-task behavior for marker-only answers, but an
// active goal or changed workspace still requires passing verification.
func completionProjection(task *taskstate.Task, activeGoal string, marker, verificationPass bool, changedFiles int, scopeBoundary string) CompletionProjection {
	proof := outcome.EvaluateCompletion(task)
	active := strings.TrimSpace(activeGoal) != ""
	if changedFiles < 0 {
		changedFiles = 0
	}
	projection := CompletionProjection{
		Proof:    proof,
		Required: active || marker || changedFiles > 0 || taskNeedsVerification(task),
		Marker:   marker,
	}
	if !projection.Required {
		projection.Ready = true
		projection.Reason = "no completion gate is active"
		return projection
	}
	if active && !marker {
		projection.Reason = "the assistant did not provide a completion marker"
		return projection
	}
	if marker && strings.TrimSpace(scopeBoundary) != "" {
		projection.Reason = "a scoped turn cannot retire the broader outcome"
		return projection
	}

	proofReady := proof.Ready
	if task == nil {
		// There is no durable criterion state to evaluate. Preserve the legacy
		// no-task marker behavior only when no workspace mutation needs proof;
		// active goals and changed files remain fail-closed.
		proofReady = (changedFiles == 0 && !active) || verificationPass
	}
	if !proofReady {
		projection.Reason = proof.Reason
		if projection.Reason == "" || projection.Reason == "no durable task" {
			projection.Reason = "current workspace changes do not have passing verification"
		}
		return projection
	}

	projection.Ready = true
	if task == nil {
		if active || changedFiles > 0 {
			projection.Reason = "completion is backed by current verification without a durable task"
		} else {
			projection.Reason = "completion marker accepted without a durable task"
		}
	} else {
		projection.Reason = proof.Reason
		if projection.Reason == "" {
			projection.Reason = "all required completion proof is current"
		}
	}
	return projection
}

func taskNeedsVerification(task *taskstate.Task) bool {
	return task != nil && task.NeedsVerification()
}

// CompletionGate returns the agent's shared result projection. The fallback
// keeps callers and older tests that construct Result values directly safe
// while production runs always populate Completion before returning.
func (r Result) CompletionGate(activeGoal string) CompletionProjection {
	if r.Completion.Required || r.Completion.Ready || r.Completion.Marker || r.Completion.Reason != "" || r.Completion.Proof.Reason != "" {
		return r.Completion
	}
	if r.Task == nil && r.GoalDone {
		return CompletionProjection{
			Proof:    outcome.EvaluateCompletion(nil),
			Required: true,
			Ready:    true,
			Marker:   true,
			Reason:   "legacy completion marker accepted without a durable task",
		}
	}
	changedFiles := len(r.FilesChanged)
	if changedFiles == 0 && r.Task != nil {
		changedFiles = len(r.Task.ChangedFiles)
	}
	return completionProjection(r.Task, activeGoal, r.GoalDone, verificationStatus(r.Verified) == "PASS", changedFiles, "")
}
