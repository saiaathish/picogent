package taskstate

import (
	"reflect"
	"testing"
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
	nilTask.AddVerification("x", false, "x")
	nilTask.Block("x")
	if nilTask.Advance() || nilTask.Current() != nil || nilTask.ConsecutiveVerificationFailures() != 0 {
		t.Fatal("nil helpers must be safe")
	}
}
