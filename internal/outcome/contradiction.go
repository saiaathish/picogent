package outcome

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/saiaathish/picogent/internal/taskstate"
)

const (
	// ContradictionSchema identifies the bounded, derived contradiction view.
	// It is not a durable task-state version and it never authorizes completion.
	ContradictionSchema     = "picogent.outcome-contradiction.v1"
	MaxContradictionBytes   = 4096
	maxContradictionSignals = 8
)

// ContradictionState describes whether comparable current evidence disagrees.
type ContradictionState string

const (
	ContradictionNone      ContradictionState = "NONE"
	ContradictionAdvisory  ContradictionState = "ADVISORY"
	ContradictionConfirmed ContradictionState = "CONFIRMED"
)

// ContradictionScope identifies the existing proof boundary that disagrees.
type ContradictionScope string

const (
	ContradictionScopeAggregate   ContradictionScope = "aggregate"
	ContradictionScopeCriterion   ContradictionScope = "criterion"
	ContradictionScopeRequirement ContradictionScope = "requirement"
)

// ContradictionSignal contains only categorical metadata. It deliberately
// excludes summaries, commands, references, timestamps, and repository text.
// CriterionIndex -1 identifies aggregate evidence. A confirmed signal means
// both observations came from a trusted typed producer in the current runtime;
// an advisory signal must not be used as completion proof or an instruction.
type ContradictionSignal struct {
	Scope          ContradictionScope     `json:"scope"`
	Kind           taskstate.EvidenceKind `json:"kind"`
	CriterionIndex int                    `json:"criterion_index"`
	ChangeSeq      int                    `json:"change_seq"`
	PositiveStatus string                 `json:"positive_status"`
	NegativeStatus string                 `json:"negative_status"`
	PositiveOrigin string                 `json:"positive_origin"`
	NegativeOrigin string                 `json:"negative_origin"`
	State          ContradictionState     `json:"state"`

	// runtimeTrusted is intentionally not serialized. Only
	// DetectContradictions can establish it from two trusted typed evidence
	// records; a caller-made report must remain advisory even when its visible
	// labels look valid.
	runtimeTrusted bool
	// runtimeKey detects mutation of the exported categorical fields after a
	// trusted signal was derived. The key is runtime-only and is intentionally
	// not accepted from JSON or generic callers.
	runtimeKey string
}

// ContradictionReport is a bounded derived view over one task snapshot.
type ContradictionReport struct {
	Schema    string                `json:"schema"`
	State     ContradictionState    `json:"state"`
	Signals   []ContradictionSignal `json:"signals,omitempty"`
	Truncated bool                  `json:"signals_truncated,omitempty"`
}

type contradictionBoundary struct {
	kind           taskstate.EvidenceKind
	criterionIndex int
	changeSeq      int
}

type contradictionBoundaryState struct {
	positive    taskstate.EvidenceSnapshot
	hasPositive bool
	negative    taskstate.EvidenceSnapshot
	hasNegative bool
}

type contradictionPolarity uint8

const (
	polarityNone contradictionPolarity = iota
	polarityPositive
	polarityNegative
)

// DetectContradictions derives disagreement from the current bounded evidence
// ledger without mutating the task. Stale generations are ignored. Explicit
// taskstate invalidation markers reset an older boundary because they describe
// supersession, not an independent negative observation.
func DetectContradictions(task *taskstate.Task) ContradictionReport {
	report := ContradictionReport{
		Schema: ContradictionSchema,
		State:  ContradictionNone,
	}
	if task == nil || task.ChangeSeq < 0 {
		return report
	}

	groups := make(map[contradictionBoundary]contradictionBoundaryState)
	for _, evidence := range task.EvidenceSnapshot() {
		kind, ok := canonicalContradictionKind(evidence.Kind)
		if !ok || evidence.ChangeSeq != task.ChangeSeq {
			continue
		}
		key := contradictionBoundary{
			kind:           kind,
			criterionIndex: evidence.CriterionIndex,
			changeSeq:      evidence.ChangeSeq,
		}
		if evidence.Supersedes {
			groups[key] = contradictionBoundaryState{}
			continue
		}
		state := groups[key]
		switch contradictionPolarityFor(evidence.Status) {
		case polarityPositive:
			state.positive = evidence
			state.hasPositive = true
		case polarityNegative:
			state.negative = evidence
			state.hasNegative = true
		}
		groups[key] = state
	}

	keys := make([]contradictionBoundary, 0, len(groups))
	for key, state := range groups {
		if state.hasPositive && state.hasNegative {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].kind != keys[j].kind {
			return keys[i].kind < keys[j].kind
		}
		if keys[i].criterionIndex != keys[j].criterionIndex {
			return keys[i].criterionIndex < keys[j].criterionIndex
		}
		return keys[i].changeSeq < keys[j].changeSeq
	})

	for _, key := range keys {
		state := groups[key]
		confirmed := trustedContradictionEvidence(key.kind, state.positive) && trustedContradictionEvidence(key.kind, state.negative)
		signalState := ContradictionAdvisory
		if confirmed {
			signalState = ContradictionConfirmed
		}
		signal := ContradictionSignal{
			Scope:          contradictionScopeFor(key.kind, key.criterionIndex),
			Kind:           key.kind,
			CriterionIndex: key.criterionIndex,
			ChangeSeq:      key.changeSeq,
			PositiveStatus: canonicalPositiveStatus(state.positive.Status),
			NegativeStatus: canonicalNegativeStatus(state.negative.Status),
			PositiveOrigin: safeContradictionOrigin(key.kind, state.positive),
			NegativeOrigin: safeContradictionOrigin(key.kind, state.negative),
			State:          signalState,
			runtimeTrusted: confirmed,
		}
		if confirmed {
			signal.runtimeKey = contradictionSignalRuntimeKey(signal)
		}
		report.Signals = append(report.Signals, signal)
		if confirmed {
			report.State = ContradictionConfirmed
		} else if report.State == ContradictionNone {
			report.State = ContradictionAdvisory
		}
		if len(report.Signals) == maxContradictionSignals {
			if len(keys) > len(report.Signals) {
				report.Truncated = true
			}
			break
		}
	}
	return boundContradictionReport(report)
}

// FormatContradictions returns bounded JSON for diagnostics and future
// routing. It canonicalizes categorical fields and never emits arbitrary
// caller-provided text.
func FormatContradictions(report ContradictionReport) string {
	report = boundContradictionReport(report)
	for {
		data, err := json.MarshalIndent(report, "", "  ")
		if err == nil && len(data) <= MaxContradictionBytes {
			return string(data)
		}
		if len(report.Signals) == 0 {
			return `{"schema":"picogent.outcome-contradiction.v1","state":"NONE"}`
		}
		report.Signals = report.Signals[:len(report.Signals)-1]
		report.Truncated = true
		report.State = contradictionStateForSignals(report.Signals)
	}
}

func boundContradictionReport(report ContradictionReport) ContradictionReport {
	report.Schema = ContradictionSchema
	if len(report.Signals) > maxContradictionSignals {
		report.Signals = append([]ContradictionSignal(nil), report.Signals[:maxContradictionSignals]...)
		report.Truncated = true
	} else {
		report.Signals = append([]ContradictionSignal(nil), report.Signals...)
	}
	valid := report.Signals[:0]
	for _, signal := range report.Signals {
		runtimeTrusted := signal.runtimeTrusted && signal.runtimeKey != "" && signal.runtimeKey == contradictionSignalRuntimeKey(signal)
		kind, ok := canonicalContradictionKind(signal.Kind)
		if !ok || contradictionPolarityFor(signal.PositiveStatus) != polarityPositive || contradictionPolarityFor(signal.NegativeStatus) != polarityNegative {
			continue
		}
		signal.Kind = kind
		if signal.CriterionIndex < -1 {
			signal.CriterionIndex = -1
		}
		if signal.ChangeSeq < 0 {
			signal.ChangeSeq = 0
		}
		signal.Scope = contradictionScopeFor(kind, signal.CriterionIndex)
		signal.PositiveStatus = canonicalPositiveStatus(signal.PositiveStatus)
		signal.NegativeStatus = canonicalNegativeStatus(signal.NegativeStatus)
		signal.PositiveOrigin = safeContradictionOriginString(kind, signal.PositiveOrigin)
		signal.NegativeOrigin = safeContradictionOriginString(kind, signal.NegativeOrigin)
		signal.runtimeTrusted = runtimeTrusted
		if !runtimeTrusted {
			signal.runtimeKey = ""
		}
		if signal.State == ContradictionConfirmed && (!runtimeTrusted || signal.PositiveOrigin == "untrusted" || signal.NegativeOrigin == "untrusted") {
			signal.State = ContradictionAdvisory
		} else if signal.State != ContradictionConfirmed {
			signal.State = ContradictionAdvisory
		}
		valid = append(valid, signal)
	}
	report.Signals = valid
	report.State = contradictionStateForSignals(report.Signals)
	return report
}

// contradictionSignalRuntimeKey is an opaque runtime witness for the
// categorical fields that establish trust. It deliberately keeps raw integer
// values so an invalid caller mutation cannot normalize into a trusted
// boundary during formatting.
func contradictionSignalRuntimeKey(signal ContradictionSignal) string {
	kind, ok := canonicalContradictionKind(signal.Kind)
	if !ok {
		return ""
	}
	return strings.Join([]string{
		string(kind),
		strconv.Itoa(signal.CriterionIndex),
		strconv.Itoa(signal.ChangeSeq),
		canonicalPositiveStatus(signal.PositiveStatus),
		canonicalNegativeStatus(signal.NegativeStatus),
		safeContradictionOriginString(kind, signal.PositiveOrigin),
		safeContradictionOriginString(kind, signal.NegativeOrigin),
	}, "\x00")
}

func contradictionStateForSignals(signals []ContradictionSignal) ContradictionState {
	if len(signals) == 0 {
		return ContradictionNone
	}
	for _, signal := range signals {
		if signal.State == ContradictionConfirmed {
			return ContradictionConfirmed
		}
	}
	return ContradictionAdvisory
}

func canonicalContradictionKind(kind taskstate.EvidenceKind) (taskstate.EvidenceKind, bool) {
	switch strings.ToLower(strings.TrimSpace(string(kind))) {
	case string(taskstate.EvidenceKindVerification):
		return taskstate.EvidenceKindVerification, true
	case string(taskstate.EvidenceKindResearch):
		return taskstate.EvidenceKindResearch, true
	case string(taskstate.EvidenceKindMeasurement), "measure", "benchmark":
		return taskstate.EvidenceKindMeasurement, true
	case string(taskstate.EvidenceKindVisual), "visual_inspection":
		return taskstate.EvidenceKindVisual, true
	case string(taskstate.EvidenceKindTests), string(taskstate.EvidenceKindTest), "test_runner":
		return taskstate.EvidenceKindTests, true
	case string(taskstate.EvidenceKindApproval), "approved":
		return taskstate.EvidenceKindApproval, true
	case string(taskstate.EvidenceKindInspection):
		return taskstate.EvidenceKindInspection, true
	default:
		return "", false
	}
}

func contradictionPolarityFor(status string) contradictionPolarity {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "PASS", "APPROVED", "CONFIRMED":
		return polarityPositive
	case "FAIL", "INCONCLUSIVE", "SKIPPED", "DENIED":
		return polarityNegative
	default:
		return polarityNone
	}
}

func canonicalPositiveStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "APPROVED":
		return "APPROVED"
	case "CONFIRMED":
		return "CONFIRMED"
	default:
		return "PASS"
	}
}

func canonicalNegativeStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "INCONCLUSIVE":
		return "INCONCLUSIVE"
	case "SKIPPED":
		return "SKIPPED"
	case "DENIED":
		return "DENIED"
	default:
		return "FAIL"
	}
}

func contradictionScopeFor(kind taskstate.EvidenceKind, criterionIndex int) ContradictionScope {
	if criterionIndex >= 0 {
		return ContradictionScopeCriterion
	}
	switch kind {
	case taskstate.EvidenceKindResearch, taskstate.EvidenceKindMeasurement, taskstate.EvidenceKindVisual, taskstate.EvidenceKindTests, taskstate.EvidenceKindApproval:
		return ContradictionScopeRequirement
	default:
		return ContradictionScopeAggregate
	}
}

func trustedContradictionEvidence(kind taskstate.EvidenceKind, evidence taskstate.EvidenceSnapshot) bool {
	return evidence.Trusted && evidence.Origin.Valid() && evidence.Origin.TrustedFor(kind)
}

func safeContradictionOrigin(kind taskstate.EvidenceKind, evidence taskstate.EvidenceSnapshot) string {
	if !trustedContradictionEvidence(kind, evidence) {
		return "untrusted"
	}
	return string(evidence.Origin)
}

func safeContradictionOriginString(kind taskstate.EvidenceKind, origin string) string {
	parsed := taskstate.EvidenceOrigin(strings.TrimSpace(origin))
	if !parsed.Valid() || !parsed.TrustedFor(kind) || parsed == taskstate.EvidenceOriginModel || parsed == taskstate.EvidenceOriginSystem {
		return "untrusted"
	}
	return string(parsed)
}
