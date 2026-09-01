package benchmark_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	benchmark "github.com/saiaathish/picogent/internal/benchmark"
	"github.com/saiaathish/picogent/internal/outcome"
	"github.com/saiaathish/picogent/internal/projecthealth"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/workspace"
)

const (
	longHorizonOutcomeScenario = "deterministic-durable-outcome"
	longHorizonOutcomeCommand  = "go test ./internal/benchmark -run '^TestLongHorizonOutcome$' -count=1"
	longHorizonOutcomeFile     = "outcome.txt"
)

// longHorizonOutcomeMetrics are bounded measurements for the scripted
// lifecycle. They are test evidence, not a second durable task schema.
type longHorizonOutcomeMetrics struct {
	LogicalTurns         int `json:"logical_turns"`
	UsefulProgress       int `json:"useful_progress"`
	InvalidatedProof     int `json:"invalidated_proof"`
	RecoveryEvents       int `json:"recovery_events"`
	EligibleStops        int `json:"eligible_stops"`
	FreshProcessReloads  int `json:"fresh_process_reloads"`
	RetainedTaskTurns    int `json:"retained_task_turns"`
	RetainedTaskEvidence int `json:"retained_task_evidence"`
}

type longHorizonOutcomeFixture struct {
	workspace string
	store     *taskstate.Store
	task      *taskstate.Task
	report    benchmark.Report
	metrics   longHorizonOutcomeMetrics
}

// TestLongHorizonOutcome records the provider-independent portion of the M
// lane. It intentionally drives the real durable task, verification, and
// Outcome Engine contracts rather than introducing a benchmark-only planner.
func TestLongHorizonOutcome(t *testing.T) {
	fixture := newLongHorizonOutcomeFixture(t)
	fixture.runMutationAndSteering(t)

	if err := fixture.report.Validate(); err != nil {
		t.Fatalf("long-horizon report: %v", err)
	}
	if fixture.metrics.InvalidatedProof != 2 {
		t.Fatalf("invalidated proof count = %d, want mutation and steering invalidation", fixture.metrics.InvalidatedProof)
	}
	if fixture.metrics.EligibleStops != 2 {
		t.Fatalf("eligible stop count = %d, want the two fresh verification stops", fixture.metrics.EligibleStops)
	}
	if fixture.metrics.RetainedTaskTurns > 16 || fixture.metrics.RetainedTaskEvidence > 16 {
		t.Fatalf("retained task state exceeded bounds: %#v", fixture.metrics)
	}

	reportJSON, err := json.Marshal(fixture.report)
	if err != nil {
		t.Fatal(err)
	}
	metricsJSON, err := json.Marshal(fixture.metrics)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("long-horizon outcome report=%s metrics=%s", reportJSON, metricsJSON)
}

func newLongHorizonOutcomeFixture(t *testing.T) *longHorizonOutcomeFixture {
	t.Helper()
	root := t.TempDir()
	workspaceRoot := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, longHorizonOutcomeFile), []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	const sessionID = "long-horizon-outcome"
	task, err := taskstate.New(sessionID, "make the deterministic outcome ready", []string{"apply the deterministic outcome"})
	if err != nil {
		t.Fatal(err)
	}
	task.DefinitionOfDone = []taskstate.Criterion{{Description: "the deterministic outcome is verified", Required: true}}
	if !task.SetIntent(&taskstate.IntentContract{
		Outcome: task.Goal,
		Class:   "implementation",
		Action:  "apply and verify the deterministic outcome",
	}) {
		t.Fatal("initial outcome intent was not recorded")
	}
	if err := task.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	store := taskstate.NewStore(filepath.Join(root, "tasks"))
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}

	sourceHead, unverified := longHorizonSourceMetadata(t)
	return &longHorizonOutcomeFixture{
		workspace: workspaceRoot,
		store:     store,
		task:      task,
		report: benchmark.Report{
			Schema:       benchmark.LongHorizonSchema,
			Scenario:     longHorizonOutcomeScenario,
			SourceHead:   sourceHead,
			Host:         runtime.GOOS + "/" + runtime.GOARCH,
			GoVersion:    runtime.Version(),
			Command:      longHorizonOutcomeCommand,
			Unverified:   unverified,
			Observations: make([]benchmark.TurnObservation, 0, 8),
		},
	}
}

func (f *longHorizonOutcomeFixture) runMutationAndSteering(t *testing.T) {
	t.Helper()

	f.finishTurn(t, taskstate.TurnRouteAdmission, "define the bounded outcome", "UNVERIFIED", taskstate.StopNone, 0, 0)
	f.observe(t, []benchmark.ScenarioEvent{benchmark.EventPlan}, benchmark.RecoveryNotRequired)

	sequence := f.beginTurn(t, taskstate.TurnRouteImplement)
	if err := os.WriteFile(filepath.Join(f.workspace, longHorizonOutcomeFile), []byte("first mutation\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.task.RecordChanged(longHorizonOutcomeFile)
	if !f.task.FinishTurn(sequence, taskstate.TurnRouteImplement, "apply the first deterministic mutation", "UNVERIFIED", taskstate.StopNone, 1, 1) {
		t.Fatal("mutation turn did not finish")
	}
	f.save(t)
	f.observe(t, []benchmark.ScenarioEvent{benchmark.EventMutation}, benchmark.RecoveryNotRequired)

	sequence = f.beginTurn(t, taskstate.TurnRouteVerify)
	if !f.task.Advance() {
		t.Fatal("criterion progress did not advance")
	}
	f.recordCurrentVerification(t)
	if !f.task.FinishTurn(sequence, taskstate.TurnRouteVerify, "verify the first mutation", "PASS", taskstate.StopNone, 1, 0) {
		t.Fatal("first verification turn did not finish")
	}
	f.save(t)
	firstVerified := f.observe(t, []benchmark.ScenarioEvent{benchmark.EventVerification, benchmark.EventStop}, benchmark.RecoveryNotRequired)
	if !firstVerified.CompletionEligible || firstVerified.Evidence != benchmark.EvidenceCurrent {
		t.Fatalf("first verification observation = %#v, want an eligible current stop", firstVerified)
	}

	sequence = f.beginTurn(t, taskstate.TurnRouteImplement)
	if err := os.WriteFile(filepath.Join(f.workspace, longHorizonOutcomeFile), []byte("second mutation\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.task.RecordChanged(longHorizonOutcomeFile)
	if !f.task.FinishTurn(sequence, taskstate.TurnRouteImplement, "change the already verified outcome", "UNVERIFIED", taskstate.StopNone, 1, 1) {
		t.Fatal("second mutation turn did not finish")
	}
	f.save(t)
	staleAfterMutation := f.observe(t, []benchmark.ScenarioEvent{benchmark.EventMutation}, benchmark.RecoveryNotRequired)
	if staleAfterMutation.CompletionEligible || staleAfterMutation.Evidence != benchmark.EvidenceStale || staleAfterMutation.Stop != benchmark.StopContinue {
		t.Fatalf("mutation invalidation observation = %#v, want stale proof and CONTINUE", staleAfterMutation)
	}

	sequence = f.beginTurn(t, taskstate.TurnRouteVerify)
	f.recordCurrentVerification(t)
	if !f.task.FinishTurn(sequence, taskstate.TurnRouteVerify, "verify the second mutation", "PASS", taskstate.StopNone, 1, 0) {
		t.Fatal("second verification turn did not finish")
	}
	f.save(t)
	secondVerified := f.observe(t, []benchmark.ScenarioEvent{benchmark.EventVerification, benchmark.EventStop}, benchmark.RecoveryNotRequired)
	if !secondVerified.CompletionEligible || secondVerified.Evidence != benchmark.EvidenceCurrent {
		t.Fatalf("second verification observation = %#v, want an eligible current stop", secondVerified)
	}

	sequence = f.beginTurn(t, taskstate.TurnRouteOther)
	steered := *f.task.Intent
	steered.Scope = "the outcome after user steering"
	if !f.task.SetIntent(&steered) {
		t.Fatal("intent steering did not advance the durable contract")
	}
	if !f.task.FinishTurn(sequence, taskstate.TurnRouteOther, "incorporate the changed outcome contract", "UNVERIFIED", taskstate.StopNone, 0, 0) {
		t.Fatal("steering turn did not finish")
	}
	f.save(t)
	staleAfterSteering := f.observe(t, []benchmark.ScenarioEvent{benchmark.EventSteering}, benchmark.RecoveryNotRequired)
	if staleAfterSteering.CompletionEligible || staleAfterSteering.Evidence != benchmark.EvidenceStale || staleAfterSteering.Stop != benchmark.StopContinue {
		t.Fatalf("steering invalidation observation = %#v, want stale proof and CONTINUE", staleAfterSteering)
	}

	f.metrics.RetainedTaskTurns = len(f.task.Turns)
	f.metrics.RetainedTaskEvidence = len(f.task.Evidence)
}

func (f *longHorizonOutcomeFixture) beginTurn(t *testing.T, route taskstate.TurnRoute) uint64 {
	t.Helper()
	sequence, ok := f.task.BeginTurn(route)
	if !ok {
		t.Fatalf("begin %s turn", route)
	}
	return sequence
}

func (f *longHorizonOutcomeFixture) finishTurn(t *testing.T, route taskstate.TurnRoute, hypothesis, evidence string, stop taskstate.StopReason, toolRounds, mutations int) {
	t.Helper()
	sequence := f.beginTurn(t, route)
	if !f.task.FinishTurn(sequence, route, hypothesis, evidence, stop, toolRounds, mutations) {
		t.Fatalf("finish %s turn", route)
	}
	f.save(t)
}

func (f *longHorizonOutcomeFixture) recordCurrentVerification(t *testing.T) {
	t.Helper()
	observation, err := workspace.Capture(context.Background(), f.workspace, []string{longHorizonOutcomeFile})
	if err != nil {
		t.Fatalf("capture verification observation: %v", err)
	}
	f.task.AddVerificationForCriterion(0, longHorizonOutcomeCommand, true, "verify PASS deterministic outcome", &observation)
}

func (f *longHorizonOutcomeFixture) save(t *testing.T) {
	t.Helper()
	if err := f.store.Save(f.task); err != nil {
		t.Fatalf("save durable outcome: %v", err)
	}
}

func (f *longHorizonOutcomeFixture) observe(t *testing.T, events []benchmark.ScenarioEvent, recovery benchmark.RecoveryState) benchmark.TurnObservation {
	t.Helper()
	last := f.task.LastTurn()
	if last == nil {
		t.Fatal("observe outcome without a durable turn")
	}
	observation := benchmark.TurnObservation{
		Turn:                len(f.report.Observations) + 1,
		TurnRevision:        last.Sequence,
		Events:              append([]benchmark.ScenarioEvent(nil), events...),
		CriteriaComplete:    criteriaComplete(f.task),
		MutationSeq:         mutationSequence(f.task),
		VerifiedMutationSeq: verifiedMutationSequence(f.task),
		Evidence:            evidenceState(f.task),
		Recovery:            recovery,
		Stop:                stopDecision(f.task),
	}
	observation.CompletionEligible = observation.CanStop()
	f.report.Observations = append(f.report.Observations, observation)
	f.metrics.LogicalTurns = len(f.report.Observations)
	if observation.CriteriaComplete {
		f.metrics.UsefulProgress++
	}
	if observation.Evidence == benchmark.EvidenceStale {
		f.metrics.InvalidatedProof++
	}
	if observation.CompletionEligible {
		f.metrics.EligibleStops++
	}
	return observation
}

func criteriaComplete(task *taskstate.Task) bool {
	if task == nil {
		return false
	}
	for index, criterion := range task.DefinitionOfDone {
		if !criterion.Required {
			continue
		}
		if index >= len(task.Steps) || !task.Steps[index].Done {
			return false
		}
	}
	return len(task.DefinitionOfDone) > 0
}

func mutationSequence(task *taskstate.Task) uint64 {
	if task == nil || task.ChangeSeq < 0 {
		return 0
	}
	return uint64(task.ChangeSeq)
}

func verifiedMutationSequence(task *taskstate.Task) uint64 {
	if task == nil || task.VerifiedChangeSeq < 0 {
		return 0
	}
	return uint64(task.VerifiedChangeSeq)
}

func evidenceState(task *taskstate.Task) benchmark.EvidenceState {
	if task == nil {
		return benchmark.EvidenceUnverified
	}
	// CriterionEvidenceState intentionally hides stale positive proof as
	// UNVERIFIED. The durable sequence mismatch is the authoritative signal
	// that the previously passing proof was invalidated by a mutation.
	if task.VerifiedChangeSeq >= 0 && task.VerifiedChangeSeq != task.ChangeSeq && len(task.Verification) > 0 {
		return benchmark.EvidenceStale
	}
	status, current := task.CriterionEvidenceState(0)
	status = strings.ToUpper(strings.TrimSpace(status))
	if current && status == "PASS" && !task.NeedsVerification() {
		return benchmark.EvidenceCurrent
	}
	if status == "INCONCLUSIVE" || status == "FAIL" || (status == "PASS" && !current) {
		return benchmark.EvidenceStale
	}
	if len(task.Verification) > 0 || len(task.Evidence) > 0 {
		return benchmark.EvidenceUnverified
	}
	return benchmark.EvidenceMissing
}

func stopDecision(task *taskstate.Task) benchmark.StopDecision {
	contract := outcome.Build(task, projecthealth.Report{Schema: projecthealth.Schema})
	switch contract.Stop.Policy {
	case outcome.StopContinue:
		return benchmark.StopContinue
	case outcome.StopPause:
		return benchmark.StopPause
	case outcome.StopRecheck:
		return benchmark.StopRecheck
	default:
		return benchmark.StopUnknown
	}
}

func longHorizonSourceMetadata(t *testing.T) (string, []string) {
	t.Helper()
	root := benchmarkModuleRootForTest(t)
	headOutput, err := exec.Command("git", "-C", root, "rev-parse", "--verify", "HEAD^{commit}").CombinedOutput()
	if err != nil {
		t.Fatalf("resolve exact source head: %v\n%s", err, headOutput)
	}
	head := strings.TrimSpace(string(headOutput))
	statusOutput, err := exec.Command("git", "-C", root, "status", "--porcelain").CombinedOutput()
	if err != nil {
		t.Fatalf("inspect source dirtiness: %v\n%s", err, statusOutput)
	}
	var unverified []string
	if strings.TrimSpace(string(statusOutput)) != "" {
		unverified = []string{"working-tree changes are not represented by source_head"}
	}
	return head, unverified
}

func benchmarkModuleRootForTest(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find go.mod for long-horizon outcome fixture")
		}
		dir = parent
	}
}
