// Package lifecycle defines the bounded cross-surface lifecycle contract used
// by deterministic tests and evidence records. The taskstate and outcome
// packages remain authoritative; this package only names scenarios and
// captures observations at their integration boundaries.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/saiaathish/picogent/internal/outcome"
	"github.com/saiaathish/picogent/internal/taskstate"
)

// Surface identifies a supported execution entry point.
type Surface string

const (
	SurfaceHeadless Surface = "headless"
	SurfaceTUI      Surface = "tui"
	SurfaceGUI      Surface = "gui"
)

func (s Surface) Valid() bool {
	switch s {
	case SurfaceHeadless, SurfaceTUI, SurfaceGUI:
		return true
	default:
		return false
	}
}

// Trigger identifies the boundary event under test. It is deliberately
// smaller than the provider/tool error vocabulary so scenarios stay portable
// across entry points.
type Trigger string

const (
	TriggerEOF                Trigger = "eof"
	TriggerSignal             Trigger = "signal"
	TriggerCancellation       Trigger = "cancellation"
	TriggerShutdown           Trigger = "shutdown"
	TriggerReconnect          Trigger = "reconnect"
	TriggerProcessKill        Trigger = "process_kill"
	TriggerTaskSaveFailure    Trigger = "task_save_failure"
	TriggerSessionSaveFailure Trigger = "session_save_failure"
)

func (t Trigger) Valid() bool {
	switch t {
	case TriggerEOF, TriggerSignal, TriggerCancellation, TriggerShutdown,
		TriggerReconnect, TriggerProcessKill, TriggerTaskSaveFailure,
		TriggerSessionSaveFailure:
		return true
	default:
		return false
	}
}

// ErrorClass is the bounded user-visible error vocabulary used by the
// scenario matrix. It intentionally does not retain provider or tool text.
type ErrorClass string

const (
	ErrorNone               ErrorClass = "none"
	ErrorCanceled           ErrorClass = "canceled"
	ErrorPermission         ErrorClass = "permission"
	ErrorProvider           ErrorClass = "provider"
	ErrorReconnect          ErrorClass = "reconnect"
	ErrorTaskPersistence    ErrorClass = "task_persistence"
	ErrorSessionPersistence ErrorClass = "session_persistence"
	ErrorUnknown            ErrorClass = "unknown"
)

func (c ErrorClass) Valid() bool {
	switch c {
	case ErrorNone, ErrorCanceled, ErrorPermission, ErrorProvider,
		ErrorReconnect, ErrorTaskPersistence, ErrorSessionPersistence, ErrorUnknown:
		return true
	default:
		return false
	}
}

// EvidenceStatus says how much of a scenario is currently recorded. The
// initial contract intentionally starts unverified; integration PRs can
// advance individual rows after fresh-process or rendered evidence exists.
type EvidenceStatus string

const (
	EvidenceUnverified    EvidenceStatus = "UNVERIFIED"
	EvidenceDeterministic EvidenceStatus = "DETERMINISTIC"
	EvidenceFreshProcess  EvidenceStatus = "FRESH_PROCESS"
	EvidenceRendered      EvidenceStatus = "RENDERED"
)

func (s EvidenceStatus) Valid() bool {
	switch s {
	case EvidenceUnverified, EvidenceDeterministic, EvidenceFreshProcess, EvidenceRendered:
		return true
	default:
		return false
	}
}

// PersistenceExpectation describes only the durable state that a scenario
// must retain or prove. Zero-valued task/turn fields mean the scenario does
// not yet make an exact assertion for that field.
type PersistenceExpectation struct {
	TaskStatus        taskstate.Status     `json:"task_status,omitempty"`
	TurnState         taskstate.TurnState  `json:"turn_state,omitempty"`
	TurnRoute         string               `json:"turn_route,omitempty"`
	StopReason        taskstate.StopReason `json:"stop_reason,omitempty"`
	MustRetainTask    bool                 `json:"must_retain_task,omitempty"`
	MustBeRecoverable bool                 `json:"must_be_recoverable,omitempty"`
	Note              string               `json:"note"`
}

// CompletionExpectation records the shared fail-closed boundary that every
// interruption and persistence-failure scenario must enforce.
type CompletionExpectation struct {
	MustNotBeReady    bool `json:"must_not_be_ready"`
	MustNotShowMarker bool `json:"must_not_show_marker"`
}

// Scenario is one row of the cross-surface lifecycle matrix.
type Scenario struct {
	ID                   string                 `json:"id"`
	Surface              Surface                `json:"surface"`
	Trigger              Trigger                `json:"trigger"`
	Persisted            PersistenceExpectation `json:"persisted"`
	Completion           CompletionExpectation  `json:"completion"`
	UserVisibleError     ErrorClass             `json:"user_visible_error"`
	FreshProcessRequired bool                   `json:"fresh_process_required"`
	Evidence             EvidenceStatus         `json:"evidence"`
}

var scenarioTable = []Scenario{
	{
		ID: "headless-eof-permission", Surface: SurfaceHeadless, Trigger: TriggerEOF,
		Persisted:        PersistenceExpectation{Note: "no turn is admitted; prior durable task state remains unchanged"},
		Completion:       CompletionExpectation{MustNotBeReady: true, MustNotShowMarker: true},
		UserVisibleError: ErrorPermission, FreshProcessRequired: true, Evidence: EvidenceUnverified,
	},
	{
		ID: "headless-signal-active-turn", Surface: SurfaceHeadless, Trigger: TriggerSignal,
		Persisted: PersistenceExpectation{
			TaskStatus: taskstate.StatusWorking, TurnState: taskstate.TurnInterrupted,
			TurnRoute: string(taskstate.TurnRouteRecover), StopReason: taskstate.StopCanceled,
			MustRetainTask: true, MustBeRecoverable: true,
			Note: "the active turn is interrupted durably before the one-shot process exits",
		},
		Completion:       CompletionExpectation{MustNotBeReady: true, MustNotShowMarker: true},
		UserVisibleError: ErrorCanceled, FreshProcessRequired: true, Evidence: EvidenceUnverified,
	},
	{
		ID: "headless-process-kill-active-turn", Surface: SurfaceHeadless, Trigger: TriggerProcessKill,
		Persisted: PersistenceExpectation{
			TaskStatus: taskstate.StatusWorking, TurnState: taskstate.TurnInterrupted,
			TurnRoute: string(taskstate.TurnRouteRecover), StopReason: taskstate.StopProcessRestart,
			MustRetainTask: true, MustBeRecoverable: true,
			Note: "an abrupt headless process kill skips graceful cleanup; the next invocation recovers the durably admitted active turn",
		},
		Completion:       CompletionExpectation{MustNotBeReady: true, MustNotShowMarker: true},
		UserVisibleError: ErrorNone, FreshProcessRequired: true, Evidence: EvidenceUnverified,
	},
	{
		ID: "headless-task-save-failure", Surface: SurfaceHeadless, Trigger: TriggerTaskSaveFailure,
		Persisted: PersistenceExpectation{
			TaskStatus: taskstate.StatusWorking, TurnState: taskstate.TurnActive,
			MustRetainTask: true, MustBeRecoverable: true,
			Note: "the last durable checkpoint remains resumable; failed close cannot publish completion",
		},
		Completion:       CompletionExpectation{MustNotBeReady: true, MustNotShowMarker: true},
		UserVisibleError: ErrorTaskPersistence, FreshProcessRequired: true, Evidence: EvidenceUnverified,
	},
	{
		ID: "tui-eof-clean-exit", Surface: SurfaceTUI, Trigger: TriggerEOF,
		Persisted:        PersistenceExpectation{Note: "no new turn is admitted; the existing session/task snapshot is retained"},
		Completion:       CompletionExpectation{MustNotBeReady: true, MustNotShowMarker: true},
		UserVisibleError: ErrorNone, FreshProcessRequired: false, Evidence: EvidenceUnverified,
	},
	{
		ID: "tui-signal-active-turn", Surface: SurfaceTUI, Trigger: TriggerSignal,
		Persisted: PersistenceExpectation{
			TaskStatus: taskstate.StatusWorking, TurnState: taskstate.TurnInterrupted,
			TurnRoute: string(taskstate.TurnRouteRecover), StopReason: taskstate.StopCanceled,
			MustRetainTask: true, MustBeRecoverable: true,
			Note: "Ctrl-C or owning-context cancellation stops the active turn before Bubble Tea cleanup",
		},
		Completion:       CompletionExpectation{MustNotBeReady: true, MustNotShowMarker: true},
		UserVisibleError: ErrorCanceled, FreshProcessRequired: true, Evidence: EvidenceUnverified,
	},
	{
		ID: "tui-process-kill-active-turn", Surface: SurfaceTUI, Trigger: TriggerProcessKill,
		Persisted: PersistenceExpectation{
			TaskStatus: taskstate.StatusWorking, TurnState: taskstate.TurnInterrupted,
			TurnRoute: string(taskstate.TurnRouteRecover), StopReason: taskstate.StopProcessRestart,
			MustRetainTask: true, MustBeRecoverable: true,
			Note: "an abrupt TUI process kill skips graceful cleanup; the next model recovers the durably admitted active turn",
		},
		Completion:       CompletionExpectation{MustNotBeReady: true, MustNotShowMarker: true},
		UserVisibleError: ErrorNone, FreshProcessRequired: true, Evidence: EvidenceUnverified,
	},
	{
		ID: "tui-session-save-failure", Surface: SurfaceTUI, Trigger: TriggerSessionSaveFailure,
		Persisted: PersistenceExpectation{
			MustRetainTask: true, MustBeRecoverable: true,
			Note: "the durable task remains resumable while the session-save error is visible",
		},
		Completion:       CompletionExpectation{MustNotBeReady: true, MustNotShowMarker: true},
		UserVisibleError: ErrorSessionPersistence, FreshProcessRequired: false, Evidence: EvidenceUnverified,
	},
	{
		ID: "gui-shutdown-active-turn", Surface: SurfaceGUI, Trigger: TriggerShutdown,
		Persisted: PersistenceExpectation{
			MustRetainTask: true,
			Note:           "shutdown stops new admission and waits for or durably interrupts the active turn before cleanup returns",
		},
		Completion:       CompletionExpectation{MustNotBeReady: true, MustNotShowMarker: true},
		UserVisibleError: ErrorNone, FreshProcessRequired: true, Evidence: EvidenceUnverified,
	},
	{
		ID: "gui-process-kill-active-turn", Surface: SurfaceGUI, Trigger: TriggerProcessKill,
		Persisted: PersistenceExpectation{
			TaskStatus: taskstate.StatusWorking, TurnState: taskstate.TurnInterrupted,
			TurnRoute: string(taskstate.TurnRouteRecover), StopReason: taskstate.StopProcessRestart,
			MustRetainTask: true, MustBeRecoverable: true,
			Note: "an abrupt GUI process kill skips graceful cleanup; the next process recovers the durably admitted active turn",
		},
		Completion:       CompletionExpectation{MustNotBeReady: true, MustNotShowMarker: true},
		UserVisibleError: ErrorNone, FreshProcessRequired: true, Evidence: EvidenceUnverified,
	},
	{
		ID: "gui-reconnect-active-turn", Surface: SurfaceGUI, Trigger: TriggerReconnect,
		Persisted: PersistenceExpectation{
			MustRetainTask: true,
			Note:           "the current session/turn remains authoritative; reconnect must not graft stale transcript state",
		},
		Completion:       CompletionExpectation{MustNotBeReady: true, MustNotShowMarker: true},
		UserVisibleError: ErrorNone, FreshProcessRequired: false, Evidence: EvidenceUnverified,
	},
	{
		ID: "gui-task-save-failure", Surface: SurfaceGUI, Trigger: TriggerTaskSaveFailure,
		Persisted: PersistenceExpectation{
			TaskStatus: taskstate.StatusWorking, TurnState: taskstate.TurnActive,
			MustRetainTask: true, MustBeRecoverable: true,
			Note: "the prior durable task checkpoint remains recoverable and the GUI cannot render completion",
		},
		Completion:       CompletionExpectation{MustNotBeReady: true, MustNotShowMarker: true},
		UserVisibleError: ErrorTaskPersistence, FreshProcessRequired: false, Evidence: EvidenceUnverified,
	},
	{
		ID: "gui-session-save-failure", Surface: SurfaceGUI, Trigger: TriggerSessionSaveFailure,
		Persisted: PersistenceExpectation{
			MustRetainTask: true, MustBeRecoverable: true,
			Note: "session persistence failure is visible without replacing the current durable task snapshot",
		},
		Completion:       CompletionExpectation{MustNotBeReady: true, MustNotShowMarker: true},
		UserVisibleError: ErrorSessionPersistence, FreshProcessRequired: false, Evidence: EvidenceUnverified,
	},
}

// Scenarios returns an isolated copy of the canonical scenario table.
func Scenarios() []Scenario {
	return append([]Scenario(nil), scenarioTable...)
}

// ValidateScenarioTable checks the structural contract before tests or
// evidence tooling consume it. It intentionally does not claim that a row is
// proven; Evidence records that separately.
func ValidateScenarioTable(scenarios []Scenario) error {
	if len(scenarios) == 0 {
		return errors.New("lifecycle scenario table is empty")
	}
	seen := make(map[string]struct{}, len(scenarios))
	fresh := make(map[Surface]bool, 3)
	for _, scenario := range scenarios {
		id := strings.TrimSpace(scenario.ID)
		if id == "" {
			return errors.New("lifecycle scenario ID is empty")
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("duplicate lifecycle scenario %q", id)
		}
		seen[id] = struct{}{}
		if !scenario.Surface.Valid() {
			return fmt.Errorf("scenario %q has invalid surface %q", id, scenario.Surface)
		}
		if !scenario.Trigger.Valid() {
			return fmt.Errorf("scenario %q has invalid trigger %q", id, scenario.Trigger)
		}
		if !scenario.UserVisibleError.Valid() {
			return fmt.Errorf("scenario %q has invalid user-visible error %q", id, scenario.UserVisibleError)
		}
		if !scenario.Evidence.Valid() {
			return fmt.Errorf("scenario %q has invalid evidence status %q", id, scenario.Evidence)
		}
		if strings.TrimSpace(scenario.Persisted.Note) == "" {
			return fmt.Errorf("scenario %q has no persistence note", id)
		}
		if !scenario.Completion.MustNotBeReady || !scenario.Completion.MustNotShowMarker {
			return fmt.Errorf("scenario %q is not fail-closed", id)
		}
		if scenario.FreshProcessRequired {
			fresh[scenario.Surface] = true
		}
	}
	for _, surface := range []Surface{SurfaceHeadless, SurfaceTUI, SurfaceGUI} {
		if !fresh[surface] {
			return fmt.Errorf("surface %q has no fresh-process scenario", surface)
		}
	}
	return nil
}

// CompletionProjection is the surface-neutral part of agent completion
// projection needed by lifecycle assertions.
type CompletionProjection struct {
	Required bool `json:"required"`
	Ready    bool `json:"ready"`
	Marker   bool `json:"marker"`
}

// ErrorSummary preserves the primary failure class separately from a
// durability failure. This lets tests assert that a provider/cancellation
// error was not swallowed when the close/save path failed too.
type ErrorSummary struct {
	Primary    ErrorClass `json:"primary"`
	Durability ErrorClass `json:"durability"`
}

func (s ErrorSummary) VisibleClass() ErrorClass {
	if s.Primary != ErrorNone && s.Primary != "" {
		return s.Primary
	}
	if s.Durability != ErrorNone && s.Durability != "" {
		return s.Durability
	}
	return ErrorNone
}

func (s ErrorSummary) HasError() bool {
	return s.Primary != ErrorNone && s.Primary != "" || s.Durability != ErrorNone && s.Durability != ""
}

// SummarizeError maps an integration error to bounded classes without
// retaining untrusted provider, tool, or filesystem text.
func SummarizeError(err error) ErrorSummary {
	if err == nil {
		return ErrorSummary{Primary: ErrorNone, Durability: ErrorNone}
	}
	text := strings.ToLower(err.Error())
	summary := ErrorSummary{Primary: ErrorNone, Durability: ErrorNone}
	setPrimary := func(class ErrorClass) {
		if summary.Primary == ErrorNone || summary.Primary == "" {
			summary.Primary = class
		}
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || strings.Contains(text, "context canceled") || strings.Contains(text, "context cancelled") || strings.Contains(text, "interrupted before completion") {
		setPrimary(ErrorCanceled)
	}
	if strings.Contains(text, "permission") {
		setPrimary(ErrorPermission)
	}
	if strings.Contains(text, "provider") || strings.Contains(text, "resource unavailable") {
		setPrimary(ErrorProvider)
	}
	if strings.Contains(text, "reconnect") || strings.Contains(text, "event stream") {
		setPrimary(ErrorReconnect)
	}

	if strings.Contains(text, "durable task state") || strings.Contains(text, "task state") && strings.Contains(text, "save") {
		summary.Durability = ErrorTaskPersistence
	}
	if strings.Contains(text, "save session") || strings.Contains(text, "session save") || strings.Contains(text, "couldn't save session") || strings.Contains(text, "could not save session") {
		summary.Durability = ErrorSessionPersistence
	}
	if summary.Primary == ErrorNone || summary.Primary == "" {
		if summary.Durability == ErrorNone || summary.Durability == "" {
			summary.Primary = ErrorUnknown
		}
	}
	return summary
}

// Observation is a read-only snapshot at a surface boundary. Outcome is the
// existing bounded projection; this type adds only the surface metadata and
// completion/error observations needed to compare entry points.
type Observation struct {
	ScenarioID  string               `json:"scenario_id"`
	Surface     Surface              `json:"surface"`
	Trigger     Trigger              `json:"trigger"`
	TaskPresent bool                 `json:"task_present"`
	TaskStatus  taskstate.Status     `json:"task_status,omitempty"`
	Outcome     outcome.TurnContract `json:"outcome"`
	Completion  CompletionProjection `json:"completion"`
	Error       ErrorSummary         `json:"error"`
}

// Observe captures a task without mutating it. Task state and the outcome
// projection are copied at the call boundary, so later callbacks cannot alter
// the observation under test.
func Observe(scenarioID string, surface Surface, trigger Trigger, task *taskstate.Task, completion CompletionProjection, err error) Observation {
	observation := Observation{
		ScenarioID: scenarioID, Surface: surface, Trigger: trigger,
		Completion: completion, Error: SummarizeError(err),
		Outcome: outcome.TurnContractForTask(task),
	}
	if task != nil {
		observation.TaskPresent = true
		observation.TaskStatus = task.Status
	}
	return observation
}

// Recoverable reports whether the captured durable task is still resumable.
// A completed task is never considered recoverable, even if a malformed
// caller also reports an interrupted last turn; the invariant checker flags
// that contradictory state separately.
func (o Observation) Recoverable() bool {
	return o.TaskPresent && o.TaskStatus != taskstate.StatusDone
}

// InvariantViolations returns stable, user-independent failures in the
// cross-surface lifecycle contract.
func (o Observation) InvariantViolations() []string {
	var violations []string
	interrupted := strings.EqualFold(o.Outcome.LastTurnState, string(taskstate.TurnInterrupted))
	if interrupted {
		if o.Completion.Ready {
			violations = append(violations, "interrupted turn was projected as complete")
		}
		if o.Completion.Marker {
			violations = append(violations, "interrupted turn retained a completion marker")
		}
		if o.TaskStatus == taskstate.StatusDone {
			violations = append(violations, "interrupted turn left a done task")
		}
		if !strings.EqualFold(o.Outcome.LastRoute, string(taskstate.TurnRouteRecover)) {
			violations = append(violations, "interrupted turn did not route to recovery")
		}
		if !o.Recoverable() {
			violations = append(violations, "interrupted turn did not retain recoverable task state")
		}
	}
	if o.Error.HasError() && o.Completion.Ready {
		violations = append(violations, "failed lifecycle boundary was projected as complete")
	}
	if o.Error.Durability != ErrorNone && o.Error.Durability != "" && o.Completion.Ready {
		violations = append(violations, "durability failure was projected as complete")
	}
	if o.Completion.Marker && !o.Completion.Required {
		violations = append(violations, "completion marker was accepted without an active gate")
	}
	return violations
}

// Check compares an observation with one named scenario and then applies the
// generic invariants. It returns deterministic text suitable for test output;
// it never includes the underlying error string.
func (s Scenario) Check(observation Observation) []string {
	violations := observation.InvariantViolations()
	if observation.ScenarioID != s.ID {
		violations = append(violations, fmt.Sprintf("scenario ID = %q, want %q", observation.ScenarioID, s.ID))
	}
	if observation.Surface != s.Surface {
		violations = append(violations, fmt.Sprintf("surface = %q, want %q", observation.Surface, s.Surface))
	}
	if observation.Trigger != s.Trigger {
		violations = append(violations, fmt.Sprintf("trigger = %q, want %q", observation.Trigger, s.Trigger))
	}
	if s.Completion.MustNotBeReady && observation.Completion.Ready {
		violations = append(violations, "scenario allowed a ready completion projection")
	}
	if s.Completion.MustNotShowMarker && observation.Completion.Marker {
		violations = append(violations, "scenario allowed a completion marker")
	}
	if expected := s.Persisted.TaskStatus; expected != "" && observation.TaskStatus != expected {
		violations = append(violations, fmt.Sprintf("task status = %q, want %q", observation.TaskStatus, expected))
	}
	if expected := s.Persisted.TurnState; expected != "" && observation.Outcome.LastTurnState != string(expected) {
		violations = append(violations, fmt.Sprintf("turn state = %q, want %q", observation.Outcome.LastTurnState, expected))
	}
	if expected := strings.TrimSpace(s.Persisted.TurnRoute); expected != "" && observation.Outcome.LastRoute != expected {
		violations = append(violations, fmt.Sprintf("turn route = %q, want %q", observation.Outcome.LastRoute, expected))
	}
	if expected := s.Persisted.StopReason; expected != "" && observation.Outcome.LastTurnStopReason != string(expected) {
		violations = append(violations, fmt.Sprintf("stop reason = %q, want %q", observation.Outcome.LastTurnStopReason, expected))
	}
	if s.Persisted.MustRetainTask && !observation.TaskPresent {
		violations = append(violations, "scenario did not retain a durable task")
	}
	if s.Persisted.MustBeRecoverable && !observation.Recoverable() {
		violations = append(violations, "scenario did not retain recoverable task state")
	}
	if expected := s.UserVisibleError; observation.Error.VisibleClass() != expected {
		violations = append(violations, fmt.Sprintf("user-visible error class = %q, want %q", observation.Error.VisibleClass(), expected))
	}
	return violations
}
