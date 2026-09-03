//go:build rendered_fixture

package gui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/benchmark"
	"github.com/saiaathish/picogent/internal/outcome"
	"github.com/saiaathish/picogent/internal/projecthealth"
	"github.com/saiaathish/picogent/internal/projects"
	"github.com/saiaathish/picogent/internal/session"
	"github.com/saiaathish/picogent/internal/taskstate"
)

const renderedLongHorizonFixtureTestCommand = "go test -tags rendered_fixture ./internal/gui -run '^TestRenderedLongHorizonFixtureAPIBoundary$' -count=1"

// TestRenderedLongHorizonFixtureAPIBoundary drives the build-tagged fixture
// through the same HTTP and event boundaries used by the embedded GUI. It is
// intentionally not a browser-quality claim: the report records that direct
// DOM and live-provider observations remain outside this medium lane.
func TestRenderedLongHorizonFixtureAPIBoundary(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	workspace := filepath.Join(home, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PICOGENT_HOME", home)

	server, ag, fixtureRuntime, err := newRenderedLongHorizonFixtureServer("seed", home, workspace)
	if err != nil {
		t.Fatal(err)
	}
	firstHTTP := httptest.NewServer(server.Handler())
	firstSSE := startRenderedLongHorizonSSE(t, firstHTTP.URL)

	report := newRenderedLongHorizonFixtureTestReport(t)
	defer func() {
		firstSSE.Close()
		server.stopForShutdown()
		server.waitForTurns()
		firstHTTP.Close()
		ag.Close()
	}()

	previousRevision := uint64(0)
	eventStart := firstSSE.Len()
	driveRenderedLongHorizonChat(t, firstHTTP.URL, "Create the rendered UI outcome probe", previousRevision)
	task := waitForRenderedLongHorizonTask(t, ag, previousRevision+1)
	_ = firstSSE.WaitForTurn(t, eventStart, task.LastTurn().Sequence, 2)
	assertRenderedLongHorizonState(t, firstHTTP.URL, task, false)
	report.Observations = append(report.Observations, renderedLongHorizonFixtureObservation(task, report, []benchmark.ScenarioEvent{benchmark.EventPlan, benchmark.EventMutation, benchmark.EventVerification}, benchmark.RecoveryNotRequired))
	previousRevision = task.LastTurn().Sequence

	eventStart = firstSSE.Len()
	driveRenderedLongHorizonChat(t, firstHTTP.URL, "Verify the rendered UI outcome probe", previousRevision)
	task = waitForRenderedLongHorizonTask(t, ag, previousRevision+1)
	_ = firstSSE.WaitForTurn(t, eventStart, task.LastTurn().Sequence, 1)
	assertRenderedLongHorizonState(t, firstHTTP.URL, task, false)
	report.Observations = append(report.Observations, renderedLongHorizonFixtureObservation(task, report, []benchmark.ScenarioEvent{benchmark.EventVerification}, benchmark.RecoveryNotRequired))
	previousRevision = task.LastTurn().Sequence

	eventStart = firstSSE.Len()
	driveRenderedLongHorizonChat(t, firstHTTP.URL, "Review the rendered UI outcome after steering its scope", previousRevision)
	task = waitForRenderedLongHorizonTask(t, ag, previousRevision+1)
	_ = firstSSE.WaitForTurn(t, eventStart, task.LastTurn().Sequence, 1)
	assertRenderedLongHorizonState(t, firstHTTP.URL, task, false)
	report.Observations = append(report.Observations, renderedLongHorizonFixtureObservation(task, report, []benchmark.ScenarioEvent{benchmark.EventSteering, benchmark.EventVerification}, benchmark.RecoveryNotRequired))
	previousRevision = task.LastTurn().Sequence

	if fixtureRuntime.verificationCount() != 3 {
		t.Fatalf("seed verification calls = %d, want one per mutation/verification/steering turn", fixtureRuntime.verificationCount())
	}
	if task.CompletionReady() {
		t.Fatal("seed task became complete without direct rendered proof")
	}
	if !task.NeedsVerification() {
		t.Fatal("steering turn unexpectedly retained current proof")
	}

	// Simulate the production crash boundary after durable turn admission. A
	// fresh Agent.SetTaskSession must close this active record as recovery before
	// exposing the same task to the reloaded GUI.
	store := taskstate.NewStore(filepath.Join(home, "tasks", projects.IDForPath(workspace)))
	active := ag.TaskSnapshot()
	if _, ok := active.BeginTurn(taskstate.TurnRouteImplement); !ok {
		t.Fatal("could not persist active pre-reload turn")
	}
	if err := store.Save(active); err != nil {
		t.Fatalf("save active pre-reload turn: %v", err)
	}
	server.stopForShutdown()
	server.waitForTurns()
	firstSSE.Close()
	firstHTTP.Close()
	ag.Close()

	reloadServer, reloadAgent, reloadRuntime, err := newRenderedLongHorizonFixtureServer("reload", home, workspace)
	if err != nil {
		t.Fatal(err)
	}
	reloadHTTP := httptest.NewServer(reloadServer.Handler())
	reloadSSE := startRenderedLongHorizonSSE(t, reloadHTTP.URL)
	defer func() {
		reloadSSE.Close()
		reloadServer.stopForShutdown()
		reloadServer.waitForTurns()
		reloadHTTP.Close()
		reloadAgent.Close()
	}()

	recovered := reloadAgent.TaskSnapshot()
	if recovered == nil || recovered.CompletionReady() {
		t.Fatalf("reloaded task = %#v, want present and fail-closed", recovered)
	}
	last := recovered.LastTurn()
	if last == nil || last.State != taskstate.TurnInterrupted || last.Route != string(taskstate.TurnRouteRecover) {
		t.Fatalf("reloaded latest turn = %#v, want process-restart recovery", last)
	}
	assertRenderedLongHorizonState(t, reloadHTTP.URL, recovered, false)
	report.Observations = append(report.Observations, renderedLongHorizonFixtureObservation(recovered, report, []benchmark.ScenarioEvent{benchmark.EventRestart, benchmark.EventRecovery}, benchmark.RecoveryComplete))
	previousRevision = last.Sequence

	eventStart = reloadSSE.Len()
	driveRenderedLongHorizonChat(t, reloadHTTP.URL, "Verify the rendered UI outcome after reload", previousRevision)
	reloadedTask := waitForRenderedLongHorizonTask(t, reloadAgent, previousRevision+1)
	_ = reloadSSE.WaitForTurn(t, eventStart, reloadedTask.LastTurn().Sequence, 1)
	assertRenderedLongHorizonState(t, reloadHTTP.URL, reloadedTask, false)
	report.Observations = append(report.Observations, renderedLongHorizonFixtureObservation(reloadedTask, report, []benchmark.ScenarioEvent{benchmark.EventVerification}, benchmark.RecoveryComplete))

	if reloadRuntime.verificationCount() != 1 {
		t.Fatalf("reload verification calls = %d, want one deterministic recheck", reloadRuntime.verificationCount())
	}
	transcript, err := session.Load(renderedLongHorizonFixtureSession)
	if err != nil {
		t.Fatal(err)
	}
	if len(transcript.Messages) < 8 {
		t.Fatalf("reloaded transcript messages = %d, want persisted multi-turn history", len(transcript.Messages))
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("rendered long-horizon fixture report: %v", err)
	}
	reportJSON, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("rendered long-horizon fixture report=%s", reportJSON)
}

type renderedLongHorizonSSE struct {
	mu     sync.Mutex
	events []event
	ready  chan error
	closed chan struct{}
	cancel context.CancelFunc
}

func startRenderedLongHorizonSSE(t *testing.T, baseURL string) *renderedLongHorizonSSE {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	collector := &renderedLongHorizonSSE{
		ready:  make(chan error, 1),
		closed: make(chan struct{}),
		cancel: cancel,
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		response, err := http.DefaultClient.Do(request)
		collector.ready <- err
		if err != nil {
			close(collector.closed)
			return
		}
		defer response.Body.Close()
		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var current event
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &current); err != nil {
				continue
			}
			if current.Type == "hello" {
				continue
			}
			collector.mu.Lock()
			collector.events = append(collector.events, current)
			collector.mu.Unlock()
		}
		close(collector.closed)
	}()
	select {
	case err := <-collector.ready:
		if err != nil {
			t.Fatalf("open rendered SSE: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out opening rendered SSE")
	}
	return collector
}

func (s *renderedLongHorizonSSE) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.events)
}

func (s *renderedLongHorizonSSE) Close() {
	if s == nil {
		return
	}
	s.cancel()
	select {
	case <-s.closed:
	case <-time.After(3 * time.Second):
	}
}

func (s *renderedLongHorizonSSE) snapshotFrom(start int) []event {
	s.mu.Lock()
	defer s.mu.Unlock()
	if start < 0 || start > len(s.events) {
		start = 0
	}
	return append([]event(nil), s.events[start:]...)
}

func (s *renderedLongHorizonSSE) WaitForTurn(t *testing.T, start int, sequence uint64, permissions int) []event {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		events := s.snapshotFrom(start)
		taskSeen := false
		permissionCount := 0
		for _, current := range events {
			if current.SessionID != renderedLongHorizonFixtureSession {
				continue
			}
			if current.Type == "permission" {
				permissionCount++
			}
			if current.Type == "task_progress" && current.Task != nil {
				last := current.Task.LastTurn()
				if current.Task.SessionID != renderedLongHorizonFixtureSession {
					t.Fatalf("SSE task session = %q, want %q", current.Task.SessionID, renderedLongHorizonFixtureSession)
				}
				if last == nil || last.Sequence < sequence {
					continue
				}
				if last.Sequence != sequence {
					t.Fatalf("SSE task identity = %#v, want session %q turn %d", current.Task, renderedLongHorizonFixtureSession, sequence)
				}
				taskSeen = true
			}
		}
		if taskSeen && permissionCount >= permissions {
			return events
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("SSE did not expose turn %d with %d permission events: %#v", sequence, permissions, s.snapshotFrom(start))
	return nil
}

func driveRenderedLongHorizonChat(t *testing.T, baseURL, prompt string, previousRevision uint64) {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"prompt": prompt})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, baseURL+"/api/chat", strings.NewReader(string(payload)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://"+request.URL.Host)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		t.Fatalf("chat status = %d, want %d: %s", response.StatusCode, http.StatusAccepted, data)
	}

	seenPermissions := map[string]bool{}
	deadline := time.Now().Add(10 * time.Second)
	started := false
	for time.Now().Before(deadline) {
		state := readRenderedLongHorizonState(t, baseURL)
		busy, _ := state["busy"].(bool)
		started = started || busy
		if pending, ok := state["pending_perm"].(map[string]any); ok {
			permissionID, _ := pending["permission_id"].(string)
			if permissionID != "" && !seenPermissions[permissionID] {
				postRenderedLongHorizonPermission(t, baseURL, permissionID)
				seenPermissions[permissionID] = true
			}
		}
		currentRevision := renderedLongHorizonStateTurnRevision(state)
		if !busy && (started || currentRevision > previousRevision) && !renderedLongHorizonStateHasPendingPermission(state) && currentRevision > previousRevision {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("chat prompt %q did not finish after turn %d", prompt, previousRevision)
}

func readRenderedLongHorizonState(t *testing.T, baseURL string) map[string]any {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, baseURL+"/api/state", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("state status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	var state map[string]any
	if err := json.NewDecoder(response.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

func postRenderedLongHorizonPermission(t *testing.T, baseURL, permissionID string) {
	t.Helper()
	payload := fmt.Sprintf(`{"allow":true,"permission_id":%q}`, permissionID)
	request, err := http.NewRequest(http.MethodPost, baseURL+"/api/permission", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://"+request.URL.Host)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("permission status = %d, want %d", response.StatusCode, http.StatusNoContent)
	}
}

func waitForRenderedLongHorizonTask(t *testing.T, ag *agent.Agent, previousRevision uint64) *taskstate.Task {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		task := ag.TaskSnapshot()
		if task != nil {
			last := task.LastTurn()
			if last != nil && last.Sequence >= previousRevision && last.State != taskstate.TurnActive {
				return task
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("agent did not expose a closed turn at revision %d", previousRevision)
	return nil
}

func assertRenderedLongHorizonState(t *testing.T, baseURL string, task *taskstate.Task, wantReady bool) {
	t.Helper()
	state := readRenderedLongHorizonState(t, baseURL)
	stateTask, ok := state["task"].(map[string]any)
	if !ok {
		t.Fatalf("rendered task state = %#v, want a task", state["task"])
	}
	if got, _ := stateTask["session_id"].(string); got != renderedLongHorizonFixtureSession {
		t.Fatalf("rendered task session = %q, want %q", got, renderedLongHorizonFixtureSession)
	}
	ready := false
	if completion, ok := state["completion"].(map[string]any); ok {
		ready, _ = completion["ready"].(bool)
	}
	if ready != wantReady {
		t.Fatalf("rendered completion ready = %v, want %v", ready, wantReady)
	}
	if ready != task.CompletionReady() {
		t.Fatalf("rendered completion ready = %v disagrees with authoritative task = %v", ready, task.CompletionReady())
	}
}

func renderedLongHorizonFixtureObservation(task *taskstate.Task, report RenderedLongHorizonReport, events []benchmark.ScenarioEvent, recovery benchmark.RecoveryState) RenderedLongHorizonObservation {
	last := task.LastTurn()
	observation := benchmark.TurnObservation{
		Turn:                len(report.Observations) + 1,
		TurnRevision:        last.Sequence,
		Events:              append([]benchmark.ScenarioEvent(nil), events...),
		CriteriaComplete:    renderedLongHorizonCriteriaComplete(task),
		MutationSeq:         renderedLongHorizonMutationSequence(task),
		VerifiedMutationSeq: renderedLongHorizonVerifiedMutationSequence(task),
		Evidence:            renderedLongHorizonEvidenceState(task),
		Recovery:            recovery,
		Stop:                renderedLongHorizonStopDecision(task),
	}
	observation.CompletionEligible = observation.CanStop()
	proof := agent.CompletionProof(task)
	return RenderedLongHorizonObservation{
		Outcome: observation,
		Rendered: RenderedLongHorizonProjection{
			TaskPresent:      true,
			TaskStatus:       task.Status,
			ProgressVisible:  true,
			CompletionReady:  proof.Ready,
			CompletionMarker: proof.Ready,
			ChangedFiles:     append([]string(nil), task.ChangedFiles...),
		},
	}
}

func renderedLongHorizonCriteriaComplete(task *taskstate.Task) bool {
	if task == nil || len(task.RequiredCriterionIndices()) == 0 {
		return false
	}
	for _, index := range task.RequiredCriterionIndices() {
		status, current := task.CriterionEvidenceState(index)
		if !current || status != "PASS" {
			return false
		}
	}
	return true
}

func renderedLongHorizonMutationSequence(task *taskstate.Task) uint64 {
	if task == nil || task.ChangeSeq < 0 {
		return 0
	}
	return uint64(task.ChangeSeq)
}

func renderedLongHorizonVerifiedMutationSequence(task *taskstate.Task) uint64 {
	if task == nil || task.VerifiedChangeSeq < 0 {
		return 0
	}
	return uint64(task.VerifiedChangeSeq)
}

func renderedLongHorizonEvidenceState(task *taskstate.Task) benchmark.EvidenceState {
	if task == nil {
		return benchmark.EvidenceUnverified
	}
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

func renderedLongHorizonStopDecision(task *taskstate.Task) benchmark.StopDecision {
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

func newRenderedLongHorizonFixtureTestReport(t *testing.T) RenderedLongHorizonReport {
	t.Helper()
	source, verified, dirty, err := renderedRecoveryFixtureSource()
	if err != nil {
		t.Fatal(err)
	}
	if len(source) != 40 {
		output, gitErr := exec.Command("git", "rev-parse", "--verify", "HEAD^{commit}").CombinedOutput()
		if gitErr != nil {
			t.Fatalf("resolve source head: %v\n%s", gitErr, output)
		}
		source = strings.TrimSpace(string(output))
		verified = false
		dirty = true
	}
	unverified := []string{
		"direct browser DOM and screenshot observations are outside the medium fixture lane",
		"live-provider quality is not measured by the deterministic local provider",
	}
	if !verified || dirty {
		unverified = append(unverified, "source provenance was not a clean verified build")
	}
	return RenderedLongHorizonReport{
		Schema:          RenderedLongHorizonSchema,
		Scenario:        "rendered-multi-turn-outcome",
		SourceHead:      source,
		SourceVerified:  verified,
		SourceTreeDirty: dirty,
		Host:            runtime.GOOS + "/" + runtime.GOARCH,
		Runtime:         runtime.Version(),
		BrowserSession:  "task-owned-rendered-fixture-test",
		BrowserTab:      "httptest-rendered-fixture",
		ObservedAtUTC:   time.Now().UTC().Format(time.RFC3339Nano),
		Command:         renderedLongHorizonFixtureTestCommand,
		Unverified:      unverified,
		Observations:    make([]RenderedLongHorizonObservation, 0, 5),
	}
}

func renderedLongHorizonStateTurnRevision(state map[string]any) uint64 {
	task, ok := state["task"].(map[string]any)
	if !ok {
		return 0
	}
	turns, ok := task["turns"].([]any)
	if !ok || len(turns) == 0 {
		return 0
	}
	last, ok := turns[len(turns)-1].(map[string]any)
	if !ok {
		return 0
	}
	sequence, _ := last["sequence"].(float64)
	return uint64(sequence)
}

func renderedLongHorizonStateHasPendingPermission(state map[string]any) bool {
	_, ok := state["pending_perm"].(map[string]any)
	return ok
}
