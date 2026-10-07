package taskstate

import (
	"errors"
	"fmt"
	"strings"
)

// ReplaceOutcome replaces the durable contract using an extracted outcome
// prompt (such as the result of ExplicitReplacement). It is an explicit model
// mutation, not a follow-up classifier. Identity and cumulative work history
// survive; completion proof must be produced again for the new contract.
// Invalid input leaves the task, including its timestamps and proof, unchanged.
func (t *Task) ReplaceOutcome(prompt string) error {
	if t == nil {
		return errors.New("task is nil")
	}
	if t.IntentRevision == ^uint64(0) {
		return errors.New("task intent revision exhausted")
	}
	if err := t.Validate(); err != nil {
		return fmt.Errorf("validate task before outcome replacement: %w", err)
	}
	prompt = strings.Join(strings.Fields(prompt), " ")
	if !directReplacementOutcome(prompt) {
		return errors.New("replacement outcome must be a direct nonempty request")
	}
	if containsDelimitedPhrase(strings.ToLower(prompt), []string{
		"keep the current goal", "keep the goal", "preserve the current goal",
		"preserve the goal", "retain the current goal", "retain the goal",
	}) {
		return errors.New("replacement cannot also retain the previous workspace goal")
	}
	inferred := Infer(prompt)
	if !inferred.TaskLike || inferred.Goal == "" || inferred.Intent == nil || len(inferred.Steps) == 0 || len(inferred.DefinitionOfDone) == 0 {
		return errors.New("replacement outcome must be a nonempty task-like request")
	}

	// Work on isolated proof ledgers so a validation failure is a true no-op.
	next := *t
	next.Evidence = append([]Evidence(nil), t.Evidence...)
	next.Verification = append([]Verification(nil), t.Verification...)
	completed := replacementCompletedSummary(t)

	// Invalidate ALL old proof before rebinding the goal, intent, steps, or
	// criteria. The ordinary intent-change helper only invalidates current
	// passes and appends criterion markers; replacement must also detach old
	// failures, advisory records, and out-of-date proof without evicting their
	// useful summaries or allowing a same-index alias into the new contract.
	for i := range next.Evidence {
		next.Evidence[i].CriterionIndex = nil
		next.Evidence[i].trusted = false
		next.Evidence[i].Status = "STALE"
	}
	for i := range next.Verification {
		// Passed remains the historical check result, not current authority.
		// Rewriting old successes as failures would also distort retry budgets.
		next.Verification[i].trusted = false
		next.Verification[i].Coverage = VerificationCoverageUnbound
		next.Verification[i].Observation = nil
	}
	next.VerifiedChangeSeq = -1
	next.normalizedFromDone = false
	// The existing evidence cap is authoritative. Do not evict diagnostic
	// history merely to add a redundant completed-work summary.
	if completed != "" && len(next.Evidence) < maxEvidence {
		next.AddEvidence(Evidence{
			Kind:      EvidenceKindInspection,
			Status:    "STALE",
			Source:    "outcome-replacement",
			Origin:    EvidenceOriginSystem,
			Summary:   completed,
			ChangeSeq: next.ChangeSeq,
		})
	}

	next.Goal = inferred.Goal
	next.Intent = normalizeIntent(inferred.Intent)
	// Do not use SetIntent: equivalent text still represents a new outcome
	// identity, and that helper invalidates proof after assigning the intent.
	next.IntentRevision++
	next.DefinitionOfDone = append([]Criterion(nil), inferred.DefinitionOfDone...)
	next.Steps = make([]Step, 0, len(inferred.Steps))
	for _, description := range inferred.Steps {
		next.Steps = append(next.Steps, Step{Description: description})
	}
	next.Status = StatusWorking
	next.CurrentStep = 0
	next.BlockedBy = ""
	next.StopReason = StopNone
	next.touch()
	if err := next.Validate(); err != nil {
		return fmt.Errorf("validate replacement outcome: %w", err)
	}
	*t = next
	return nil
}

func replacementCompletedSummary(t *Task) string {
	var completed []string
	for _, step := range t.Steps {
		if step.Done {
			completed = append(completed, step.Description)
		}
	}
	if len(completed) == 0 {
		return ""
	}
	// Put progress first so a long goal cannot truncate all completed steps.
	return compactText("Previous outcome recorded completed steps: "+strings.Join(completed, "; ")+". Previous goal: "+t.Goal, maxEvidenceSummary)
}
