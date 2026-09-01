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
	longHorizonOutcomeSession  = "long-horizon-outcome"

	longHorizonOutcomeHelperEnv    = "PICOGENT_LONG_HORIZON_OUTCOME_HELPER"
	longHorizonOutcomeWorkspaceEnv = "PICOGENT_LONG_HORIZON_OUTCOME_WORKSPACE"
	longHorizonOutcomeTaskDirEnv   = "PICOGENT_LONG_HORIZON_OUTCOME_TASK_DIR"
	longHorizonOutcomeResultEnv    = "PICOGENT_LONG_HORIZON_OUTCOME_RESULT"
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

type longHorizonOutcomeChildResult struct {
	RestartObservation   benchmark.TurnObservation `json:"restart_observation"`
	VerifiedObservation  benchmark.TurnObservation `json:"verified_observation"`
	TaskRevision         uint64                    `json:"task_revision"`
	RetainedTaskTurns    int                       `json:"retained_task_turns"`
	RetainedTaskEvidence int                       `json:"retained_task_evidence"`
}

// TestLongHorizonOutcome records the provider-independent portion of the M
// lane. It intentionally drives the real durable task, verification, and
// Outcome Engine contracts rather than introducing a benchmark-only planner.
func TestLongHorizonOutcome(t *testing.T) {
	if os.Getenv(longHorizonOutcomeHelperEnv) == "1" {
		runLongHorizonOutcomeHelper(t)
		return
	}

	fixture := newLongHorizonOutcomeFixture(t)
	fixture.runMutationAndSteering(t)
	fixture.runFreshProcessRecovery(t)

	if err := fixture.report.Validate(); err != nil {
		t.Fatalf("long-horizon report: %v", err)
	}
	if fixture.metrics.InvalidatedProof < 2 {
		t.Fatalf("invalidated proof count = %d, want mutation and steering invalidation", fixture.metrics.InvalidatedProof)
	}
	if fixture.metrics.RecoveryEvents != 1 || fixture.metrics.FreshProcessReloads != 2 {
		t.Fatalf("recovery metrics = %#v, want one child recovery and two fresh-process reloads", fixture.metrics)
	}
	if fixture.metrics.EligibleStops != 3 {
		t.Fatalf("eligible stop count = %d, want the three fresh verification stops", fixture.metrics.EligibleStops)
	}
	if fixture.metrics.LogicalTurns != 8 || fixture.metrics.RetainedTaskTurns > 16 || fixture.metrics.RetainedTaskEvidence > 16 {
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

	task, err := taskstate.New(longHorizonOutcomeSession, "make the deterministic outcome ready", []string{"apply the deterministic outcome"})
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
			Schema:     benchmark.LongHorizonSchema,
			Scenario:   longHorizonOutcomeScenario,
			SourceHead: sourceHead,
			Host:       runtime.GOOS + "/" + runtime.GOARCH,
			GoVersion:  runtime.Version(),
			Command:    longHorizonOutcomeCommand,
			Unverified: append(unverified,
				"live-provider quality is not measured by this fixture",
				"rendered GUI and TUI behavior is not measured by this fixture",
			),
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

func (f *longHorizonOutcomeFixture) runFreshProcessRecovery(t *testing.T) {
	t.Helper()
	// Persist an active turn exactly as the production loop does before a
	// provider call. The child process must discover and close it as a restart
	// recovery rather than inheriting an in-flight turn as completion proof.
	if _, ok := f.task.BeginTurn(taskstate.TurnRouteImplement); !ok {
		t.Fatal("active restart turn did not start")
	}
	f.save(t)

	taskPath, err := f.store.Path(f.task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	resultPath := filepath.Join(filepath.Dir(f.workspace), "outcome-child-result.json")
	cmd := exec.Command(os.Args[0], "-test.run", "^TestLongHorizonOutcome$", "-test.count=1")
	cmd.Env = replaceEnv(os.Environ(), map[string]string{
		longHorizonOutcomeHelperEnv:    "1",
		longHorizonOutcomeWorkspaceEnv: f.workspace,
		longHorizonOutcomeTaskDirEnv:   filepath.Dir(taskPath),
		longHorizonOutcomeResultEnv:    resultPath,
	})
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fresh process outcome recovery: %v\n%s", err, output)
	}
	data, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatalf("read fresh process outcome result: %v", err)
	}
	var result longHorizonOutcomeChildResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("decode fresh process outcome result: %v", err)
	}

	if result.RestartObservation.Recovery != benchmark.RecoveryComplete || result.RestartObservation.CompletionEligible || result.RestartObservation.Stop != benchmark.StopContinue {
		t.Fatalf("restart observation = %#v, want completed recovery without a stop", result.RestartObservation)
	}
	if !result.VerifiedObservation.CompletionEligible || result.VerifiedObservation.Evidence != benchmark.EvidenceCurrent || result.VerifiedObservation.Stop != benchmark.StopRecheck {
		t.Fatalf("post-restart verification = %#v, want an eligible current stop", result.VerifiedObservation)
	}
	if result.TaskRevision == 0 || result.RetainedTaskTurns == 0 || result.RetainedTaskEvidence == 0 {
		t.Fatalf("fresh process returned incomplete durable metrics: %#v", result)
	}

	f.recordObservation(t, result.RestartObservation)
	f.recordObservation(t, result.VerifiedObservation)
	f.metrics.FreshProcessReloads++ // the child Store.Load
	f.metrics.FreshProcessReloads++ // the parent Store.Load below

	reloaded, err := f.store.Load(f.task.SessionID)
	if err != nil {
		t.Fatalf("reload final outcome in parent: %v", err)
	}
	if reloaded.CompletionReady() || !reloaded.NeedsVerification() {
		t.Fatalf("persisted proof became trusted without a live producer: ready=%v needs=%v", reloaded.CompletionReady(), reloaded.NeedsVerification())
	}
	freshObservation, err := workspace.Capture(context.Background(), f.workspace, []string{longHorizonOutcomeFile})
	if err != nil {
		t.Fatalf("capture final fresh workspace observation: %v", err)
	}
	if !reloaded.ReestablishWorkspaceVerification(&freshObservation) || !reloaded.CompletionReady() {
		t.Fatalf("fresh live verification did not restore completion proof: ready=%v needs=%v", reloaded.CompletionReady(), reloaded.NeedsVerification())
	}
	if err := f.store.Save(reloaded); err != nil {
		t.Fatalf("save re-established completion proof: %v", err)
	}
	f.task = reloaded
	f.metrics.RecoveryEvents++
	f.metrics.RetainedTaskTurns = len(f.task.Turns)
	f.metrics.RetainedTaskEvidence = len(f.task.Evidence)
}

func runLongHorizonOutcomeHelper(t *testing.T) {
	t.Helper()
	workspaceRoot := os.Getenv(longHorizonOutcomeWorkspaceEnv)
	taskDir := os.Getenv(longHorizonOutcomeTaskDirEnv)
	resultPath := os.Getenv(longHorizonOutcomeResultEnv)
	if workspaceRoot == "" || taskDir == "" || resultPath == "" {
		t.Fatal("fresh process outcome helper paths are required")
	}
	store := taskstate.NewStore(taskDir)
	task, err := store.Load(longHorizonOutcomeSession)
	if err != nil {
		t.Fatalf("load outcome in fresh process: %v", err)
	}
	last := task.LastTurn()
	if last == nil || last.State != taskstate.TurnActive {
		t.Fatalf("fresh process saw turn=%#v, want active restart turn", last)
	}
	if !task.RecoverActiveTurn() {
		t.Fatal("fresh process did not recover active turn")
	}
	if err := store.Save(task); err != nil {
		t.Fatalf("save recovered outcome in fresh process: %v", err)
	}
	fixture := &longHorizonOutcomeFixture{workspace: workspaceRoot, store: store, task: task}
	restartObservation := longHorizonOutcomeObservation(task, 7, []benchmark.ScenarioEvent{benchmark.EventRestart, benchmark.EventRecovery}, benchmark.RecoveryComplete)

	sequence, ok := task.BeginTurn(taskstate.TurnRouteVerify)
	if !ok {
		t.Fatal("post-restart recovery turn did not start")
	}
	fixture.recordCurrentVerification(t)
	if !task.FinishTurn(sequence, taskstate.TurnRouteVerify, "re-establish proof after process restart", "PASS", taskstate.StopNone, 1, 0) {
		t.Fatal("post-restart verification turn did not finish")
	}
	if err := store.Save(task); err != nil {
		t.Fatalf("save post-restart verification: %v", err)
	}
	verifiedObservation := longHorizonOutcomeObservation(task, 8, []benchmark.ScenarioEvent{benchmark.EventVerification, benchmark.EventStop}, benchmark.RecoveryComplete)
	if !verifiedObservation.CompletionEligible {
		t.Fatalf("fresh process verification did not become eligible: %#v", verifiedObservation)
	}
	result := longHorizonOutcomeChildResult{
		RestartObservation:   restartObservation,
		VerifiedObservation:  verifiedObservation,
		TaskRevision:         task.Revision,
		RetainedTaskTurns:    len(task.Turns),
		RetainedTaskEvidence: len(task.Evidence),
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultPath, data, 0o600); err != nil {
		t.Fatalf("write fresh process outcome result: %v", err)
	}
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
	observation := longHorizonOutcomeObservation(f.task, len(f.report.Observations)+1, events, recovery)
	f.recordObservation(t, observation)
	return observation
}

func (f *longHorizonOutcomeFixture) recordObservation(t *testing.T, observation benchmark.TurnObservation) {
	t.Helper()
	if observation.Turn != len(f.report.Observations)+1 {
		t.Fatalf("observation turn=%d, want %d", observation.Turn, len(f.report.Observations)+1)
	}
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
}

func longHorizonOutcomeObservation(task *taskstate.Task, turn int, events []benchmark.ScenarioEvent, recovery benchmark.RecoveryState) benchmark.TurnObservation {
	last := task.LastTurn()
	if last == nil {
		return benchmark.TurnObservation{}
	}
	observation := benchmark.TurnObservation{
		Turn:                turn,
		TurnRevision:        last.Sequence,
		Events:              append([]benchmark.ScenarioEvent(nil), events...),
		CriteriaComplete:    criteriaComplete(task),
		MutationSeq:         mutationSequence(task),
		VerifiedMutationSeq: verifiedMutationSequence(task),
		Evidence:            evidenceState(task),
		Recovery:            recovery,
		Stop:                stopDecision(task),
	}
	observation.CompletionEligible = observation.CanStop()
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
