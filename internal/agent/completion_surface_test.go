package agent

import (
	"reflect"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/outcome"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/projecthealth"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/testsupport"
	"github.com/saiaathish/picogent/internal/tools"
)

func TestHeadlessCompletionGateProjectsLongHorizonStates(t *testing.T) {
	cases, err := testsupport.NewCompletionProjectionCases()
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			wantProof := outcome.EvaluateCompletion(tc.Task)
			got := (Result{Task: tc.Task, GoalDone: tc.Marker}).CompletionGate(tc.Goal)
			contract := outcome.Build(tc.Task, projecthealth.Report{Schema: projecthealth.Schema})

			if !reflect.DeepEqual(got.Proof, wantProof) {
				t.Fatalf("headless proof = %#v, want durable proof %#v", got.Proof, wantProof)
			}
			if got.Required != true || got.Marker != tc.Marker || got.Ready != tc.WantReady {
				t.Fatalf("headless projection = %#v, want required=true marker=%v ready=%v", got, tc.Marker, tc.WantReady)
			}
			if got.Explanation() == "" {
				t.Fatal("headless projection has no bounded explanation")
			}
			last := tc.Task.LastTurn()
			if last == nil || contract.Turn.TurnSequence != last.Sequence || contract.Turn.LastTurnState != string(last.State) || contract.Turn.LastRoute != last.Route {
				t.Fatalf("headless turn projection = %#v, want durable turn %#v", contract.Turn, last)
			}
			wantRecovery := tc.State == testsupport.StateRecoveryPending
			if contract.Turn.NeedsRecovery() != wantRecovery {
				t.Fatalf("headless recovery projection = %v, want %v: %#v", contract.Turn.NeedsRecovery(), wantRecovery, contract.Turn)
			}
			if tc.State == testsupport.StateCurrentProof && contract.Stop.Policy != outcome.StopRecheck {
				t.Fatalf("headless current-proof stop policy = %q, want %q", contract.Stop.Policy, outcome.StopRecheck)
			}
		})
	}
}

func TestHeadlessCompletionProjectionReloadRequiresFreshWorkspaceEvidence(t *testing.T) {
	workspaceRoot := t.TempDir()
	fixture, err := testsupport.NewWorkspaceBoundCompletionFixture(workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	store := taskstate.NewStore(t.TempDir())
	if err := store.Save(fixture.Task); err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.Load(fixture.Task.SessionID)
	if err != nil {
		t.Fatal(err)
	}

	pending := (Result{Task: reloaded, GoalDone: true}).CompletionGate(reloaded.Goal)
	if pending.Ready || reloaded.CompletionReady() {
		t.Fatalf("reloaded proof was trusted without a fresh observation: projection=%#v task=%#v", pending, reloaded.CompletionCheck())
	}
	if !reloaded.ReestablishWorkspaceVerification(&fixture.Observation) {
		t.Fatal("fresh workspace observation did not rebind persisted proof")
	}
	ready := (Result{Task: reloaded, GoalDone: true}).CompletionGate(reloaded.Goal)
	if !ready.Ready || !reloaded.CompletionReady() {
		t.Fatalf("fresh workspace observation did not restore completion: projection=%#v task=%#v", ready, reloaded.CompletionCheck())
	}
}

func TestSetTaskSessionRecoversInterruptedOutcomeTurn(t *testing.T) {
	const sessionID = "projection-agent-recovery"
	store := taskstate.NewStore(t.TempDir())
	task, err := taskstate.New(sessionID, "resume the interrupted outcome", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.DefinitionOfDone = []taskstate.Criterion{{Description: "the outcome is verified", Required: true}}
	if !task.SetIntent(&taskstate.IntentContract{Outcome: task.Goal, Class: "implementation", Action: "deliver the outcome"}) {
		t.Fatal("recovery intent was not recorded")
	}
	if err := task.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	task.RecordCriterionVerification(0, "verify", "the outcome is verified", "verify")
	sequence, ok := task.BeginTurn(taskstate.TurnRouteImplement)
	if !ok {
		t.Fatal("active recovery turn did not start")
	}
	task.RecordChanged("outcome.txt")
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}

	workspaceRoot := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = workspaceRoot
	cfg.Provider = config.ProviderOllama
	a := New(cfg, nil, tools.NewRegistry(tools.Context{Workspace: workspaceRoot}), perm.New(config.ModeFast, workspaceRoot, nil))
	defer a.Close()
	a.SetTaskStore(store)
	if err := a.SetTaskSession(sessionID); err != nil {
		t.Fatal(err)
	}

	recovered := a.TaskSnapshot()
	if recovered == nil {
		t.Fatal("SetTaskSession did not attach the persisted task")
	}
	last := recovered.LastTurn()
	if last == nil || last.Sequence != sequence || last.State != taskstate.TurnInterrupted || last.Route != string(taskstate.TurnRouteRecover) || last.StopReason != taskstate.StopProcessRestart || strings.TrimSpace(last.Hypothesis) == "" || last.FinishedAt == nil {
		t.Fatalf("SetTaskSession recovery turn = %#v, want interrupted/recover metadata", last)
	}
	if last.MutationCount != 1 || len(last.ChangedFiles) != 1 || last.ChangedFiles[0] != "outcome.txt" {
		t.Fatalf("SetTaskSession recovery side effects = %#v, want one changed outcome file", last)
	}
	contract := outcome.TurnContractForTask(recovered)
	if !contract.NeedsRecovery() {
		t.Fatalf("recovered turn contract did not request conservative routing: %#v", contract)
	}
}
