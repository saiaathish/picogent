package benchmark

import (
	"fmt"
	"strings"
)

// LongHorizonSchema identifies the bounded, provider-independent report used
// by the long-horizon outcome fixture. It is an evidence format, not a
// durable task-state schema.
const LongHorizonSchema = "picogent.v4.long-horizon-outcome.v1"

const (
	MaxLongHorizonObservations = 256
	MaxLongHorizonEvents       = 8
	MaxLongHorizonFailures     = 64
	MaxLongHorizonUnverified   = 64
	MaxLongHorizonTextBytes    = 512
)

// ScenarioEvent is a fixed vocabulary for the lifecycle transitions that a
// long-horizon fixture may exercise. Repository or provider text cannot add a
// new event category.
type ScenarioEvent string

const (
	EventPlan         ScenarioEvent = "plan"
	EventMutation     ScenarioEvent = "mutation"
	EventVerification ScenarioEvent = "verification"
	EventRestart      ScenarioEvent = "restart"
	EventSteering     ScenarioEvent = "steering"
	EventRecovery     ScenarioEvent = "recovery"
	EventStop         ScenarioEvent = "stop"
)

func (e ScenarioEvent) valid() bool {
	switch e {
	case EventPlan, EventMutation, EventVerification, EventRestart, EventSteering, EventRecovery, EventStop:
		return true
	default:
		return false
	}
}

// EvidenceState distinguishes fresh proof from states that must not authorize
// completion.
type EvidenceState string

const (
	EvidenceCurrent    EvidenceState = "current"
	EvidenceStale      EvidenceState = "stale"
	EvidenceMissing    EvidenceState = "missing"
	EvidenceUnverified EvidenceState = "unverified"
)

func (e EvidenceState) valid() bool {
	switch e {
	case EvidenceCurrent, EvidenceStale, EvidenceMissing, EvidenceUnverified:
		return true
	default:
		return false
	}
}

// RecoveryState records whether a recovery operation is relevant to the
// scenario and whether it has completed successfully.
type RecoveryState string

const (
	RecoveryNotRequired RecoveryState = "not_required"
	RecoveryPending     RecoveryState = "pending"
	RecoveryComplete    RecoveryState = "complete"
	RecoveryFailed      RecoveryState = "failed"
)

func (r RecoveryState) valid() bool {
	switch r {
	case RecoveryNotRequired, RecoveryPending, RecoveryComplete, RecoveryFailed:
		return true
	default:
		return false
	}
}

// StopDecision is the observed Outcome Engine stop policy for one durable
// turn. Completion eligibility remains a separate claim derived from the
// authoritative criterion/evidence state.
type StopDecision string

const (
	StopContinue StopDecision = "CONTINUE"
	StopPause    StopDecision = "PAUSE"
	StopRecheck  StopDecision = "RECHECK"
	StopUnknown  StopDecision = "UNKNOWN"
)

func (s StopDecision) valid() bool {
	switch s {
	case StopContinue, StopPause, StopRecheck, StopUnknown:
		return true
	default:
		return false
	}
}

// TurnObservation is the bounded per-turn observation consumed by the
// long-horizon report. MutationSeq and VerifiedMutationSeq make evidence
// freshness explicit instead of inferring it from a timestamp or transcript.
type TurnObservation struct {
	Turn                int             `json:"turn"`
	TurnRevision        uint64          `json:"turn_revision"`
	Events              []ScenarioEvent `json:"events"`
	CriteriaComplete    bool            `json:"criteria_complete"`
	MutationSeq         uint64          `json:"mutation_seq"`
	VerifiedMutationSeq uint64          `json:"verified_mutation_seq"`
	Evidence            EvidenceState   `json:"evidence"`
	Recovery            RecoveryState   `json:"recovery"`
	Stop                StopDecision    `json:"stop"`
	CompletionEligible  bool            `json:"completion_eligible"`
}

// CanStop is the fail-closed completion predicate for the measurement
// contract. It does not mutate task state or replace the production outcome
// gate; it only says when an observation is internally consistent enough to
// record an eligible stop decision.
func (o TurnObservation) CanStop() bool {
	if !o.CriteriaComplete || o.Evidence != EvidenceCurrent || o.VerifiedMutationSeq != o.MutationSeq || o.Stop != StopRecheck {
		return false
	}
	return o.Recovery == RecoveryNotRequired || o.Recovery == RecoveryComplete
}

// Report is an ephemeral, bounded evidence report. SourceHead is mandatory so
// a result cannot be detached from the exact code under measurement.
type Report struct {
	Schema            string            `json:"schema"`
	Scenario          string            `json:"scenario"`
	SourceHead        string            `json:"source_head"`
	BaselineHead      string            `json:"baseline_head,omitempty"`
	Host              string            `json:"host"`
	GoVersion         string            `json:"go_version"`
	Command           string            `json:"command"`
	Observations      []TurnObservation `json:"observations"`
	InvariantFailures []string          `json:"invariant_failures,omitempty"`
	Unverified        []string          `json:"unverified,omitempty"`
}

// Validate checks the report shape and the invariants that prevent stale proof
// or an incomplete lifecycle from being recorded as completion. It never
// treats a report with recorded invariant failures as successful evidence.
func (r Report) Validate() error {
	if r.Schema != LongHorizonSchema {
		return fmt.Errorf("schema=%q, want %q", r.Schema, LongHorizonSchema)
	}
	for name, value := range map[string]string{
		"scenario":   r.Scenario,
		"host":       r.Host,
		"go_version": r.GoVersion,
		"command":    r.Command,
	} {
		if err := validateText(name, value, true); err != nil {
			return err
		}
	}
	if !validSHA(r.SourceHead) {
		return fmt.Errorf("source_head must be a full 40-character commit SHA")
	}
	if r.BaselineHead != "" && !validSHA(r.BaselineHead) {
		return fmt.Errorf("baseline_head must be a full 40-character commit SHA")
	}
	if len(r.Observations) == 0 || len(r.Observations) > MaxLongHorizonObservations {
		return fmt.Errorf("observations=%d outside 1..%d", len(r.Observations), MaxLongHorizonObservations)
	}
	if err := validateTextList("invariant_failures", r.InvariantFailures, MaxLongHorizonFailures); err != nil {
		return err
	}
	if err := validateTextList("unverified", r.Unverified, MaxLongHorizonUnverified); err != nil {
		return err
	}

	var previousRevision uint64
	for index, observation := range r.Observations {
		wantTurn := index + 1
		if observation.Turn != wantTurn {
			return fmt.Errorf("observation %d turn=%d, want %d", index, observation.Turn, wantTurn)
		}
		if observation.TurnRevision == 0 || (index > 0 && observation.TurnRevision <= previousRevision) {
			return fmt.Errorf("observation %d turn revision=%d is not strictly increasing", index, observation.TurnRevision)
		}
		previousRevision = observation.TurnRevision
		if len(observation.Events) == 0 || len(observation.Events) > MaxLongHorizonEvents {
			return fmt.Errorf("observation %d events=%d outside 1..%d", index, len(observation.Events), MaxLongHorizonEvents)
		}
		seenEvents := make(map[ScenarioEvent]struct{}, len(observation.Events))
		for _, event := range observation.Events {
			if !event.valid() {
				return fmt.Errorf("observation %d has unknown event %q", index, event)
			}
			if _, seen := seenEvents[event]; seen {
				return fmt.Errorf("observation %d repeats event %q", index, event)
			}
			seenEvents[event] = struct{}{}
		}
		if !observation.Evidence.valid() {
			return fmt.Errorf("observation %d has unknown evidence state %q", index, observation.Evidence)
		}
		if !observation.Recovery.valid() {
			return fmt.Errorf("observation %d has unknown recovery state %q", index, observation.Recovery)
		}
		if !observation.Stop.valid() {
			return fmt.Errorf("observation %d has unknown stop decision %q", index, observation.Stop)
		}
		if observation.Evidence == EvidenceCurrent && observation.VerifiedMutationSeq != observation.MutationSeq {
			return fmt.Errorf("observation %d marks evidence current at mutation=%d but verification covers=%d", index, observation.MutationSeq, observation.VerifiedMutationSeq)
		}
		if observation.CompletionEligible != observation.CanStop() {
			return fmt.Errorf("observation %d completion eligibility is inconsistent with the fail-closed predicate", index)
		}
		if len(r.InvariantFailures) > 0 && observation.CompletionEligible {
			return fmt.Errorf("observation %d claims completion while invariant failures are recorded", index)
		}
	}
	return nil
}

func validateText(name, value string, required bool) error {
	if len(value) > MaxLongHorizonTextBytes {
		return fmt.Errorf("%s exceeds %d bytes", name, MaxLongHorizonTextBytes)
	}
	value = strings.TrimSpace(value)
	if required && value == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}

func validateTextList(name string, values []string, max int) error {
	if len(values) > max {
		return fmt.Errorf("%s=%d exceeds %d", name, len(values), max)
	}
	for index, value := range values {
		if err := validateText(fmt.Sprintf("%s[%d]", name, index), value, true); err != nil {
			return err
		}
	}
	return nil
}

func validSHA(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') && (character < 'A' || character > 'F') {
			return false
		}
	}
	return true
}
