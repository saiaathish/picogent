package taskstate

import (
	"strings"
	"time"
)

const (
	maxTurnRecords    = 16
	maxTurnRoute      = 32
	maxTurnHypothesis = 320
	maxTurnEvidence   = 20
	maxTurnToolRounds = 128
	maxTurnMutations  = maxChangedFiles
)

// TurnState describes the lifecycle of one durable execution attempt.
// Active records are intentionally persisted before the provider is called so
// a process restart can distinguish an interrupted attempt from a cleanly
// completed one.
type TurnState string

const (
	TurnActive      TurnState = "active"
	TurnCompleted   TurnState = "completed"
	TurnInterrupted TurnState = "interrupted"
)

// TurnRoute is a small, canonical vocabulary for the kind of work attempted
// by a turn. It is routing context, not a user-facing planner or a command.
type TurnRoute string

const (
	TurnRouteAdmission TurnRoute = "admission"
	TurnRouteInspect   TurnRoute = "inspect"
	TurnRouteImplement TurnRoute = "implement"
	TurnRouteVerify    TurnRoute = "verify"
	TurnRouteRecover   TurnRoute = "recover"
	TurnRouteBlocked   TurnRoute = "blocked"
	TurnRouteComplete  TurnRoute = "complete"
	TurnRouteOther     TurnRoute = "other"
)

// TurnRecord is a bounded, durable summary of one execution attempt. It
// deliberately stores no raw prompt, model output, tool output, or command.
// Those remain in the separately bounded transcript/evidence surfaces.
type TurnRecord struct {
	Sequence       uint64     `json:"sequence"`
	Attempt        int        `json:"attempt"`
	IntentRevision uint64     `json:"intent_revision,omitempty"`
	State          TurnState  `json:"state"`
	Route          string     `json:"route,omitempty"`
	Hypothesis     string     `json:"hypothesis,omitempty"`
	EvidenceState  string     `json:"evidence_state,omitempty"`
	StopReason     StopReason `json:"stop_reason,omitempty"`
	ToolRounds     int        `json:"tool_rounds,omitempty"`
	MutationCount  int        `json:"mutation_count,omitempty"`
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
}

// Valid reports whether the turn lifecycle state is supported.
func (s TurnState) Valid() bool {
	switch s {
	case TurnActive, TurnCompleted, TurnInterrupted:
		return true
	default:
		return false
	}
}

// SetIntent records a changed interpretation without replacing the durable
// outcome. Intent revisions let a resumed task distinguish user steering from
// the original request while keeping the definition of done authoritative.
func (t *Task) SetIntent(intent *IntentContract) bool {
	if t == nil {
		return false
	}
	normalized := normalizeIntent(intent)
	if normalized == nil {
		if t.Intent == nil {
			return false
		}
		t.Intent = nil
		if t.IntentRevision < ^uint64(0) {
			t.IntentRevision++
		}
		t.touch()
		return true
	}
	if t.Intent != nil && *t.Intent == *normalized {
		return false
	}
	t.Intent = normalized
	t.IntentRevision++
	if t.IntentRevision == 0 {
		// Saturate rather than allowing an ABA-style wraparound after an
		// impossibly long-lived task.
		t.IntentRevision = ^uint64(0)
	}
	t.touch()
	return true
}

func normalizeIntent(intent *IntentContract) *IntentContract {
	if intent == nil {
		return nil
	}
	cp := *intent
	cp.Outcome = compactText(cp.Outcome, maxTaskGoal)
	cp.Class = compactText(cp.Class, maxIntentClass)
	cp.Action = compactText(cp.Action, maxIntentAction)
	cp.Completeness = compactText(cp.Completeness, maxIntentCompleteness)
	cp.Scope = compactText(cp.Scope, maxIntentScope)
	cp.Risk = compactText(cp.Risk, maxIntentRisk)
	cp.Confidence = compactText(cp.Confidence, maxIntentConfidence)
	if cp.Outcome == "" {
		return nil
	}
	return &cp
}

// BeginTurn persists an active attempt and returns its identity. If a prior
// attempt was still active, it is closed as interrupted before the new record
// is appended. Callers must retain the returned sequence and pass it to
// FinishTurn; an old turn can never finish a newer turn.
func (t *Task) BeginTurn(route TurnRoute) (uint64, bool) {
	if t == nil {
		return 0, false
	}
	if t.TurnRevision == ^uint64(0) {
		return 0, false
	}
	now := time.Now().UTC()
	if len(t.Turns) > 0 {
		latest := &t.Turns[len(t.Turns)-1]
		if latest.State == TurnActive {
			latest.State = TurnInterrupted
			latest.FinishedAt = timePtr(now)
		}
	}
	t.TurnRevision++
	record := TurnRecord{
		Sequence:       t.TurnRevision,
		Attempt:        t.Attempts,
		IntentRevision: t.IntentRevision,
		State:          TurnActive,
		Route:          normalizeTurnRoute(route),
		EvidenceState:  "UNVERIFIED",
		StartedAt:      now,
	}
	if len(t.Turns) >= maxTurnRecords {
		copy(t.Turns, t.Turns[len(t.Turns)-maxTurnRecords+1:])
		t.Turns = t.Turns[:maxTurnRecords-1]
	}
	t.Turns = append(t.Turns, record)
	t.touch()
	return record.Sequence, true
}

// FinishTurn closes the active turn identified by sequence. Unknown or stale
// identities are ignored, preserving lifecycle ordering under replacement or
// restart races.
func (t *Task) FinishTurn(sequence uint64, route TurnRoute, hypothesis, evidence string, stop StopReason, toolRounds, mutations int) bool {
	return t.closeTurn(sequence, TurnCompleted, route, hypothesis, evidence, stop, toolRounds, mutations)
}

// InterruptTurn closes an active turn as interrupted. Cancellation is a
// durable outcome, not an implicit active record that only becomes truthful
// after a later resume. The same sequence check as FinishTurn prevents a
// stale canceled run from closing a replacement turn.
func (t *Task) InterruptTurn(sequence uint64, route TurnRoute, hypothesis, evidence string, stop StopReason, toolRounds, mutations int) bool {
	return t.closeTurn(sequence, TurnInterrupted, route, hypothesis, evidence, stop, toolRounds, mutations)
}

func (t *Task) closeTurn(sequence uint64, state TurnState, route TurnRoute, hypothesis, evidence string, stop StopReason, toolRounds, mutations int) bool {
	if t == nil || sequence == 0 || len(t.Turns) == 0 {
		return false
	}
	latest := &t.Turns[len(t.Turns)-1]
	if latest.Sequence != sequence || latest.State != TurnActive {
		return false
	}
	if state != TurnCompleted && state != TurnInterrupted {
		return false
	}
	if !stop.Valid() {
		stop = StopNone
	}
	latest.State = state
	latest.Route = normalizeTurnRoute(route)
	latest.Hypothesis = compactText(hypothesis, maxTurnHypothesis)
	latest.EvidenceState = normalizeTurnEvidence(evidence)
	latest.StopReason = stop
	latest.ToolRounds = clampTurnCount(toolRounds, maxTurnToolRounds)
	latest.MutationCount = clampTurnCount(mutations, maxTurnMutations)
	latest.FinishedAt = timePtr(time.Now().UTC())
	t.touch()
	return true
}

// LastTurn returns an isolated copy of the most recent turn summary.
func (t *Task) LastTurn() *TurnRecord {
	if t == nil || len(t.Turns) == 0 {
		return nil
	}
	copy := t.Turns[len(t.Turns)-1]
	if copy.FinishedAt != nil {
		finished := *copy.FinishedAt
		copy.FinishedAt = &finished
	}
	return &copy
}

func normalizeTurnRoute(route TurnRoute) string {
	switch TurnRoute(strings.ToLower(strings.TrimSpace(string(route)))) {
	case TurnRouteAdmission:
		return string(TurnRouteAdmission)
	case TurnRouteInspect:
		return string(TurnRouteInspect)
	case TurnRouteImplement:
		return string(TurnRouteImplement)
	case TurnRouteVerify:
		return string(TurnRouteVerify)
	case TurnRouteRecover:
		return string(TurnRouteRecover)
	case TurnRouteBlocked:
		return string(TurnRouteBlocked)
	case TurnRouteComplete:
		return string(TurnRouteComplete)
	default:
		return string(TurnRouteOther)
	}
}

func normalizeTurnEvidence(evidence string) string {
	switch strings.ToUpper(strings.TrimSpace(evidence)) {
	case "PASS", "FAIL", "INCONCLUSIVE", "SKIPPED":
		return strings.ToUpper(strings.TrimSpace(evidence))
	default:
		return "UNVERIFIED"
	}
}

func clampTurnCount(value, max int) int {
	if value < 0 {
		return 0
	}
	if value > max {
		return max
	}
	return value
}

func timePtr(value time.Time) *time.Time {
	return &value
}
