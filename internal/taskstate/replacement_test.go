package taskstate

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/workspace"
)

func TestReplaceOutcomeCompletionBeforeAfterAndReload(t *testing.T) {
	task := replacementTask(t, "fix the cache")
	observation, err := workspace.Capture(context.Background(), t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	task.RecordChanged("cache.go")
	task.AddVerificationForCriteria(task.RequiredCriterionIndices(), "check cache", true, "verify PASS — cache checked", &observation)
	for task.Advance() {
	}
	if !task.CompletionReady() {
		t.Fatalf("fixture lacks completion proof: %#v", task.CompletionCheck())
	}
	if err := task.SetStatus(StatusDone); err != nil {
		t.Fatal(err)
	}
	store := NewStore(t.TempDir())
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}
	oldRevision, oldIntentRevision := task.Revision, task.IntentRevision
	if err := task.ReplaceOutcome("document the API"); err != nil {
		t.Fatal(err)
	}
	if task.Revision != oldRevision || task.IntentRevision != oldIntentRevision+1 {
		t.Fatalf("replacement changed persistence identity or failed to advance intent: %+v", task)
	}
	assertReplacementUnproven(t, task)
	if task.ReestablishWorkspaceVerification(&observation) {
		t.Fatal("the old workspace observation re-authorized replacement proof")
	}
	if err := task.SetStatus(StatusDone); err == nil {
		t.Fatal("replacement became done using old proof")
	}
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.Load(task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	assertReplacementUnproven(t, reloaded)
	if reloaded.IntentRevision != task.IntentRevision || reloaded.Revision != task.Revision || reloaded.Goal != task.Goal {
		t.Fatalf("reload lost replacement identity: %+v", reloaded)
	}
	if reloaded.ReestablishWorkspaceVerification(&observation) {
		t.Fatal("reload rebound retired workspace proof")
	}
	// Fresh proof can satisfy the new contract, with no new model required.
	for _, index := range reloaded.RequiredCriterionIndices() {
		reloaded.RecordCriterionVerification(index, "PASS", "API documentation checked", "check docs")
	}
	if !reloaded.CompletionReady() {
		t.Fatalf("fresh proof did not satisfy replacement: %#v", reloaded.CompletionCheck())
	}
	if err := reloaded.SetStatus(StatusDone); err != nil {
		t.Fatal(err)
	}
}

func TestReplaceOutcomeDetachesEveryOldCriterion(t *testing.T) {
	task := replacementTask(t, "fix the cache")
	for _, index := range task.RequiredCriterionIndices() {
		task.RecordCriterionVerification(index, "PASS", "old criterion checked", "old verifier")
	}
	task.RecordTestsEvidence("PASS", "old tests checked", "old runner")
	if !task.CompletionReady() {
		t.Fatal("old proof was not current")
	}
	// Include non-passing, advisory, and stale bindings, not only current PASS.
	last := len(task.DefinitionOfDone) - 1
	task.RecordCriterionVerification(last, "FAIL", "old failure retained", "old verifier")
	task.AddEvidenceForCriterion(0, Evidence{
		Kind: EvidenceKindInspection, Status: "UNVERIFIED", Summary: "old advisory retained",
	})
	task.RecordChanged("cache.go")
	oldEvidence := append([]Evidence(nil), task.Evidence...)
	if err := task.ReplaceOutcome("document the API"); err != nil {
		t.Fatal(err)
	}
	if len(task.DefinitionOfDone) >= last+1 {
		t.Fatal("fixture must replace the old plan with fewer criteria")
	}
	assertReplacementUnproven(t, task)
	if len(task.Evidence) != len(oldEvidence) {
		t.Fatal("replacement dropped old diagnostic evidence")
	}
	for i, evidence := range task.Evidence {
		if evidence.Summary != oldEvidence[i].Summary || evidence.Reference != oldEvidence[i].Reference || evidence.At != oldEvidence[i].At {
			t.Fatalf("diagnostic summary or provenance changed at %d: %#v", i, evidence)
		}
	}
	// Fresh evidence for a later criterion cannot resurrect an old index zero.
	for _, index := range task.RequiredCriterionIndices()[1:] {
		task.RecordCriterionVerification(index, "PASS", "new criterion checked", "new verifier")
	}
	if task.CompletionReady() || task.FirstMissingRequiredCriterion() != 0 {
		t.Fatalf("same-index old proof satisfied replacement: %#v", task.CompletionCheck())
	}
	if err := task.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestReplaceOutcomeSameTextAdvancesIntent(t *testing.T) {
	task := replacementTask(t, "fix the cache")
	for _, index := range task.RequiredCriterionIndices() {
		task.RecordCriterionVerification(index, "PASS", "cache criterion checked", "check cache")
	}
	task.RecordTestsEvidence("PASS", "cache tests checked", "check cache")
	if !task.CompletionReady() {
		t.Fatal("old proof was not current")
	}
	goal, intent, revision := task.Goal, *task.Intent, task.IntentRevision
	for i := uint64(1); i <= 2; i++ {
		if err := task.ReplaceOutcome(goal); err != nil {
			t.Fatal(err)
		}
		if task.Goal != goal || *task.Intent != intent || task.IntentRevision != revision+i {
			t.Fatalf("same-text replacement failed to advance identity: %+v", task)
		}
		assertReplacementUnproven(t, task)
	}
}

func TestReplaceOutcomePreservesCumulativeState(t *testing.T) {
	for _, state := range []Status{StatusPlanning, StatusWorking, StatusVerifying, StatusBlocked, StatusDone} {
		t.Run(string(state), func(t *testing.T) {
			task := replacementTask(t, "fix the cache")
			task.Revision = 17
			retirement := &GoalRetirement{Text: "old workspace outcome", Revision: 9}
			task.ReplacedWorkspaceGoal = retirement
			task.NoteAttempt()
			sequence, ok := task.BeginTurn(TurnRouteImplement)
			if !ok {
				t.Fatal("could not begin fixture turn")
			}
			task.RecordChanged("cache.go")
			task.RecordChanged("cache.go")
			if !task.FinishTurn(sequence, TurnRouteImplement, "cache fixed", "PASS", StopNone, 2, 2) {
				t.Fatal("could not finish fixture turn")
			}
			// An active turn is also retained for the parent to close or recover.
			if _, ok := task.BeginTurn(TurnRouteInspect); !ok {
				t.Fatal("could not begin active fixture turn")
			}
			task.AddConstraint("do not publish")
			task.AddRisk("external service may be unavailable")
			task.AddUncertainty("hardware behavior remains unknown")
			task.RecordCriterionVerification(0, "PASS", "cache fix inspected", "check cache")
			task.Advance()
			task.ChangedFilesCapped = true
			task.Status = state
			if state == StatusBlocked {
				task.BlockedBy, task.StopReason = "permission needed", StopPermissionNeeded
			}
			task.normalizedFromDone = true
			before := replacementClone(t, task)
			if err := task.ReplaceOutcome("review the API"); err != nil {
				t.Fatal(err)
			}
			if task.ID != before.ID || task.SessionID != before.SessionID || task.Version != before.Version || task.Revision != before.Revision || task.TurnRevision != before.TurnRevision || task.Attempts != before.Attempts || task.ChangeSeq != before.ChangeSeq || task.ChangedFilesCapped != before.ChangedFilesCapped || task.CreatedAt != before.CreatedAt {
				t.Fatalf("replacement lost cumulative identity or work state: %+v", task)
			}
			if task.ReplacedWorkspaceGoal != retirement {
				t.Fatal("replacement changed the pending workspace goal retirement")
			}
			if !reflect.DeepEqual(task.Turns, before.Turns) || !reflect.DeepEqual(task.ChangedFiles, before.ChangedFiles) || !reflect.DeepEqual(task.Constraints, before.Constraints) || !reflect.DeepEqual(task.Risks, before.Risks) || !reflect.DeepEqual(task.Uncertainty, before.Uncertainty) {
				t.Fatal("replacement lost turn, file, or advisory history")
			}
			if task.UpdatedAt.Before(before.UpdatedAt) || task.normalizedFromDone {
				t.Fatal("replacement retained old terminal normalization or regressed its timestamp")
			}
			assertReplacementUnproven(t, task)
			inferred := Infer("review the API")
			if task.Goal != inferred.Goal || !reflect.DeepEqual(task.Intent, inferred.Intent) || !reflect.DeepEqual(task.DefinitionOfDone, inferred.DefinitionOfDone) {
				t.Fatal("replacement contract differs from normal inference")
			}
			for i, step := range task.Steps {
				if step.Description != inferred.Steps[i] {
					t.Fatalf("step %d differs from inference", i)
				}
			}
			foundSummary := false
			for _, evidence := range task.Evidence {
				if evidence.Source == "outcome-replacement" && strings.Contains(evidence.Summary, before.Steps[0].Description) {
					foundSummary = true
				}
			}
			if !foundSummary {
				t.Fatal("recorded completed work was lost")
			}
		})
	}
}

func TestReplaceOutcomeRetainsVerificationDiagnosticsWithoutAuthority(t *testing.T) {
	task := replacementTask(t, "fix the cache")
	task.AddVerification("old failure", false, "verify FAIL — useful failure detail")
	task.AddVerification("old success", true, "verify PASS — useful success detail")
	before := append([]Verification(nil), task.Verification...)
	if err := task.ReplaceOutcome("document the API"); err != nil {
		t.Fatal(err)
	}
	if len(task.Verification) != len(before) || task.ConsecutiveVerificationFailures() != 0 {
		t.Fatal("replacement changed historical check results or retry diagnostics")
	}
	for i, verification := range task.Verification {
		if verification.Command != before[i].Command || verification.Summary != before[i].Summary || verification.At != before[i].At || verification.Passed != before[i].Passed {
			t.Fatalf("historical verification changed: %#v", verification)
		}
		if !verification.Retired || verification.trusted || verification.Observation != nil || verification.Coverage != VerificationCoverageUnbound {
			t.Fatalf("retired verification retained authority: %#v", verification)
		}
	}
	if task.VerifiedChangeSeq != -1 || !task.NeedsVerification() {
		t.Fatal("replacement did not require new verification")
	}
}

func TestReplaceOutcomeRetiresOldFailureBudget(t *testing.T) {
	task := replacementTask(t, "fix the cache")
	task.AddVerification("old check", false, "verify FAIL — old error")
	task.AddVerification("old check", false, "verify FAIL — old error")
	if task.ConsecutiveVerificationFailures() != 2 {
		t.Fatal("fixture lacks old failures")
	}
	if err := task.ReplaceOutcome("document the API"); err != nil {
		t.Fatal(err)
	}
	store := NewStore(t.TempDir())
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.Load(task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.ConsecutiveVerificationFailures() != 0 {
		t.Fatal("retired failures consumed the new outcome's retry budget")
	}
	for i := 1; i <= 2; i++ {
		reloaded.AddVerification("new check", false, "verify FAIL — new error")
		if reloaded.ConsecutiveVerificationFailures() != i {
			t.Fatal("old failure history contaminated current retry count")
		}
	}
}

func TestReplacementStoreRejectsOversizedRetirementBeforePublication(t *testing.T) {
	task := replacementTask(t, "fix the cache")
	store := NewStore(t.TempDir())
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}
	beforeRevision := task.Revision
	candidate := *task
	candidate.ReplacedWorkspaceGoal = &GoalRetirement{Text: strings.Repeat("x", maxTaskFileBytes), Revision: 1}
	if err := store.Save(&candidate); err == nil {
		t.Fatal("unrestorable retirement was published")
	}
	if candidate.Revision != beforeRevision {
		t.Fatal("failed save changed caller revision")
	}
	persisted, err := store.Load(task.SessionID)
	if err != nil || persisted.Revision != beforeRevision || persisted.ReplacedWorkspaceGoal != nil {
		t.Fatalf("oversized save damaged previous durable task: revision=%d err=%v", beforeRevision, err)
	}
}

func TestReplaceOutcomeBoundedHistory(t *testing.T) {
	task := replacementTask(t, "fix the cache")
	task.Advance()
	for i := 0; i < maxEvidence; i++ {
		task.AddEvidence(Evidence{
			Kind: EvidenceKindInspection, Status: "UNVERIFIED",
			Summary: strings.Repeat("x", i+1),
		})
	}
	before := append([]Evidence(nil), task.Evidence...)
	if err := task.ReplaceOutcome("document the API"); err != nil {
		t.Fatal(err)
	}
	if len(task.Evidence) != maxEvidence {
		t.Fatal("replacement changed the bounded evidence ledger size")
	}
	for i, evidence := range task.Evidence {
		if evidence.Summary != before[i].Summary {
			t.Fatal("completed-work summary evicted useful bounded history")
		}
	}
	assertReplacementUnproven(t, task)
}

func TestReplaceOutcomeInvalidatesPreviouslyUnrequiredQualityProof(t *testing.T) {
	for _, prompt := range []string{
		"review the latest API", "optimize performance", "improve the UI", "fix auth",
	} {
		t.Run(prompt, func(t *testing.T) {
			task := replacementTask(t, "document the API")
			task.RecordResearchEvidence("PASS", "old research retained", "old docs")
			task.RecordMeasurementEvidence("PASS", "old measurement retained", "old benchmark")
			task.RecordVisualEvidence("PASS", "old visual review retained", "old screenshot")
			task.RecordTestsEvidence("PASS", "old tests retained", "old runner")
			task.RecordApprovalEvidence("APPROVED", "old approval retained", "old user choice")
			if err := task.ReplaceOutcome(prompt); err != nil {
				t.Fatal(err)
			}
			if len(task.RequiredEvidenceKinds()) == 0 {
				t.Fatal("fixture did not add a quality requirement")
			}
			assertReplacementUnproven(t, task)
			for _, index := range task.RequiredCriterionIndices() {
				task.RecordCriterionTestsEvidence(index, "PASS", "new criterion checked", "new runner")
			}
			if task.CompletionReady() {
				t.Fatal("old quality proof satisfied a newly required proof kind")
			}
		})
	}
}

func TestReplaceOutcomeRejectsInvalidInputsWithoutMutation(t *testing.T) {
	for _, prompt := range []string{
		"", " \n\t ", "...", "hello", "instead", "also", "focus", "what is the API", "a better outcome",
		"how do I fix the cache?", "can you fix the cache", "if needed fix the cache",
		"do not fix the cache", `"fix the cache"`,
		"don't, under any circumstances, fix all tests", "don’t, under any circumstances, fix all tests",
		"never: fix all tests", "not—fix all tests",
	} {
		t.Run(prompt, func(t *testing.T) {
			task := replacementTask(t, "fix the cache")
			task.RecordCriterionVerification(0, "PASS", "old proof retained", "check cache")
			before := replacementClone(t, task)
			if err := task.ReplaceOutcome(prompt); err == nil {
				t.Fatalf("ReplaceOutcome(%q) accepted a non-tasklike request", prompt)
			}
			if !reflect.DeepEqual(task, before) {
				t.Fatalf("invalid replacement %q mutated state", prompt)
			}
		})
	}
	var absent *Task
	if err := absent.ReplaceOutcome("fix the cache"); err == nil {
		t.Fatal("nil task accepted replacement")
	}
	task := replacementTask(t, "fix the cache")
	task.SessionID = ""
	before := replacementClone(t, task)
	if err := task.ReplaceOutcome("document the API"); err == nil || !reflect.DeepEqual(task, before) {
		t.Fatal("invalid task accepted replacement or was mutated")
	}
}

func TestReplaceOutcomeRejectsExhaustedIntentIdentity(t *testing.T) {
	task := replacementTask(t, "fix the cache")
	task.IntentRevision = ^uint64(0) - 1
	if err := task.ReplaceOutcome(task.Goal); err != nil {
		t.Fatal(err)
	}
	if task.IntentRevision != ^uint64(0) {
		t.Fatal("last available intent identity was not consumed")
	}
	for _, prompt := range []string{task.Goal, "document the API"} {
		before := replacementClone(t, task)
		if err := task.ReplaceOutcome(prompt); err == nil {
			t.Fatal("exhausted intent identity was reused")
		}
		if !reflect.DeepEqual(task, before) {
			t.Fatal("rejected exhausted replacement mutated state")
		}
	}
}

func replacementTask(t *testing.T, prompt string) *Task {
	t.Helper()
	task, tasklike, err := NewFromPrompt("outcome-replacement", prompt)
	if err != nil || !tasklike || task == nil {
		t.Fatalf("create replacement fixture: tasklike=%v err=%v", tasklike, err)
	}
	return task
}

func assertReplacementUnproven(t *testing.T, task *Task) {
	t.Helper()
	if task.Status != StatusWorking || task.CurrentStep != 0 || task.BlockedBy != "" || task.StopReason != StopNone {
		t.Fatalf("replacement did not reset progress and blocking state: %+v", task)
	}
	if task.CompletionReady() || task.VerifiedChangeSeq != -1 || !task.NeedsVerification() {
		t.Fatalf("replacement retained completion authority: %#v", task.CompletionCheck())
	}
	for _, step := range task.Steps {
		if step.Done {
			t.Fatal("replacement inherited a completed step")
		}
	}
	for _, index := range task.RequiredCriterionIndices() {
		if status, current := task.CriterionEvidenceState(index); status != "UNVERIFIED" || current {
			t.Fatalf("replacement criterion %d inherited proof: %s, %v", index, status, current)
		}
	}
	for _, evidence := range task.Evidence {
		if evidence.CriterionIndex != nil || evidence.trusted || evidence.Status != "STALE" {
			t.Fatalf("old evidence was not detached and stale: %#v", evidence)
		}
	}
	for _, kind := range task.RequiredEvidenceKinds() {
		if _, current, _ := task.RequirementEvidenceState(kind); current {
			t.Fatalf("replacement retained current %s evidence", kind)
		}
	}
	if err := task.Validate(); err != nil {
		t.Fatalf("replacement is invalid: %v", err)
	}
}

func replacementClone(t *testing.T, task *Task) *Task {
	t.Helper()
	data, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	var clone Task
	if err := json.Unmarshal(data, &clone); err != nil {
		t.Fatal(err)
	}
	// Preserve runtime fields too, so no-op checks compare all model state.
	clone.normalizedFromDone = task.normalizedFromDone
	for i := range clone.Evidence {
		clone.Evidence[i].trusted = task.Evidence[i].trusted
	}
	for i := range clone.Verification {
		clone.Verification[i].trusted = task.Verification[i].trusted
	}
	return &clone
}
