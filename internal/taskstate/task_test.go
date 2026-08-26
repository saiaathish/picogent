package taskstate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
	if err := task.SetStatus(StatusDone); err != nil {
		t.Fatal(err)
	}
	if err := task.SetStatus(StatusWorking); err == nil {
		t.Fatal("done task must be terminal")
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

func TestVerificationObservationBindsAndInvalidatesPassingEvidence(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "fixed.txt")
	if err := os.WriteFile(path, []byte("fixed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	observation, err := workspace.Capture(context.Background(), root, []string{"fixed.txt"})
	if err != nil {
		t.Fatal(err)
	}
	task, err := New("observation-binding", "fix the file", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.RecordChanged("fixed.txt")
	task.AddVerificationWithObservation("go test ./...", true, "verify PASS\n1 passed", &observation)
	if task.NeedsVerification() || task.Verification[0].Observation == nil {
		t.Fatalf("bound passing evidence = %#v", task)
	}
	// The task owns an isolated observation rather than the caller's slice.
	observation.Files[0].Digest = strings.Repeat("0", 64)
	if task.Verification[0].Observation.Files[0].Digest == observation.Files[0].Digest {
		t.Fatal("verification observation aliases caller data")
	}
	if !task.InvalidateLatestVerification("workspace content changed") {
		t.Fatal("passing evidence was not invalidated")
	}
	if task.Verification[0].Passed || !strings.HasPrefix(task.Verification[0].Summary, "verify INCONCLUSIVE") || !task.NeedsVerification() {
		t.Fatalf("invalidated evidence = %#v", task.Verification[0])
	}
	if len(task.Evidence) != 2 || task.Evidence[1].Status != "INCONCLUSIVE" {
		t.Fatalf("invalidation ledger = %#v", task.Evidence)
	}
}

func TestVerificationEvidenceUsesCanonicalStatuses(t *testing.T) {
	tests := []struct {
		name       string
		passed     bool
		summary    string
		wantStatus string
		wantPassed bool
	}{
		{name: "inconclusive", passed: false, summary: "verify INCONCLUSIVE timeout", wantStatus: "INCONCLUSIVE"},
		{name: "skipped", passed: false, summary: "verify SKIPPED no runner", wantStatus: "SKIPPED"},
		{name: "lookalike pass", passed: true, summary: "verify PASSIVE output", wantStatus: "INCONCLUSIVE"},
		{name: "legacy bool", passed: true, summary: "pass", wantStatus: "PASS", wantPassed: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task, err := New("verification-status-"+tt.name, "goal", nil)
			if err != nil {
				t.Fatal(err)
			}
			task.AddVerification("verify", tt.passed, tt.summary)
			if len(task.Evidence) != 1 || task.Evidence[0].Status != tt.wantStatus {
				t.Fatalf("evidence = %+v, want status %s", task.Evidence, tt.wantStatus)
			}
			if got := task.Verification[0].Passed; got != tt.wantPassed {
				t.Fatalf("passed = %v, want %v", got, tt.wantPassed)
			}
		})
	}
}

func TestValidateRejectsMalformedPersistedObservation(t *testing.T) {
	task, err := New("malformed-observation", "goal", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.RecordChanged("missing.txt")
	task.Verification = []Verification{{
		Passed:  true,
		Summary: "verify PASS",
		Observation: &workspace.Observation{
			Root:  t.TempDir(),
			Files: []workspace.FileObservation{{Path: "missing.txt", Known: true, Size: 1}},
		},
	}}
	if err := task.Validate(); err == nil {
		t.Fatal("malformed persisted observation should fail validation")
	}
}

func TestOutcomeNotesAndEvidenceStayBoundedAndDeduplicated(t *testing.T) {
	task, err := New("outcome", "finish the project", nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxOutcomeNotes+2; i++ {
		task.AddConstraint("constraint " + string(rune('a'+i)))
		task.AddRisk("risk " + string(rune('a'+i)))
		task.AddUncertainty("unknown " + string(rune('a'+i)))
	}
	if len(task.Constraints) != maxOutcomeNotes || len(task.Risks) != maxOutcomeNotes || len(task.Uncertainty) != maxOutcomeNotes {
		t.Fatalf("bounded notes = constraints=%d risks=%d uncertainty=%d", len(task.Constraints), len(task.Risks), len(task.Uncertainty))
	}
	task.AddEvidence(Evidence{Kind: "inspection", Status: "CONFIRMED", Summary: "repo map found the service", Reference: "internal/repomap", ChangeSeq: 0})
	got := len(task.Evidence)
	task.AddEvidence(task.Evidence[0])
	if len(task.Evidence) != got {
		t.Fatalf("duplicate evidence appended: %+v", task.Evidence)
	}
	if err := task.Validate(); err != nil {
		t.Fatalf("bounded outcome invalid: %v", err)
	}
}

func TestEvidenceValidationRejectsUnboundedState(t *testing.T) {
	task, err := New("outcome-invalid", "goal", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.Evidence = make([]Evidence, maxEvidence+1)
	if err := task.Validate(); err == nil {
		t.Fatal("oversized evidence should fail validation")
	}
}

func TestDurableCollectionsStayBounded(t *testing.T) {
	task, err := New("bounds", "goal", nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxChangedFiles+1; i++ {
		task.RecordChanged("file-" + string(rune('a'+i%26)) + ".go-" + fmt.Sprint(i))
	}
	if len(task.ChangedFiles) != maxChangedFiles || !task.ChangedFilesCapped {
		t.Fatalf("changed files = %d capped=%v", len(task.ChangedFiles), task.ChangedFilesCapped)
	}
	for i := 0; i < maxVerification+4; i++ {
		task.AddVerification("verify", false, "failure "+fmt.Sprint(i))
	}
	if len(task.Verification) != maxVerification {
		t.Fatalf("verification history = %d", len(task.Verification))
	}
	if err := task.Validate(); err != nil {
		t.Fatalf("bounded task invalid: %v", err)
	}
}

func TestValidateRejectsUnboundedLegacyCollections(t *testing.T) {
	task, err := New("legacy-bounds", "goal", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.Steps = make([]Step, maxTaskSteps+1)
	if err := task.Validate(); err == nil {
		t.Fatal("oversized steps should fail validation")
	}
	task.Steps = nil
	task.ChangedFiles = make([]string, maxChangedFiles+1)
	if err := task.Validate(); err == nil {
		t.Fatal("oversized changed files should fail validation")
	}
	task.ChangedFiles = nil
	task.Intent = &IntentContract{Outcome: task.Goal, Action: strings.Repeat("a", maxIntentAction+1)}
	if err := task.Validate(); err == nil {
		t.Fatal("oversized intent metadata should fail validation")
	}
	task.Intent = nil
	task.Evidence = []Evidence{{Kind: "inspection", Status: "CONFIRMED", Summary: "bounded", Reference: strings.Repeat("a", maxEvidenceReference+1)}}
	if err := task.Validate(); err == nil {
		t.Fatal("oversized evidence metadata should fail validation")
	}
}

func TestEvidenceJSONRoundTrip(t *testing.T) {
	task, err := New("evidence-json", "fix signup", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.RecordChanged("internal/signup.go")
	task.AddVerification("go test ./internal/signup", true, "pass")

	data, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	var restored Task
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.ChangeSeq != 1 || restored.VerifiedChangeSeq != 1 || restored.NeedsVerification() {
		t.Fatalf("restored evidence = %+v", restored)
	}
}

func TestStatusValidExhaustive(t *testing.T) {
	for _, status := range []Status{StatusPlanning, StatusWorking, StatusVerifying, StatusBlocked, StatusDone} {
		if !status.Valid() {
			t.Fatalf("%q should be valid", status)
		}
	}
	if Status("paused").Valid() {
		t.Fatal("unknown status should be invalid")
	}
}

func TestAdvanceAtEndAndNilHelpers(t *testing.T) {
	task, err := New("s", "goal", nil)
	if err != nil {
		t.Fatal(err)
	}
	if task.Advance() || task.Current() != nil {
		t.Fatal("empty task should have no current step")
	}
	var nilTask *Task
	nilTask.NoteAttempt()
	nilTask.AddChangedFiles("x")
	nilTask.RecordChanged("x")
	nilTask.AddVerification("x", false, "x")
	nilTask.Block("x")
	if nilTask.Advance() || nilTask.Current() != nil || nilTask.ConsecutiveVerificationFailures() != 0 || nilTask.NeedsVerification() {
		t.Fatal("nil helpers must be safe")
	}
}
