package outcome

import (
	"strings"

	"github.com/saiaathish/picogent/internal/redact"
	"github.com/saiaathish/picogent/internal/taskstate"
)

const (
	maxTurnContractText         = 320
	maxTurnContractToolRounds   = 128
	maxTurnContractMutations    = 128
	maxTurnContractChangedFiles = 16
)

// TurnContract is the bounded outcome/recovery projection shared by the
// Outcome Engine and model router. Task state remains authoritative; this
// view contains no prompt, repository, or tool output and is rebuilt from the
// latest durable snapshot.
//
// The Last* fields describe the most recent durable turn. The other fields
// describe the task-level outcome state that helps a caller choose a safe
// continuation. Keeping both in one projection prevents routing callers from
// silently losing steering, recovery, or stop information.
type TurnContract struct {
	IntentClass                  string              `json:"intent_class,omitempty"`
	IntentRevision               uint64              `json:"intent_revision,omitempty"`
	CriterionIndex               int                 `json:"criterion_index,omitempty"`
	CriterionEvidence            string              `json:"criterion_evidence"`
	Attempts                     int                 `json:"attempts,omitempty"`
	ConsecutiveVerificationFails int                 `json:"consecutive_verification_fails,omitempty"`
	CompletionReady              bool                `json:"completion_ready"`
	StopReason                   string              `json:"stop_reason,omitempty"`
	TurnSequence                 uint64              `json:"turn_sequence,omitempty"`
	LastTurnIntentRevision       uint64              `json:"last_turn_intent_revision,omitempty"`
	LastTurnState                string              `json:"last_turn_state,omitempty"`
	LastRoute                    string              `json:"last_route,omitempty"`
	LastHypothesis               string              `json:"last_hypothesis,omitempty"`
	LastEvidenceState            string              `json:"last_evidence_state,omitempty"`
	LastTurnStopReason           string              `json:"last_turn_stop_reason,omitempty"`
	LastTurnAttempt              int                 `json:"last_turn_attempt,omitempty"`
	LastTurnToolRounds           int                 `json:"last_turn_tool_rounds,omitempty"`
	LastTurnMutations            int                 `json:"last_turn_mutations,omitempty"`
	LastTurnChangedFiles         []string            `json:"last_turn_changed_files,omitempty"`
	LastTurnChangedFilesCapped   bool                `json:"last_turn_changed_files_capped,omitempty"`
	Failure                      FailureIntelligence `json:"failure,omitempty"`
}

// TurnContractForTask derives the shared bounded projection from a durable
// task. It never mutates the task and deliberately uses the same completion
// predicate that controls task retirement.
func TurnContractForTask(task *taskstate.Task) TurnContract {
	if task == nil {
		return boundTurnContract(turnContractForTask(nil, CompletionCheck{}))
	}
	return boundTurnContract(turnContractForTask(task, task.CompletionCheck()))
}

func turnContractForTask(task *taskstate.Task, completion CompletionCheck) TurnContract {
	result := TurnContract{
		CriterionIndex:    -1,
		CriterionEvidence: "UNVERIFIED",
	}
	if task == nil {
		return result
	}

	if task.Intent != nil {
		result.IntentClass = task.Intent.Class
	}
	result.IntentRevision = task.IntentRevision
	if task.Attempts > 0 {
		result.Attempts = task.Attempts
	}
	if failures := task.ConsecutiveVerificationFailures(); failures > 0 {
		result.ConsecutiveVerificationFails = failures
	}
	result.Failure = FailureIntelligenceForTask(task)
	result.CompletionReady = completion.Ready
	if task.StopReason.Valid() && task.StopReason != taskstate.StopNone {
		result.StopReason = string(task.StopReason)
	}

	if turn := task.LastTurn(); turn != nil {
		result.TurnSequence = turn.Sequence
		result.LastTurnIntentRevision = turn.IntentRevision
		result.LastTurnState = string(turn.State)
		result.LastRoute = turn.Route
		result.LastHypothesis = turn.Hypothesis
		result.LastEvidenceState = turn.EvidenceState
		if turn.StopReason.Valid() && turn.StopReason != taskstate.StopNone {
			result.LastTurnStopReason = string(turn.StopReason)
		}
		result.LastTurnAttempt = turn.Attempt
		result.LastTurnToolRounds = turn.ToolRounds
		result.LastTurnMutations = turn.MutationCount
		result.LastTurnChangedFiles = append([]string(nil), turn.ChangedFiles...)
		result.LastTurnChangedFilesCapped = turn.ChangedFilesCapped
	}

	result.CriterionIndex = task.FirstMissingRequiredCriterion()
	if result.CriterionIndex >= 0 {
		if status, current := task.CriterionEvidenceState(result.CriterionIndex); current || status != "UNVERIFIED" {
			result.CriterionEvidence = status
		}
	} else if result.CompletionReady {
		result.CriterionEvidence = "PASS"
	}
	return result
}

// NeedsRecovery reports whether the shared durable projection contains a
// bounded signal that should make the next model route more conservative.
func (c TurnContract) NeedsRecovery() bool {
	if strings.EqualFold(strings.TrimSpace(c.LastTurnState), string(taskstate.TurnInterrupted)) || strings.EqualFold(strings.TrimSpace(c.LastRoute), string(taskstate.TurnRouteRecover)) {
		return true
	}
	if c.ConsecutiveVerificationFails > 0 {
		return true
	}
	if c.Failure.RepeatCount > 0 || c.Failure.NeedsNewHypothesis || c.Failure.NeedsDifferentRoute {
		return true
	}
	switch strings.ToUpper(strings.TrimSpace(c.CriterionEvidence)) {
	case "FAIL", "INCONCLUSIVE", "SKIPPED":
		return true
	default:
		return false
	}
}

func boundTurnContract(contract TurnContract) TurnContract {
	contract.IntentClass = compactContractString(redact.Text(contract.IntentClass), 64)
	contract.CriterionEvidence = normalizeTurnEvidence(contract.CriterionEvidence)
	contract.StopReason = normalizeTurnStopReason(contract.StopReason)
	contract.LastTurnState = normalizeTurnState(contract.LastTurnState)
	contract.LastRoute = normalizeTurnRoute(contract.LastRoute)
	contract.LastHypothesis = compactContractString(redact.Text(contract.LastHypothesis), maxTurnContractText)
	contract.LastEvidenceState = normalizeTurnEvidence(contract.LastEvidenceState)
	contract.LastTurnStopReason = normalizeTurnStopReason(contract.LastTurnStopReason)
	if contract.CriterionIndex < -1 {
		contract.CriterionIndex = -1
	}
	if contract.Attempts < 0 {
		contract.Attempts = 0
	}
	if contract.ConsecutiveVerificationFails < 0 {
		contract.ConsecutiveVerificationFails = 0
	}
	if contract.LastTurnAttempt < 0 {
		contract.LastTurnAttempt = 0
	}
	contract.LastTurnToolRounds = clampTurnContractCount(contract.LastTurnToolRounds, maxTurnContractToolRounds)
	contract.LastTurnMutations = clampTurnContractCount(contract.LastTurnMutations, maxTurnContractMutations)
	contract.LastTurnChangedFiles, contract.LastTurnChangedFilesCapped = boundTurnChangedFiles(contract.LastTurnChangedFiles, contract.LastTurnChangedFilesCapped)
	contract.Failure = boundFailureIntelligence(contract.Failure)
	return contract
}

func boundTurnChangedFiles(paths []string, capped bool) ([]string, bool) {
	if len(paths) == 0 {
		return nil, capped
	}
	out := make([]string, 0, minInt(len(paths), maxTurnContractChangedFiles))
	seen := make(map[string]struct{}, len(paths))
	for _, raw := range paths {
		path := compactContractString(redact.Text(raw), maxTurnContractText)
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		if len(out) >= maxTurnContractChangedFiles {
			return out, true
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out, capped
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func normalizeTurnEvidence(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	switch value {
	case "PASS", "FAIL", "INCONCLUSIVE", "SKIPPED", "UNVERIFIED":
		return value
	default:
		return "UNVERIFIED"
	}
}

func normalizeTurnStopReason(value string) string {
	reason := taskstate.StopReason(strings.TrimSpace(value))
	if reason == taskstate.StopNone || !reason.Valid() {
		return ""
	}
	return string(reason)
}

func normalizeTurnState(value string) string {
	switch taskstate.TurnState(strings.ToLower(strings.TrimSpace(value))) {
	case taskstate.TurnActive:
		return string(taskstate.TurnActive)
	case taskstate.TurnCompleted:
		return string(taskstate.TurnCompleted)
	case taskstate.TurnInterrupted:
		return string(taskstate.TurnInterrupted)
	default:
		return ""
	}
}

func normalizeTurnRoute(value string) string {
	switch taskstate.TurnRoute(strings.ToLower(strings.TrimSpace(value))) {
	case taskstate.TurnRouteAdmission:
		return string(taskstate.TurnRouteAdmission)
	case taskstate.TurnRouteInspect:
		return string(taskstate.TurnRouteInspect)
	case taskstate.TurnRouteImplement:
		return string(taskstate.TurnRouteImplement)
	case taskstate.TurnRouteVerify:
		return string(taskstate.TurnRouteVerify)
	case taskstate.TurnRouteRecover:
		return string(taskstate.TurnRouteRecover)
	case taskstate.TurnRouteBlocked:
		return string(taskstate.TurnRouteBlocked)
	case taskstate.TurnRouteComplete:
		return string(taskstate.TurnRouteComplete)
	case taskstate.TurnRouteOther:
		return string(taskstate.TurnRouteOther)
	default:
		return ""
	}
}

func clampTurnContractCount(value, max int) int {
	if value < 0 {
		return 0
	}
	if value > max {
		return max
	}
	return value
}
