package outcome

import (
	"strings"

	"github.com/saiaathish/picogent/internal/taskstate"
)

const (
	// EvidenceLedgerEntry is intentionally categorical. It is a derived view
	// of taskstate evidence and never carries summaries, commands, references,
	// or other caller-controlled text across the Outcome Engine boundary.
	maxEvidenceLedgerEntries       = 32
	maxEvidenceLedgerPromptEntries = 3
)

// EvidenceLedgerEntry describes one bounded evidence record or one missing
// required proof boundary. Current means the record is from the task's
// current change sequence and its runtime producer is trusted for the bound
// kind; it is not completion authority.
type EvidenceLedgerEntry struct {
	Kind            taskstate.EvidenceKind   `json:"kind"`
	RequirementKind taskstate.EvidenceKind   `json:"requirement_kind,omitempty"`
	CriterionIndex  int                      `json:"criterion_index"`
	Status          string                   `json:"status"`
	Origin          taskstate.EvidenceOrigin `json:"origin,omitempty"`
	ChangeSeq       int                      `json:"change_seq"`
	Trusted         bool                     `json:"trusted"`
	Fresh           bool                     `json:"fresh"`
	Current         bool                     `json:"current"`
	Supersedes      bool                     `json:"supersedes,omitempty"`
	Observed        bool                     `json:"observed,omitempty"`
}

func evidenceLedgerFor(task *taskstate.Task) []EvidenceLedgerEntry {
	if task == nil {
		return nil
	}

	snapshots := task.EvidenceSnapshot()
	entries := make([]EvidenceLedgerEntry, 0, len(snapshots)+len(task.RequiredCriterionIndices())+len(task.RequiredEvidenceKinds()))
	seenCriteria := make(map[int]bool)
	seenRequirements := make(map[taskstate.EvidenceKind]bool)
	for _, snapshot := range snapshots {
		requirementKind := evidenceLedgerRequirementKind(task, snapshot)
		entry := EvidenceLedgerEntry{
			Kind:            snapshot.Kind,
			RequirementKind: requirementKind,
			CriterionIndex:  snapshot.CriterionIndex,
			Status:          snapshot.Status,
			Origin:          snapshot.Origin,
			ChangeSeq:       snapshot.ChangeSeq,
			Trusted:         snapshot.Trusted,
			Fresh:           snapshot.ChangeSeq == task.ChangeSeq,
			Supersedes:      snapshot.Supersedes,
			Observed:        true,
		}
		entry.Current = evidenceLedgerCurrent(entry)
		entries = append(entries, entry)
		if snapshot.CriterionIndex >= 0 {
			seenCriteria[snapshot.CriterionIndex] = true
		}
		if requirementKind != "" {
			seenRequirements[requirementKind] = true
		}
	}

	// Missing required boundaries remain explicit instead of disappearing into
	// a zero-entry ledger. They carry no provenance and therefore cannot look
	// like proof merely because the task is at the current change sequence.
	for _, index := range task.RequiredCriterionIndices() {
		if seenCriteria[index] {
			continue
		}
		entries = append(entries, EvidenceLedgerEntry{
			Kind:           taskstate.EvidenceKindVerification,
			CriterionIndex: index,
			Status:         "UNVERIFIED",
		})
	}
	for _, kind := range task.RequiredEvidenceKinds() {
		kind = normalizeRequirementKind(kind)
		if kind == "" || seenRequirements[kind] {
			continue
		}
		entries = append(entries, EvidenceLedgerEntry{
			Kind:            kind,
			RequirementKind: kind,
			CriterionIndex:  -1,
			Status:          "UNVERIFIED",
		})
	}

	return boundEvidenceLedger(entries)
}

func evidenceLedgerRequirementKind(task *taskstate.Task, snapshot taskstate.EvidenceSnapshot) taskstate.EvidenceKind {
	if task == nil || snapshot.CriterionIndex >= 0 {
		return ""
	}
	for _, required := range task.RequiredEvidenceKinds() {
		required = normalizeRequirementKind(required)
		if required == "" {
			continue
		}
		if snapshot.Kind == required || (required == taskstate.EvidenceKindTests && snapshot.Kind == taskstate.EvidenceKindVerification) {
			return required
		}
	}
	return ""
}

func evidenceLedgerCurrent(entry EvidenceLedgerEntry) bool {
	if !entry.Observed || !entry.Trusted || !entry.Fresh || entry.Supersedes || !entry.Origin.Valid() {
		return false
	}
	if entry.CriterionIndex >= 0 {
		if entry.Kind != taskstate.EvidenceKindVerification && entry.Kind != taskstate.EvidenceKindTests {
			return false
		}
		return entry.Origin.TrustedFor(entry.Kind)
	}
	if entry.RequirementKind != "" {
		return entry.Origin.TrustedFor(entry.RequirementKind)
	}
	return entry.Origin.TrustedFor(entry.Kind)
}

func boundEvidenceLedger(entries []EvidenceLedgerEntry) []EvidenceLedgerEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]EvidenceLedgerEntry, 0, minInt(len(entries), maxEvidenceLedgerEntries))
	for _, entry := range entries {
		entry.Kind = normalizeEvidenceLedgerKind(entry.Kind)
		if entry.Kind == "" {
			continue
		}
		entry.RequirementKind = normalizeRequirementKind(entry.RequirementKind)
		if entry.RequirementKind != "" && entry.CriterionIndex >= 0 {
			entry.RequirementKind = ""
		}
		if entry.RequirementKind != "" && entry.Kind != entry.RequirementKind && !(entry.RequirementKind == taskstate.EvidenceKindTests && entry.Kind == taskstate.EvidenceKindVerification) {
			entry.RequirementKind = ""
		}
		if entry.CriterionIndex < -1 {
			entry.CriterionIndex = -1
		}
		entry.Status = normalizeEvidenceLedgerStatus(entry.Status)
		if !entry.Origin.Valid() {
			entry.Origin = ""
		}
		if entry.ChangeSeq < 0 {
			entry.ChangeSeq = 0
		}
		if !entry.Observed {
			entry.Trusted = false
			entry.Fresh = false
			entry.Current = false
			entry.Origin = ""
		}
		entry.Current = entry.Current && evidenceLedgerCurrent(entry)
		if entry.CriterionIndex >= 0 && entry.Kind != taskstate.EvidenceKindVerification && entry.Kind != taskstate.EvidenceKindTests {
			entry.Current = false
		}
		if len(out) >= maxEvidenceLedgerEntries {
			break
		}
		out = append(out, entry)
	}
	return out
}

func normalizeEvidenceLedgerKind(kind taskstate.EvidenceKind) taskstate.EvidenceKind {
	switch strings.ToLower(strings.TrimSpace(string(kind))) {
	case string(taskstate.EvidenceKindVerification):
		return taskstate.EvidenceKindVerification
	case string(taskstate.EvidenceKindResearch):
		return taskstate.EvidenceKindResearch
	case string(taskstate.EvidenceKindMeasurement), "measure", "benchmark":
		return taskstate.EvidenceKindMeasurement
	case string(taskstate.EvidenceKindVisual), "visual_inspection":
		return taskstate.EvidenceKindVisual
	case string(taskstate.EvidenceKindTests), string(taskstate.EvidenceKindTest), "test_runner":
		return taskstate.EvidenceKindTests
	case string(taskstate.EvidenceKindApproval), "approved":
		return taskstate.EvidenceKindApproval
	case string(taskstate.EvidenceKindInspection):
		return taskstate.EvidenceKindInspection
	default:
		return ""
	}
}

func normalizeEvidenceLedgerStatus(status string) string {
	status = strings.ToUpper(strings.TrimSpace(status))
	switch status {
	case "PASS", "APPROVED", "CONFIRMED", "FAIL", "INCONCLUSIVE", "SKIPPED", "DENIED", "UNVERIFIED":
		return status
	default:
		return "UNVERIFIED"
	}
}

func evidenceLedgerPromptSummary(entries []EvidenceLedgerEntry) string {
	if len(entries) == 0 {
		return "entries=0"
	}
	counts := make(map[string]int)
	trusted, fresh, current := 0, 0, 0
	for _, entry := range entries {
		counts[entry.Status]++
		if entry.Trusted {
			trusted++
		}
		if entry.Fresh {
			fresh++
		}
		if entry.Current {
			current++
		}
	}
	statusParts := make([]string, 0, 4)
	for _, status := range []string{"PASS", "FAIL", "INCONCLUSIVE", "UNVERIFIED"} {
		if counts[status] > 0 {
			statusParts = append(statusParts, status+"="+itoa(counts[status]))
		}
	}
	samples := make([]string, 0, minInt(len(entries), maxEvidenceLedgerPromptEntries))
	for _, entry := range entries[:minInt(len(entries), maxEvidenceLedgerPromptEntries)] {
		binding := "aggregate"
		if entry.CriterionIndex >= 0 {
			binding = "criterion=" + itoa(entry.CriterionIndex)
		} else if entry.RequirementKind != "" {
			binding = "requirement=" + string(entry.RequirementKind)
		}
		samples = append(samples, string(entry.Kind)+"/"+entry.Status+"/"+binding+"/current="+boolWord(entry.Current))
	}
	return "entries=" + itoa(len(entries)) +
		" trusted=" + itoa(trusted) +
		" fresh=" + itoa(fresh) +
		" current=" + itoa(current) +
		" statuses=" + strings.Join(statusParts, ",") +
		" sample=" + strings.Join(samples, ";")
}
