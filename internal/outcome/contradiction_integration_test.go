package outcome

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/saiaathish/picogent/internal/projecthealth"
	"github.com/saiaathish/picogent/internal/taskstate"
)

func TestBuildRoutesConfirmedContradictionAndKeepsCompletionAuthoritative(t *testing.T) {
	task, err := taskstate.New("contradiction-integration", "finish the outcome", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.DefinitionOfDone = []taskstate.Criterion{{Description: "required proof", Required: true}}
	task.RecordCriterionVerification(0, "PASS", "required proof passed", "verify")
	task.RecordCriterionVerification(0, "FAIL", "required proof failed", "verify")

	contract := Build(task, projecthealth.Report{Schema: projecthealth.Schema})
	if contract.Contradictions.State != ContradictionConfirmed || len(contract.Contradictions.Signals) != 1 {
		t.Fatalf("contradiction report = %#v", contract.Contradictions)
	}
	if contract.Contradictions.Signals[0].Scope != ContradictionScopeCriterion || contract.Contradictions.Signals[0].CriterionIndex != 0 {
		t.Fatalf("criterion contradiction signal = %#v", contract.Contradictions.Signals[0])
	}
	if contract.Next.Kind != KindContradiction || contract.State != StateDiagnose {
		t.Fatalf("contradiction routing = state=%q next=%#v", contract.State, contract.Next)
	}
	if contract.Stop.Policy != StopRecheck || contract.Stop.EvidenceState != string(ContradictionConfirmed) || contract.Stop.Reason != contradictionReason {
		t.Fatalf("contradiction stop = %#v", contract.Stop)
	}
	if contract.CompletionReady || contract.Completion.Ready {
		t.Fatalf("failed criterion incorrectly authorized completion = %#v", contract.Completion)
	}
	if !reflect.DeepEqual(contract.Turn.Contradictions, contract.Contradictions) || !contract.Turn.NeedsRecovery() {
		t.Fatalf("turn projection lost confirmed contradiction = %#v", contract.Turn)
	}
	if !reflect.DeepEqual(contract.Completion, EvaluateCompletion(task)) {
		t.Fatalf("engine changed completion authority = engine=%#v taskstate=%#v", contract.Completion, EvaluateCompletion(task))
	}

	task.RecordCriterionVerification(0, "PASS", "required proof passed again", "verify")
	contract = Build(task, projecthealth.Report{Schema: projecthealth.Schema})
	if !contract.CompletionReady || !contract.Completion.Ready {
		t.Fatalf("current criterion proof was not accepted by completion evaluator = %#v", contract.Completion)
	}
	// The historical FAIL remains a same-generation disagreement. Completion is
	// still reported directly from taskstate, while the engine keeps the safe
	// contradiction route visible until the conflict is rechecked.
	if contract.Next.Kind != KindContradiction || contract.Stop.Policy != StopRecheck {
		t.Fatalf("resolved criterion contradiction was silently discarded = next=%#v stop=%#v", contract.Next, contract.Stop)
	}
}

func TestSelectRoutesAggregateRequirementContradiction(t *testing.T) {
	task, err := taskstate.New("aggregate-contradiction", "make the outcome reliable", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.Intent = &taskstate.IntentContract{Outcome: task.Goal, NeedsTests: true}
	task.RecordTestsEvidence("PASS", "tests passed", "test runner")
	task.RecordTestsEvidence("FAIL", "tests failed", "test runner")

	decision := Select(task, projecthealth.Report{Schema: projecthealth.Schema})
	if decision.Kind != KindContradiction || decision.Action != contradictionAction || decision.Reason != contradictionReason {
		t.Fatalf("aggregate contradiction decision = %#v", decision)
	}
	if decision.EvidenceState != string(ContradictionConfirmed) || decision.Confidence != "high" {
		t.Fatalf("aggregate contradiction metadata = %#v", decision)
	}
}

func TestOptionalCriterionContradictionRemainsNonExecutable(t *testing.T) {
	task, err := taskstate.New("optional-contradiction", "finish the outcome", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.DefinitionOfDone = []taskstate.Criterion{{Description: "optional polish", Required: false}}
	task.RecordCriterionVerification(0, "PASS", "optional polish passed", "verify")
	task.RecordCriterionVerification(0, "FAIL", "optional polish failed", "verify")

	contract := Build(task, projecthealth.Report{Schema: projecthealth.Schema})
	if contract.Contradictions.State != ContradictionConfirmed || len(contract.Contradictions.Signals) != 1 {
		t.Fatalf("optional contradiction report = %#v", contract.Contradictions)
	}
	if contract.Next.Kind == KindContradiction {
		t.Fatalf("optional criterion selected an executable contradiction route = next=%#v", contract.Next)
	}
	if strings.Contains(EngineInstruction(contract), "Contradiction route:") {
		t.Fatal("optional criterion contradiction became a contradiction instruction")
	}
}

func TestConfirmedContradictionDoesNotOverrideBlockedOrFreshVerification(t *testing.T) {
	tests := []struct {
		name string
		make func(*testing.T) *taskstate.Task
		want Kind
	}{
		{
			name: "blocked task",
			make: func(t *testing.T) *taskstate.Task {
				task, err := taskstate.New("blocked-contradiction", "finish safely", nil)
				if err != nil {
					t.Fatal(err)
				}
				task.Block("permission needed")
				task.RecordTestsEvidence("PASS", "tests passed", "test runner")
				task.RecordTestsEvidence("FAIL", "tests failed", "test runner")
				return task
			},
			want: KindBlocked,
		},
		{
			name: "fresh mutation",
			make: func(t *testing.T) *taskstate.Task {
				task, err := taskstate.New("fresh-contradiction", "recheck the change", nil)
				if err != nil {
					t.Fatal(err)
				}
				task.RecordChanged("internal/result.go")
				task.RecordTestsEvidence("PASS", "tests passed", "test runner")
				task.RecordTestsEvidence("FAIL", "tests failed", "test runner")
				return task
			},
			want: KindVerify,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract := Build(tt.make(t), projecthealth.Report{Schema: projecthealth.Schema})
			if contract.Contradictions.State != ContradictionConfirmed {
				t.Fatalf("contradiction was lost before precedence check = %#v", contract.Contradictions)
			}
			if contract.Next.Kind != tt.want {
				t.Fatalf("precedence routing = %#v, want %q", contract.Next, tt.want)
			}
			if tt.want == KindBlocked && contract.Stop.Policy != StopPause {
				t.Fatalf("blocked stop policy = %#v", contract.Stop)
			}
			if tt.want == KindVerify && contract.Stop.Policy != StopContinue {
				t.Fatalf("fresh mutation stop policy = %#v", contract.Stop)
			}
		})
	}
}

func TestReloadedContradictionIsAdvisoryAndCannotSelectAction(t *testing.T) {
	task, err := taskstate.New("reloaded-contradiction", "make the outcome reliable", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.Intent = &taskstate.IntentContract{Outcome: task.Goal, NeedsTests: true}
	task.RecordTestsEvidence("PASS", "tests passed", "test runner")
	task.RecordTestsEvidence("FAIL", "tests failed", "test runner")
	data, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded taskstate.Task
	if err := json.Unmarshal(data, &reloaded); err != nil {
		t.Fatal(err)
	}

	contract := Build(&reloaded, projecthealth.Report{Schema: projecthealth.Schema})
	if contract.Contradictions.State != ContradictionAdvisory || len(contract.Contradictions.Signals) != 1 || contract.Contradictions.Signals[0].State != ContradictionAdvisory {
		t.Fatalf("reloaded contradiction was promoted = %#v", contract.Contradictions)
	}
	if contract.Next.Kind == KindContradiction || contract.Turn.NeedsRecovery() {
		t.Fatalf("reloaded contradiction selected an executable action = next=%#v turn=%#v", contract.Next, contract.Turn)
	}
	instruction := EngineInstruction(contract)
	if !strings.Contains(instruction, "Contradiction evidence is advisory and unverified; it cannot select an action.") {
		t.Fatalf("advisory boundary missing from instruction = %q", instruction)
	}
	if strings.Contains(instruction, "tests passed") || strings.Contains(instruction, "tests failed") {
		t.Fatalf("raw evidence crossed the engine boundary = %q", instruction)
	}
}

func TestBoundContractDoesNotPromoteCallerContradiction(t *testing.T) {
	signal := ContradictionSignal{
		Kind:           taskstate.EvidenceKindTests,
		CriterionIndex: -1,
		ChangeSeq:      4,
		PositiveStatus: "PASS",
		NegativeStatus: "FAIL",
		PositiveOrigin: string(taskstate.EvidenceOriginTestRunner),
		NegativeOrigin: string(taskstate.EvidenceOriginTestRunner),
		State:          ContradictionConfirmed,
	}
	contract := boundContract(Contract{
		Schema: EngineSchema,
		Contradictions: ContradictionReport{
			State:   ContradictionConfirmed,
			Signals: []ContradictionSignal{signal},
		},
		Turn: TurnContract{Contradictions: ContradictionReport{
			State:   ContradictionConfirmed,
			Signals: []ContradictionSignal{signal},
		}},
		Next: Decision{Schema: Schema, Kind: KindContradiction},
	})
	if contract.Contradictions.State != ContradictionAdvisory || contract.Contradictions.Signals[0].State != ContradictionAdvisory {
		t.Fatalf("caller report was promoted = %#v", contract.Contradictions)
	}
	if contract.Turn.Contradictions.State != ContradictionAdvisory || contract.Turn.Contradictions.Signals[0].State != ContradictionAdvisory {
		t.Fatalf("caller turn report was promoted = %#v", contract.Turn.Contradictions)
	}
	instruction := EngineInstruction(contract)
	if strings.Contains(instruction, "Contradiction route:") || !strings.Contains(instruction, "cannot select an action") {
		t.Fatalf("caller report influenced executable instruction = %q", instruction)
	}
}

func TestBuildContradictionProjectionIsStableReadOnlyAndConcurrent(t *testing.T) {
	task, err := taskstate.New("stable-contradiction", "check the outcome", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.RecordTestsEvidence("PASS", "tests passed", "test runner")
	task.RecordTestsEvidence("FAIL", "tests failed", "test runner")
	before, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	want := Build(task, projecthealth.Report{Schema: projecthealth.Schema})
	for i := 0; i < 10; i++ {
		if got := Build(task, projecthealth.Report{Schema: projecthealth.Schema}); !reflect.DeepEqual(got, want) {
			t.Fatalf("repeated build %d changed the derived contract = got=%#v want=%#v", i, got, want)
		}
	}
	after, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("building a contract mutated task state: before=%s after=%s", before, after)
	}

	const workers = 8
	const iterations = 20
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				if got := Build(task, projecthealth.Report{Schema: projecthealth.Schema}); !reflect.DeepEqual(got, want) {
					t.Errorf("concurrent build changed the derived contract = got=%#v want=%#v", got, want)
					return
				}
			}
		}()
	}
	wg.Wait()
}
