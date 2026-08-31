package outcome

import (
	"reflect"
	"testing"

	"github.com/saiaathish/picogent/internal/projecthealth"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/workspace"
)

// TestCompletionContractMatrix keeps the completion decision and its derived
// engine/router projections on one executable table. The larger integration
// slice can use these cases when it wires the GUI, TUI, and headless surfaces
// to the shared contract.
func TestCompletionContractMatrix(t *testing.T) {
	newTask := func(t *testing.T, goal string) *taskstate.Task {
		t.Helper()
		task, err := taskstate.New("completion-contract", goal, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := task.SetStatus(taskstate.StatusWorking); err != nil {
			t.Fatal(err)
		}
		return task
	}

	tests := []struct {
		name              string
		build             func(*testing.T) *taskstate.Task
		wantReady         bool
		wantState         State
		wantNext          Kind
		wantStop          StopPolicy
		wantMissing       []int
		wantRequirements  []taskstate.EvidenceKind
		wantVerifyCurrent bool
	}{
		{
			name:      "no durable task",
			build:     func(*testing.T) *taskstate.Task { return nil },
			wantState: StateNoOutcome,
			wantNext:  KindInspect,
			wantStop:  StopPause,
		},
		{
			name: "blocked task",
			build: func(*testing.T) *taskstate.Task {
				return &taskstate.Task{Status: taskstate.StatusBlocked}
			},
			wantState: StateBlocked,
			wantNext:  KindBlocked,
			wantStop:  StopPause,
		},
		{
			name: "required criterion missing",
			build: func(t *testing.T) *taskstate.Task {
				task := newTask(t, "finish the outcome")
				task.DefinitionOfDone = []taskstate.Criterion{{Description: "required proof", Required: true}}
				return task
			},
			wantState:   StateWorking,
			wantNext:    KindCriterion,
			wantStop:    StopContinue,
			wantMissing: []int{0},
		},
		{
			name: "required criterion passes",
			build: func(t *testing.T) *taskstate.Task {
				task := newTask(t, "finish the outcome")
				task.DefinitionOfDone = []taskstate.Criterion{{Description: "required proof", Required: true}}
				task.RecordCriterionVerification(0, "PASS", "required proof passed", "verify")
				return task
			},
			wantReady: true,
			wantState: StateInspect,
			wantNext:  KindInspect,
			wantStop:  StopRecheck,
		},
		{
			name: "definition of done outranks drifted legacy steps",
			build: func(t *testing.T) *taskstate.Task {
				task := newTask(t, "finish the outcome")
				task.Steps = []taskstate.Step{{Description: "legacy progress", Done: true}}
				task.CurrentStep = 1
				task.DefinitionOfDone = []taskstate.Criterion{
					{Description: "legacy progress", Required: true},
					{Description: "new required proof", Required: true},
				}
				task.RecordCriterionVerification(0, "PASS", "legacy progress passed", "verify")
				return task
			},
			wantState:   StateWorking,
			wantNext:    KindCriterion,
			wantStop:    StopContinue,
			wantMissing: []int{1},
		},
		{
			name: "later mutation invalidates criterion proof",
			build: func(t *testing.T) *taskstate.Task {
				task := newTask(t, "finish the outcome")
				task.DefinitionOfDone = []taskstate.Criterion{{Description: "required proof", Required: true}}
				task.RecordCriterionVerification(0, "PASS", "required proof passed", "verify")
				task.RecordChanged("internal/change.go")
				return task
			},
			wantState:   StateVerify,
			wantNext:    KindVerify,
			wantStop:    StopContinue,
			wantMissing: []int{0},
		},
		{
			name: "quality requirement missing",
			build: func(t *testing.T) *taskstate.Task {
				task := newTask(t, "make the outcome reliable")
				task.Intent = &taskstate.IntentContract{Outcome: task.Goal, NeedsTests: true}
				return task
			},
			wantState:         StateWorking,
			wantNext:          KindRequirement,
			wantStop:          StopContinue,
			wantRequirements:  []taskstate.EvidenceKind{taskstate.EvidenceKindTests},
			wantVerifyCurrent: false,
		},
		{
			name: "partial verification cannot authorize completion",
			build: func(t *testing.T) *taskstate.Task {
				task := newTask(t, "verify the current workspace")
				task.RecordChanged("internal/change.go")
				observation := &workspace.Observation{
					Files: []workspace.FileObservation{{Path: "internal/change.go", Known: true}},
				}
				task.AddVerificationForCriteriaWithCoverage(nil, "go test ./...", true, "verify PASS", observation, taskstate.VerificationCoveragePartial)
				return task
			},
			wantState:         StateVerify,
			wantNext:          KindVerify,
			wantStop:          StopContinue,
			wantVerifyCurrent: false,
		},
		{
			name: "done marker without workspace-bound proof",
			build: func(t *testing.T) *taskstate.Task {
				task := newTask(t, "verify the workspace")
				task.RecordChanged("internal/change.go")
				task.AddVerification("go test ./...", true, "verify PASS")
				// Preserve a legacy terminal marker. The shared contract must force a
				// fresh workspace-bound recheck instead of claiming completion.
				task.Status = taskstate.StatusDone
				return task
			},
			wantState:         StateRecheck,
			wantNext:          KindInspect,
			wantStop:          StopContinue,
			wantVerifyCurrent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := tt.build(t)
			before := EvaluateCompletion(task)
			contract := Build(task, projecthealth.Report{Schema: projecthealth.Schema})
			sharedTurn := TurnContractForTask(task)

			if before.Ready != tt.wantReady {
				t.Fatalf("completion ready = %v, want %v: %#v", before.Ready, tt.wantReady, before)
			}
			if !reflect.DeepEqual(before, contract.Completion) {
				t.Fatalf("engine completion diverged from evaluator: evaluator=%#v engine=%#v", before, contract.Completion)
			}
			if contract.CompletionReady != tt.wantReady || sharedTurn.CompletionReady != tt.wantReady {
				t.Fatalf("completion projections diverged: engine=%v turn=%v want=%v", contract.CompletionReady, sharedTurn.CompletionReady, tt.wantReady)
			}
			if contract.State != tt.wantState || contract.Next.Kind != tt.wantNext || contract.Stop.Policy != tt.wantStop {
				t.Fatalf("routing projection = state=%q next=%q stop=%q, want state=%q next=%q stop=%q", contract.State, contract.Next.Kind, contract.Stop.Policy, tt.wantState, tt.wantNext, tt.wantStop)
			}
			if !reflect.DeepEqual(contract.Completion.MissingCriteria, tt.wantMissing) {
				t.Fatalf("missing criteria = %v, want %v", contract.Completion.MissingCriteria, tt.wantMissing)
			}
			if !reflect.DeepEqual(contract.Completion.MissingRequirements, tt.wantRequirements) {
				t.Fatalf("missing requirements = %v, want %v", contract.Completion.MissingRequirements, tt.wantRequirements)
			}
			if contract.Completion.VerificationCurrent != tt.wantVerifyCurrent {
				t.Fatalf("verification current = %v, want %v", contract.Completion.VerificationCurrent, tt.wantVerifyCurrent)
			}
			if after := EvaluateCompletion(task); !reflect.DeepEqual(before, after) {
				t.Fatalf("completion evaluation or projection mutated task state: before=%#v after=%#v", before, after)
			}
		})
	}
}
