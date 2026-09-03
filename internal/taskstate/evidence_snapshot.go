package taskstate

import "strings"

// EvidenceSnapshot is the non-durable, categorical view of one evidence
// record needed by derived outcome analysis. It intentionally omits summaries,
// commands, references, and timestamps so callers cannot accidentally turn
// repository or model text into an instruction.
//
// Trusted is runtime-only provenance. It is false after JSON reload until a
// live typed producer re-establishes the evidence. Supersedes identifies the
// fixed provenance emitted by an existing completion-boundary invalidation;
// it is a reset marker, not completion proof.
type EvidenceSnapshot struct {
	Kind           EvidenceKind
	Status         string
	Origin         EvidenceOrigin
	ChangeSeq      int
	CriterionIndex int
	Trusted        bool
	Supersedes     bool
}

// EvidenceSnapshot returns an isolated, bounded categorical copy of the
// current evidence ledger. Aggregate evidence uses CriterionIndex -1. Invalid
// criterion bindings are omitted rather than being reinterpreted as aggregate
// evidence. The result is advisory input for derived views; it never grants a
// caller completion authority.
func (t *Task) EvidenceSnapshot() []EvidenceSnapshot {
	if t == nil || len(t.Evidence) == 0 {
		return nil
	}
	criteriaCount := len(t.criteriaDefinition())
	out := make([]EvidenceSnapshot, 0, len(t.Evidence))
	for _, evidence := range t.Evidence {
		criterionIndex := -1
		if evidence.CriterionIndex != nil {
			criterionIndex = *evidence.CriterionIndex
			if criterionIndex < 0 || criterionIndex >= criteriaCount {
				continue
			}
		}
		kind := normalizeEvidenceKind(evidence.Kind)
		status := normalizeEvidenceStatus(evidence.Status)
		if kind == "" || status == "" {
			continue
		}
		out = append(out, EvidenceSnapshot{
			Kind:           kind,
			Status:         status,
			Origin:         EvidenceOrigin(strings.TrimSpace(string(evidence.Origin))),
			ChangeSeq:      evidence.ChangeSeq,
			CriterionIndex: criterionIndex,
			Trusted:        evidence.trusted,
			Supersedes:     completionInvalidationEvidence(evidence),
		})
	}
	return out
}

// completionInvalidationEvidence recognizes only the fixed provenance shapes
// emitted by the existing taskstate invalidation paths. These records explain
// why an older PASS is no longer current; treating them as contradictory
// failures would create a false conflict after intent changes, undo, or stale
// workspace verification. The detector still requires runtime trust before a
// signal can be confirmed.
func completionInvalidationEvidence(evidence Evidence) bool {
	if normalizeEvidenceStatus(evidence.Status) != "INCONCLUSIVE" {
		return false
	}
	source := strings.TrimSpace(evidence.Source)
	reference := strings.TrimSpace(evidence.Reference)
	switch {
	case source == "outcome-contract" && evidence.Origin == EvidenceOriginSystem && reference == "durable intent change":
		return true
	case source == "workspace-observation" && evidence.Origin == EvidenceOriginVerifier && (reference == "workspace restoration" || reference == "workspace.Observation"):
		return true
	default:
		return false
	}
}
