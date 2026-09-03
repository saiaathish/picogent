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
	contradictionAction     = "diagnose the conflicting evidence and recheck the requested outcome"
	contradictionReason     = "current trusted evidence contains a contradiction; diagnose and recheck before continuing"
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

	// runtimeConfirmed preserves a confirmed boundary that falls beyond the
	// bounded signal list. It is intentionally not serialized, and runtimeKey
	// invalidates it if a caller mutates the visible report fields.
	runtimeConfirmed               bool
	runtimeConfirmedAffectsOutcome bool
	runtimeKey                     string
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
			if key.criterionIndex >= 0 {
				// A criterion-bound invalidation supersedes every proof kind for
				// that criterion. The existing taskstate marker is emitted as a
				// verification record even when the invalidated PASS came from the
				// typed tests producer.
				for boundary := range groups {
					if boundary.criterionIndex == key.criterionIndex && boundary.changeSeq == key.changeSeq {
						delete(groups, boundary)
					}
				}
			} else {
				groups[key] = contradictionBoundaryState{}
			}
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
	confirmedAny := false
	confirmedAffectsOutcome := false
	for _, key := range keys {
		state := groups[key]
		if trustedContradictionEvidence(key.kind, state.positive) && trustedContradictionEvidence(key.kind, state.negative) {
			confirmedAny = true
			if key.criterionIndex < 0 {
				if requiredContradictionKind(task, key.kind) {
					confirmedAffectsOutcome = true
					break
				}
				continue
			}
			for _, index := range task.RequiredCriterionIndices() {
				if index == key.criterionIndex {
					confirmedAffectsOutcome = true
					break
				}
			}
			if confirmedAffectsOutcome {
				break
			}
		}
	}

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
	if confirmedAny {
		report.State = ContradictionConfirmed
	} else if len(report.Signals) > 0 {
		report.State = ContradictionAdvisory
	}
	report.runtimeConfirmed = confirmedAny
	report.runtimeConfirmedAffectsOutcome = confirmedAffectsOutcome
	report.runtimeKey = contradictionReportRuntimeKey(report)
	return boundContradictionReport(report)
}

func requiredContradictionKind(task *taskstate.Task, kind taskstate.EvidenceKind) bool {
	if task == nil {
		return false
	}
	kind, ok := canonicalContradictionKind(kind)
	if !ok {
		return false
	}
	for _, required := range task.RequiredEvidenceKinds() {
		required, ok := canonicalContradictionKind(required)
		if !ok {
			continue
		}
		if required == kind || (required == taskstate.EvidenceKindTests && kind == taskstate.EvidenceKindVerification) {
			return true
		}
	}
	return false
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
		if report.runtimeConfirmed {
			report.State = ContradictionConfirmed
		}
	}
}

func boundContradictionReport(report ContradictionReport) ContradictionReport {
	runtimeConfirmed := report.runtimeConfirmed && report.runtimeKey != "" && report.runtimeKey == contradictionReportRuntimeKey(report)
	runtimeConfirmedAffectsOutcome := runtimeConfirmed && report.runtimeConfirmedAffectsOutcome
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
		runtimeTrusted = runtimeTrusted && signal.runtimeKey == contradictionSignalRuntimeKey(signal)
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
	if runtimeConfirmed {
		report.State = ContradictionConfirmed
	} else {
		report.State = contradictionStateForSignals(report.Signals)
	}
	report.runtimeConfirmed = runtimeConfirmed
	report.runtimeConfirmedAffectsOutcome = runtimeConfirmedAffectsOutcome
	report.runtimeKey = ""
	if runtimeConfirmed {
		report.runtimeKey = contradictionReportRuntimeKey(report)
	}
	return report
}

func trustedContradictionReport(report ContradictionReport) bool {
	return report.State == ContradictionConfirmed &&
		report.runtimeConfirmed &&
		report.runtimeKey != "" &&
		report.runtimeKey == contradictionReportRuntimeKey(report)
}

func trustedContradictionReportAffectsOutcome(report ContradictionReport) bool {
	return trustedContradictionReport(report) && report.runtimeConfirmedAffectsOutcome
}

// reconcileContradictionReports is the boundary between the engine's two
// projections. A valid engine build supplies the same current report twice;
// a caller or reloaded contract may supply divergent reports. Divergence is
// retained only as bounded advisory categorical data, and both projections
// receive that same downgraded value so routing cannot read the stronger one.
func reconcileContradictionReports(left, right ContradictionReport) ContradictionReport {
	left = boundContradictionReport(left)
	right = boundContradictionReport(right)
	if sameVisibleContradictionReport(left, right) {
		return left
	}

	merged := ContradictionReport{Schema: ContradictionSchema}
	seen := make(map[string]struct{})
	appendSignals := func(report ContradictionReport) {
		for _, signal := range report.Signals {
			signal.State = ContradictionAdvisory
			signal.runtimeTrusted = false
			signal.runtimeKey = ""
			key := contradictionSignalVisibleKey(signal)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			if len(merged.Signals) >= maxContradictionSignals {
				merged.Truncated = true
				continue
			}
			merged.Signals = append(merged.Signals, signal)
		}
	}
	appendSignals(left)
	appendSignals(right)
	merged.Truncated = merged.Truncated || left.Truncated || right.Truncated
	if len(merged.Signals) > 0 {
		merged.State = ContradictionAdvisory
	}
	return boundContradictionReport(merged)
}

func sameVisibleContradictionReport(left, right ContradictionReport) bool {
	if left.Schema != right.Schema || left.State != right.State || left.Truncated != right.Truncated || len(left.Signals) != len(right.Signals) {
		return false
	}
	for index := range left.Signals {
		if contradictionSignalVisibleKey(left.Signals[index]) != contradictionSignalVisibleKey(right.Signals[index]) || left.Signals[index].State != right.Signals[index].State {
			return false
		}
	}
	return true
}

func contradictionSignalVisibleKey(signal ContradictionSignal) string {
	return strings.Join([]string{
		string(signal.Scope),
		string(signal.Kind),
		strconv.Itoa(signal.CriterionIndex),
		strconv.Itoa(signal.ChangeSeq),
		signal.PositiveStatus,
		signal.NegativeStatus,
		signal.PositiveOrigin,
		signal.NegativeOrigin,
	}, "\x00")
}

func contradictionReportRuntimeKey(report ContradictionReport) string {
	parts := []string{
		string(report.State),
		strconv.FormatBool(report.Truncated),
		strconv.FormatBool(report.runtimeConfirmedAffectsOutcome),
		strconv.Itoa(len(report.Signals)),
	}
	for _, signal := range report.Signals {
		parts = append(parts,
			string(signal.Scope),
			string(signal.Kind),
			strconv.Itoa(signal.CriterionIndex),
			strconv.Itoa(signal.ChangeSeq),
			signal.PositiveStatus,
			signal.NegativeStatus,
			signal.PositiveOrigin,
			signal.NegativeOrigin,
			string(signal.State),
			strconv.FormatBool(signal.runtimeTrusted),
			signal.runtimeKey,
		)
	}
	return strings.Join(parts, "\x00")
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
