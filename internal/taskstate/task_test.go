package taskstate

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/saiaathish/picogent/internal/workspace"
)

func TestNewAndProgress(t *testing.T) {
	task, err := New("session-1", "  Fix   signup tests  ", []string{" reproduce ", "", "patch", "verify"})
	if err != nil {
		t.Fatal(err)
	}
	if task.Goal != "Fix signup tests" || task.Status != StatusPlanning || len(task.Steps) != 3 {
		t.Fatalf("unexpected task: %+v", task)
	}
	if task.ID == "" || task.Version != CurrentVersion || task.CreatedAt.IsZero() || task.UpdatedAt.IsZero() {
		t.Fatalf("missing durable metadata: %+v", task)
	}
	if err := task.SetStatus(StatusVerifying); err == nil {
		t.Fatal("planning must not jump directly to verifying")
	}
	if err := task.SetStatus(StatusWorking); err != nil {
		t.Fatal(err)
	}
	if got := task.Current(); got == nil || got.Description != "reproduce" {
		t.Fatalf("current = %+v", got)
	}
	if !task.Advance() || task.CurrentStep != 1 || !task.Steps[0].Done {
		t.Fatalf("advance failed: %+v", task)
	}
	task.NoteAttempt()
	task.AddChangedFiles("./internal/a.go", "internal/a.go", " internal/b.go ", "")
	wantFiles := []string{"internal/a.go", "internal/b.go"}
	if task.Attempts != 1 || !reflect.DeepEqual(task.ChangedFiles, wantFiles) {
		t.Fatalf("attempts/files = %d %#v", task.Attempts, task.ChangedFiles)
	}
	task.AddVerification("go   test ./...", false, " one failed ")
	task.AddVerification("go test ./...", false, "still failed")
	if got := task.ConsecutiveVerificationFailures(); got != 2 {
		t.Fatalf("failures = %d", got)
	}
	task.AddVerification("go test ./...", true, "pass")
	if got := task.ConsecutiveVerificationFailures(); got != 0 {
		t.Fatalf("failures after pass = %d", got)
	}
	task.Block(" waiting   for token ")
	if task.Status != StatusBlocked || task.BlockedBy != "waiting for token" {
		t.Fatalf("block = %+v", task)
	}
	if err := task.SetStatus(StatusWorking); err != nil || task.BlockedBy != "" {
		t.Fatalf("resume = %v %+v", err, task)
	}
	// Direct assignment represents a legacy persisted terminal record. New
	// transitions to done are covered by TestSetStatusDoneRequiresCurrentProof.
	task.Status = StatusDone
	if err := task.SetStatus(StatusWorking); err == nil {
		t.Fatal("done task must be terminal")
	}
}

func TestSetStatusDoneRequiresCurrentProof(t *testing.T) {
	task, err := New("done-gate", "finish the outcome", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.DefinitionOfDone = []Criterion{{Description: "required proof", Required: true}}
	if err := task.SetStatus(StatusWorking); err != nil {
		t.Fatal(err)
	}
	if err := task.SetStatus(StatusDone); err == nil {
		t.Fatal("unproven task was allowed to become done")
	}
	if task.Status != StatusWorking {
		t.Fatalf("failed done transition changed status to %q", task.Status)
	}
	task.RecordCriterionVerification(0, "PASS", "verify PASS", "verify")
	if !task.CompletionReady() {
		t.Fatal("current criterion proof was not recognized")
	}
	if err := task.SetStatus(StatusDone); err != nil {
		t.Fatalf("proven task was rejected: %v", err)
	}
}

func TestValidateRejectsInvalidState(t *testing.T) {
	base := func() *Task {
		task, err := New("s", "goal", []string{"step"})
		if err != nil {
			t.Fatal(err)
		}
		return task
	}
	tests := []struct {
		name string
		edit func(*Task)
	}{
		{"version", func(v *Task) { v.Version++ }},
		{"id", func(v *Task) { v.ID = "" }},
		{"session", func(v *Task) { v.SessionID = "" }},
		{"goal", func(v *Task) { v.Goal = "" }},
		{"status", func(v *Task) { v.Status = "unknown" }},
		{"negative step", func(v *Task) { v.CurrentStep = -1 }},
		{"large step", func(v *Task) { v.CurrentStep = 2 }},
		{"attempts", func(v *Task) { v.Attempts = -1 }},
		{"negative change sequence", func(v *Task) { v.ChangeSeq = -1 }},
		{"invalid verified change sequence", func(v *Task) { v.VerifiedChangeSeq = -2 }},
		{"verified change sequence ahead", func(v *Task) { v.ChangeSeq, v.VerifiedChangeSeq = 1, 2 }},
		{"empty step", func(v *Task) { v.Steps[0].Description = " " }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := base()
			tt.edit(v)
			if err := v.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	if err := (*Task)(nil).Validate(); err == nil {
		t.Fatal("nil task should fail validation")
	}
}

func TestEvidenceTracksLatestChange(t *testing.T) {
	task, err := New("evidence", "fix signup", nil)
	if err != nil {
		t.Fatal(err)
	}
	if task.NeedsVerification() {
		t.Fatal("new task should not need verification")
	}

	task.RecordChanged(" ./internal/signup.go ")
	if task.ChangeSeq != 1 || !reflect.DeepEqual(task.ChangedFiles, []string{"internal/signup.go"}) {
		t.Fatalf("first change = seq %d files %#v", task.ChangeSeq, task.ChangedFiles)
	}
	if !task.NeedsVerification() {
		t.Fatal("a change must need verification")
	}

	task.AddVerification("go test ./internal/signup", true, "pass")
	if task.VerifiedChangeSeq != 1 || task.NeedsVerification() {
		t.Fatalf("passing evidence = verified %d needs=%v", task.VerifiedChangeSeq, task.NeedsVerification())
	}
	if len(task.Evidence) != 1 || task.Evidence[0].Kind != "verification" || task.Evidence[0].Status != "PASS" || task.Evidence[0].ChangeSeq != 1 {
		t.Fatalf("verification ledger = %+v", task.Evidence)
	}

	// A later edit to the same display path must invalidate the earlier pass.
	task.RecordChanged("internal/signup.go")
	if task.ChangeSeq != 2 || len(task.ChangedFiles) != 1 || !task.NeedsVerification() {
		t.Fatalf("repeat change = seq %d files %#v needs=%v", task.ChangeSeq, task.ChangedFiles, task.NeedsVerification())
	}
	task.AddVerification("go test ./internal/signup", true, "pass again")
	if task.VerifiedChangeSeq != 2 || task.NeedsVerification() {
		t.Fatalf("second pass = verified %d needs=%v", task.VerifiedChangeSeq, task.NeedsVerification())
	}
	task.AddVerification("go test ./internal/signup", false, "failed")
	if task.VerifiedChangeSeq != -1 || !task.NeedsVerification() {
		t.Fatalf("failed evidence = verified %d needs=%v", task.VerifiedChangeSeq, task.NeedsVerification())
	}

	legacy := *task
	legacy.ChangedFiles = []string{"internal/legacy.go"}
	legacy.ChangeSeq = 0
	legacy.VerifiedChangeSeq = 0
	legacy.Verification = nil
	if !legacy.NeedsVerification() {
		t.Fatal("legacy changed files without a sequence must remain unverified")
	}
	legacy.Status = StatusDone
	if !legacy.InitializeChangeSequence() || legacy.ChangeSeq != 1 || !legacy.NeedsVerification() {
		t.Fatalf("legacy migration = %+v", legacy)
	}
	if err := legacy.SetStatus(StatusVerifying); err != nil {
		t.Fatalf("unverified legacy done task should reopen for verification: %v", err)
	}
	legacy.AddVerification("go test ./internal/legacy", true, "pass")
	if legacy.NeedsVerification() {
		t.Fatalf("migrated passing evidence should cover legacy state: %+v", legacy)
	}
	if legacy.InitializeChangeSequence() {
		t.Fatal("legacy sequence migration must be idempotent")
	}
}

func TestInvalidateWorkspaceEvidenceIsIdempotent(t *testing.T) {
	task, err := New("undo-invalidation", "restore the workspace safely", []string{"edit", "verify"})
	if err != nil {
		t.Fatal(err)
	}
	if err := task.SetStatus(StatusWorking); err != nil {
		t.Fatal(err)
	}
	sequence, ok := task.BeginTurn(TurnRouteImplement)
	if !ok {
		t.Fatal("turn did not start")
	}
	task.RecordChanged("note.txt")
	if !task.FinishTurn(sequence, TurnRouteImplement, "edit the note", "PASS", StopNone, 1, 1) {
		t.Fatal("turn did not finish")
	}
	task.AddVerification("go test ./...", true, "verify PASS")
	originalFiles := append([]string(nil), task.ChangedFiles...)
	originalTurns := append([]TurnRecord(nil), task.Turns...)

	if !task.InvalidateWorkspaceEvidence("undo restored workspace files") {
		t.Fatal("passing workspace evidence was not invalidated")
	}
	if task.VerifiedChangeSeq != -1 || !task.NeedsVerification() || task.Verification[len(task.Verification)-1].Passed {
		t.Fatalf("invalidated task = %#v", task)
	}
	if !reflect.DeepEqual(task.ChangedFiles, originalFiles) || task.ChangeSeq != 1 || !reflect.DeepEqual(task.Turns, originalTurns) {
		t.Fatalf("undo invalidation changed history = files %#v seq %d turns %#v", task.ChangedFiles, task.ChangeSeq, task.Turns)
	}
	beforeRetry, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	if task.InvalidateWorkspaceEvidence("undo restored workspace files") {
		t.Fatal("second invalidation reported a change")
	}
	afterRetry, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(beforeRetry, afterRetry) {
		t.Fatal("second invalidation changed durable state")
	}
}

func TestCompletionReadyRequiresCurrentPassForEveryRequiredCriterion(t *testing.T) {
	task, err := New("criterion-proof", "finish the outcome", []string{"first", "second", "optional"})
	if err != nil {
		t.Fatal(err)
	}
	task.DefinitionOfDone = []Criterion{
		{Description: "first required", Required: true},
		{Description: "second required", Required: true},
		{Description: "optional polish", Required: false},
	}

	if task.CompletionReady() {
		t.Fatal("missing criterion evidence incorrectly completed the task")
	}
	task.RecordCriterionVerification(1, "PASS", "second passed", "verify")
	if got := task.FirstMissingRequiredCriterion(); got != 0 {
		t.Fatalf("first missing criterion = %d, want 0", got)
	}
	if task.CompletionReady() {
		t.Fatal("one required criterion incorrectly completed the task")
	}

	task.RecordCriterionVerification(0, "FAIL", "first failed", "verify")
	if status, current := task.CriterionEvidenceState(0); status != "FAIL" || !current {
		t.Fatalf("failed current evidence = status=%q current=%v", status, current)
	}
	if task.CompletionReady() {
		t.Fatal("failed required criterion incorrectly completed the task")
	}

	task.RecordCriterionVerification(0, "PASS", "first passed", "verify")
	if !task.CompletionReady() {
		t.Fatal("all required criteria passed but task is not completion-ready")
	}

	// Optional criteria are observable but never block completion.
	task.RecordCriterionVerification(2, "FAIL", "optional failed", "verify")
	if !task.CompletionReady() {
		t.Fatal("optional failed criterion blocked completion")
	}

	// Any later mutation makes every earlier criterion-bound pass stale.
	task.RecordChanged("first")
	if status, current := task.CriterionEvidenceState(0); status != "UNVERIFIED" || current {
		t.Fatalf("stale criterion evidence = status=%q current=%v", status, current)
	}
	if task.CompletionReady() {
		t.Fatal("stale criterion evidence incorrectly completed the task")
	}

	task.RecordCriterionVerification(0, "PASS", "first rerun passed", "verify")
	if task.CompletionReady() {
		t.Fatal("one stale required criterion incorrectly completed the task")
	}
	task.RecordCriterionVerification(1, "PASS", "second rerun passed", "verify")
	if !task.CompletionReady() {
		t.Fatal("all required criteria lack completion after current rerun")
	}
}

func TestCompletionCheckRequiresCurrentTrustedEvidenceForEachRequirement(t *testing.T) {
	task, err := New("quality-proof", "finish the quality outcome", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.Intent = &IntentContract{
		Outcome:          task.Goal,
		NeedsResearch:    true,
		NeedsMeasurement: true,
		NeedsVisual:      true,
		NeedsTests:       true,
		NeedsApproval:    true,
	}

	wantKinds := []EvidenceKind{
		EvidenceKindResearch,
		EvidenceKindMeasurement,
		EvidenceKindVisual,
		EvidenceKindTests,
		EvidenceKindApproval,
	}
	check := task.CompletionCheck()
	if check.Ready || !reflect.DeepEqual(check.MissingRequirements, wantKinds) {
		t.Fatalf("missing quality proof = %#v, want all %v", check, wantKinds)
	}
	if len(check.Requirements) != len(wantKinds) {
		t.Fatalf("requirement states = %#v", check.Requirements)
	}
	for _, requirement := range check.Requirements {
		if requirement.Status != "UNVERIFIED" || requirement.Current {
			t.Fatalf("missing requirement state = %#v", requirement)
		}
	}

	// A model-labelled record and a record from the wrong producer are retained
	// for auditability but cannot satisfy a quality requirement.
	task.AddRequirementEvidence(EvidenceKindResearch, "PASS", EvidenceOriginModel, "the model says research is complete", "model")
	task.AddRequirementEvidence(EvidenceKindMeasurement, "PASS", EvidenceOriginResearchTool, "research output claimed a measurement", "research")
	task.AddRequirementEvidence(EvidenceKindVisual, "PASS", EvidenceOriginVisualInspection, "generic caller labels visual proof", "caller")
	if status, current, origin := task.RequirementEvidenceState(EvidenceKindVisual); status != "PASS" || current || origin != EvidenceOriginVisualInspection {
		t.Fatalf("generic allow-listed visual proof was trusted = status=%q current=%v origin=%q", status, current, origin)
	}
	// The remaining three records use the matching kind and trusted producer.
	task.RecordVisualEvidence("PASS", "rendered result inspected", "browser")
	task.RecordTestsEvidence("PASS", "targeted tests passed", "test runner")
	task.RecordApprovalEvidence("APPROVED", "user approved the bounded action", "user")

	check = task.CompletionCheck()
	if check.Ready || !reflect.DeepEqual(check.MissingRequirements, []EvidenceKind{EvidenceKindResearch, EvidenceKindMeasurement}) {
		t.Fatalf("untrusted quality proof was accepted = %#v", check)
	}
	if status, current, origin := task.RequirementEvidenceState(EvidenceKindResearch); status != "PASS" || current || origin != EvidenceOriginModel {
		t.Fatalf("model research proof = status=%q current=%v origin=%q", status, current, origin)
	}
	if status, current, origin := task.RequirementEvidenceState(EvidenceKindMeasurement); status != "PASS" || current || origin != EvidenceOriginResearchTool {
		t.Fatalf("wrong measurement producer = status=%q current=%v origin=%q", status, current, origin)
	}

	// Matching proof kind and origin closes the two remaining gaps.
	task.RecordResearchEvidence("PASS", "current API research completed", "research tool")
	task.RecordMeasurementEvidence("PASS", "baseline and post-change measurements recorded", "benchmark")
	check = task.CompletionCheck()
	if !check.Ready || len(check.MissingRequirements) != 0 {
		t.Fatalf("trusted quality proof did not complete the criterionless outcome = %#v", check)
	}

	// A later mutation makes every earlier quality record stale, even though the
	// producer and proof kind were previously trusted.
	task.RecordChanged("quality.go")
	check = task.CompletionCheck()
	if check.Ready || !reflect.DeepEqual(check.MissingRequirements, wantKinds) {
		t.Fatalf("stale quality proof remained current = %#v", check)
	}
}

func TestCriterionEvidenceRequiresTrustedKindAndOrigin(t *testing.T) {
	task, err := New("criterion-trust", "finish the bounded outcome", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.DefinitionOfDone = []Criterion{{Description: "required proof", Required: true}}

	// A typed verifier record establishes the expected proof boundary.
	task.RecordCriterionVerification(0, "PASS", "trusted verification passed", "verify")
	if !task.CompletionReady() {
		t.Fatal("trusted criterion evidence did not satisfy completion")
	}

	// A later generic caller-labelled PASS is retained, but must supersede the
	// older trusted record as the latest untrusted state rather than reopening
	// the completion boundary with a false positive.
	task.AddEvidenceForCriterion(0, Evidence{
		Kind:      EvidenceKindVerification,
		Origin:    EvidenceOriginVerifier,
		Status:    "PASS",
		Summary:   "the caller says the criterion passed",
		ChangeSeq: task.ChangeSeq,
	})
	if status, current := task.CriterionEvidenceState(0); status != "UNVERIFIED" || current || task.CompletionReady() {
		t.Fatalf("model-origin criterion evidence was trusted: status=%q current=%v ready=%v", status, current, task.CompletionReady())
	}

	// A verifier origin cannot upgrade an unrelated evidence kind into
	// criterion proof. Only the typed verification/tests proof boundary is
	// accepted.
	task.AddEvidenceForCriterion(0, Evidence{
		Kind:      EvidenceKindInspection,
		Origin:    EvidenceOriginVerifier,
		Status:    "PASS",
		Summary:   "inspection completed",
		ChangeSeq: task.ChangeSeq,
	})
	if status, current := task.CriterionEvidenceState(0); status != "UNVERIFIED" || current || task.CompletionReady() {
		t.Fatalf("wrong-kind criterion evidence was trusted: status=%q current=%v ready=%v", status, current, task.CompletionReady())
	}

	task.RecordCriterionTestsEvidence(0, "PASS", "criterion test passed", "test runner")
	if !task.CompletionReady() {
		t.Fatal("trusted test-runner criterion evidence did not satisfy completion")
	}
}

func TestRequirementEvidenceRejectsMismatchedProofKind(t *testing.T) {
	task, err := New("kind-proof", "run the required tests", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.Intent = &IntentContract{Outcome: task.Goal, NeedsTests: true}

	task.AddRequirementEvidence(EvidenceKindResearch, "PASS", EvidenceOriginResearchTool, "research completed", "docs")
	if status, current, _ := task.RequirementEvidenceState(EvidenceKindTests); status != "UNVERIFIED" || current {
		t.Fatalf("research evidence satisfied tests = status=%q current=%v", status, current)
	}

	task.AddRequirementEvidence(EvidenceKindTests, "PASS", EvidenceOriginModel, "the model says tests passed", "model")
	if status, current, origin := task.RequirementEvidenceState(EvidenceKindTests); status != "PASS" || current || origin != EvidenceOriginModel {
		t.Fatalf("untrusted test proof = status=%q current=%v origin=%q", status, current, origin)
	}
	if task.CompletionReady() {
		t.Fatal("mismatched or model-origin proof completed the outcome")
	}

	task.RecordTestsEvidence("PASS", "go test passed", "go test ./...")
	if !task.CompletionReady() {
		t.Fatal("trusted test proof did not complete the requirement-only outcome")
	}
}

func TestCriterionEvidenceRejectsWrongIndexAndRoundTrips(t *testing.T) {
	task, err := New("criterion-json", "verify the result", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.DefinitionOfDone = []Criterion{{Description: "required", Required: true}}
	task.AddEvidenceForCriterion(1, Evidence{Kind: "verification", Status: "PASS", Summary: "wrong criterion", ChangeSeq: task.ChangeSeq})
	if task.CompletionReady() || len(task.Evidence) != 0 {
		t.Fatalf("out-of-range criterion evidence was accepted: %#v", task.Evidence)
	}
	task.RecordCriterionVerification(0, "PASS", "required passed", "verify")
	data, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	var restored Task
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if err := restored.Validate(); err != nil {
		t.Fatalf("criterion evidence round-trip invalid: %v", err)
	}
	if len(restored.Evidence) != 1 || restored.Evidence[0].CriterionIndex == nil || *restored.Evidence[0].CriterionIndex != 0 || restored.CompletionReady() {
		t.Fatalf("criterion evidence round-trip = %#v", restored)
	}

	bad := restored
	bad.Evidence = append([]Evidence(nil), restored.Evidence...)
	index := 1
	bad.Evidence[0].CriterionIndex = &index
	if err := bad.Validate(); err == nil {
		t.Fatal("invalid criterion index survived validation")
	}
}

func TestCriterionlessCompletionRequiresWorkspaceBoundVerification(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "checked.txt")
	if err := os.WriteFile(path, []byte("checked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	observation, err := workspace.Capture(context.Background(), root, []string{"checked.txt"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := New("criterionless-proof", "check the workspace", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.RecordChanged("checked.txt")
	task.AddVerificationWithObservation("go test ./...", true, "verify PASS", &observation)
	if !task.CompletionReady() {
		t.Fatal("current workspace-bound verification did not complete criterion-less task")
	}
	task.AddVerification("go test ./...", true, "verify PASS")
	if task.CompletionReady() {
		t.Fatal("unbound later verification replaced workspace-bound completion proof")
	}
}

func TestAllOptionalCriteriaRequireWorkspaceBoundVerification(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "optional.txt")
	if err := os.WriteFile(path, []byte("optional\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	observation, err := workspace.Capture(context.Background(), root, []string{"optional.txt"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := New("optional-proof", "check the optional outcome", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.DefinitionOfDone = []Criterion{{Description: "nice to have", Required: false}}
	if task.CompletionReady() {
		t.Fatal("all-optional definition was vacuously completion-ready")
	}
	task.AddEvidenceForCriterion(0, Evidence{Kind: "verification", Origin: EvidenceOriginVerifier, Status: "PASS", Summary: "optional passed", ChangeSeq: task.ChangeSeq})
	if task.CompletionReady() {
		t.Fatal("optional criterion evidence replaced aggregate completion proof")
	}

	task.RecordChanged("optional.txt")
	task.AddVerificationWithObservation("go test ./...", true, "verify PASS", &observation)
	if !task.CompletionReady() {
		t.Fatal("workspace-bound passing verification did not complete all-optional task")
	}
}

func TestPartialVerificationCannotCompleteRequiredOrCriterionlessTask(t *testing.T) {
	observation := workspace.Observation{Files: []workspace.FileObservation{{Path: "current.go", Known: true}}}

	required, err := New("partial-required", "finish the required outcome", nil)
	if err != nil {
		t.Fatal(err)
	}
	required.DefinitionOfDone = []Criterion{{Description: "required proof", Required: true}}
	required.RecordChanged("current.go")
	required.AddVerificationForCriteriaWithCoverage([]int{0}, "verify", true, "verify PASS", &observation, VerificationCoveragePartial)
	latest := required.Verification[len(required.Verification)-1]
	if latest.Passed || latest.Coverage != VerificationCoveragePartial {
		t.Fatalf("partial required verification = %#v", latest)
	}
	if status, current := required.CriterionEvidenceState(0); status == "PASS" || !current || required.CompletionReady() {
		t.Fatalf("partial required evidence = status=%q current=%v ready=%v", status, current, required.CompletionReady())
	}

	criterionless, err := New("partial-criterionless", "finish the criterionless outcome", nil)
	if err != nil {
		t.Fatal(err)
	}
	criterionless.RecordChanged("current.go")
	criterionless.AddVerificationForCriteriaWithCoverage(nil, "verify", true, "verify PASS", &observation, VerificationCoveragePartial)
	if criterionless.CompletionReady() {
		t.Fatal("partial criterionless verification completed the task")
	}
}
