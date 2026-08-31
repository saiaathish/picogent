// Package taskstate provides Picogent's compact, durable execution state.
// It intentionally stores outcomes and progress, not chat history or reasoning.
package taskstate

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/verify"
	"github.com/saiaathish/picogent/internal/workspace"
)

const CurrentVersion = 1

const (
	// Verification coverage describes whether a workspace observation binds
	// the complete proof boundary for the task or only a safe subset.
	VerificationCoverageComplete = "complete"
	VerificationCoveragePartial  = "partial"
	VerificationCoverageUnbound  = "unbound"
)

const (
	maxTaskGoal             = 600
	maxTaskIdentity         = 200
	maxTaskSteps            = 8
	maxStepDescription      = 300
	maxChangedFiles         = 128
	maxChangedFilePath      = 500
	maxVerification         = 32
	maxVerificationCommand  = 300
	maxVerificationSummary  = 800
	maxVerificationCoverage = 16
	maxOutcomeNotes         = 8
	maxOutcomeNote          = 500
	maxEvidence             = 16
	maxEvidenceKind         = 48
	maxEvidenceStatus       = 32
	maxEvidenceSource       = 64
	maxEvidenceOrigin       = 32
	maxEvidenceSummary      = 800
	maxEvidenceReference    = 300
	maxEvidenceConfidence   = 24
	maxIntentClass          = 48
	maxIntentAction         = 64
	maxIntentCompleteness   = 32
	maxIntentScope          = 300
	maxIntentRisk           = 32
	maxIntentConfidence     = 24
	maxTaskAttempts         = 128
	maxBlockedBy            = 500
)

// Status is the current phase of a task.
type Status string

const (
	StatusPlanning  Status = "planning"
	StatusWorking   Status = "working"
	StatusVerifying Status = "verifying"
	StatusBlocked   Status = "blocked"
	StatusDone      Status = "done"
)

// Step is one durable unit of task progress.
type Step struct {
	Description string `json:"description"`
	Done        bool   `json:"done,omitempty"`
}

// Verification records concise evidence from a check. Output should be
// summarized before storage so task state remains cheap to inject after resume.
type Verification struct {
	Command string `json:"command,omitempty"`
	Passed  bool   `json:"passed"`
	Summary string `json:"summary,omitempty"`
	// Coverage is empty in pre-v4 state. When an observation exists, that
	// legacy value is interpreted as complete; new partial records are
	// explicit so they cannot authorize durable completion after a cap.
	Coverage    string                 `json:"coverage,omitempty"`
	Observation *workspace.Observation `json:"observation,omitempty"`
	At          time.Time              `json:"at"`
	// trusted is runtime-only provenance. Persisted labels and booleans are
	// advisory after reload and must be re-established by a live producer.
	trusted bool
}

// EvidenceKind identifies the proof boundary an evidence record covers. The
// kind is deliberately separate from Source: a source describes where a
// record came from, while the kind says what claim it can support.
type EvidenceKind string

const (
	EvidenceKindVerification EvidenceKind = "verification"
	EvidenceKindResearch     EvidenceKind = "research"
	EvidenceKindMeasurement  EvidenceKind = "measurement"
	EvidenceKindVisual       EvidenceKind = "visual"
	EvidenceKindTests        EvidenceKind = "tests"
	EvidenceKindApproval     EvidenceKind = "approval"
	EvidenceKindInspection   EvidenceKind = "inspection"

	// EvidenceKindTest is accepted as a compatibility spelling and is
	// canonicalized to EvidenceKindTests when evidence is stored.
	EvidenceKindTest EvidenceKind = "test"
)

// EvidenceOrigin identifies the mechanism that produced a proof record. Only
// known origin/kind pairs can satisfy an inferred quality requirement; model
// narration and arbitrary repository text are never trusted origins.
type EvidenceOrigin string

const (
	EvidenceOriginVerifier         EvidenceOrigin = "verifier"
	EvidenceOriginWorkspaceTool    EvidenceOrigin = "workspace_tool"
	EvidenceOriginResearchTool     EvidenceOrigin = "research_tool"
	EvidenceOriginExternalDocs     EvidenceOrigin = "external_docs"
	EvidenceOriginMeasurementTool  EvidenceOrigin = "measurement_tool"
	EvidenceOriginBenchmark        EvidenceOrigin = "benchmark"
	EvidenceOriginBrowser          EvidenceOrigin = "browser"
	EvidenceOriginVisualInspection EvidenceOrigin = "visual_inspection"
	EvidenceOriginTestRunner       EvidenceOrigin = "test_runner"
	EvidenceOriginUserApproval     EvidenceOrigin = "user_approval"
	EvidenceOriginUser             EvidenceOrigin = "user"
	EvidenceOriginSystem           EvidenceOrigin = "system"
	EvidenceOriginModel            EvidenceOrigin = "model"
)

// Valid reports whether the evidence kind is part of the bounded vocabulary.
// Unknown kinds remain storable as advisory evidence for forward compatibility
// but cannot satisfy a completion requirement.
func (k EvidenceKind) Valid() bool {
	switch normalizeEvidenceKind(k) {
	case EvidenceKindVerification, EvidenceKindResearch, EvidenceKindMeasurement,
		EvidenceKindVisual, EvidenceKindTests, EvidenceKindApproval, EvidenceKindInspection:
		return true
	default:
		return false
	}
}

// Valid reports whether the origin is a known producer label. A known label
// is not by itself sufficient proof; TrustedFor also checks the proof kind.
func (o EvidenceOrigin) Valid() bool {
	switch o {
	case EvidenceOriginVerifier, EvidenceOriginWorkspaceTool, EvidenceOriginResearchTool,
		EvidenceOriginExternalDocs, EvidenceOriginMeasurementTool, EvidenceOriginBenchmark,
		EvidenceOriginBrowser, EvidenceOriginVisualInspection, EvidenceOriginTestRunner,
		EvidenceOriginUserApproval, EvidenceOriginUser, EvidenceOriginSystem, EvidenceOriginModel:
		return true
	default:
		return false
	}
}

// TrustedFor reports whether this origin may satisfy the requested proof kind.
// This is intentionally a narrow allow-list. In particular, OriginModel and
// OriginSystem never satisfy quality requirements merely because a caller
// labels their text as a pass.
func (o EvidenceOrigin) TrustedFor(kind EvidenceKind) bool {
	switch normalizeEvidenceKind(kind) {
	case EvidenceKindVerification:
		return o == EvidenceOriginVerifier || o == EvidenceOriginWorkspaceTool || o == EvidenceOriginTestRunner
	case EvidenceKindResearch:
		return o == EvidenceOriginResearchTool || o == EvidenceOriginExternalDocs
	case EvidenceKindMeasurement:
		return o == EvidenceOriginMeasurementTool || o == EvidenceOriginBenchmark
	case EvidenceKindVisual:
		return o == EvidenceOriginBrowser || o == EvidenceOriginVisualInspection
	case EvidenceKindTests:
		return o == EvidenceOriginVerifier || o == EvidenceOriginWorkspaceTool || o == EvidenceOriginTestRunner
	case EvidenceKindApproval:
		return o == EvidenceOriginUserApproval || o == EvidenceOriginUser
	default:
		return false
	}
}

// Evidence is a compact, source-labelled fact used to reason about an
// outcome. Raw command output stays outside durable state; Summary is a
// bounded distillation and Reference points at the useful source.
type Evidence struct {
	Kind       EvidenceKind   `json:"kind"`
	Status     string         `json:"status"`
	Source     string         `json:"source,omitempty"`
	Origin     EvidenceOrigin `json:"origin,omitempty"`
	Summary    string         `json:"summary"`
	Reference  string         `json:"reference,omitempty"`
	Confidence string         `json:"confidence,omitempty"`
	ChangeSeq  int            `json:"change_seq,omitempty"`
	// CriterionIndex is nil for legacy or aggregate evidence. A pointer keeps
	// an explicit reference to criterion zero distinct from an unbound record.
	CriterionIndex *int      `json:"criterion_index,omitempty"`
	At             time.Time `json:"at"`
	// trusted is runtime-only provenance. Origin is retained for audit display,
	// but a serialized or generic caller-supplied label cannot satisfy proof.
	trusted bool
}

// IntentContract is the compact, internal interpretation of a user request.
// It keeps vague intent and its risk/proof implications durable without
// exposing a planning mode or requiring the user to name agent concepts.
type IntentContract struct {
	Outcome          string `json:"outcome"`
	Class            string `json:"class,omitempty"`
	Action           string `json:"action,omitempty"`
	Completeness     string `json:"completeness,omitempty"`
	Scope            string `json:"scope,omitempty"`
	Risk             string `json:"risk,omitempty"`
	NeedsResearch    bool   `json:"needs_research,omitempty"`
	NeedsMeasurement bool   `json:"needs_measurement,omitempty"`
	NeedsVisual      bool   `json:"needs_visual,omitempty"`
	NeedsTests       bool   `json:"needs_tests,omitempty"`
	NeedsApproval    bool   `json:"needs_approval,omitempty"`
	Confidence       string `json:"confidence,omitempty"`
}

// Criterion is one compact, internal definition-of-done item. Evidence is
// recorded separately in Verification so raw model narration does not become
// durable task state.
type Criterion struct {
	Description string `json:"description"`
	Required    bool   `json:"required,omitempty"`
}

// RequirementEvidenceState is the compact durable status of one inferred
// quality requirement. Current is true only when a recognized origin supplied
// a current passing record for the matching kind.
type RequirementEvidenceState struct {
	Kind    EvidenceKind   `json:"kind"`
	Status  string         `json:"status"`
	Origin  EvidenceOrigin `json:"origin,omitempty"`
	Current bool           `json:"current"`
}

// CompletionCheck is the durable explanation of the completion predicate. It
// exposes the exact missing criteria and inferred proof requirements instead
// of reducing an incomplete outcome to one opaque boolean.
type CompletionCheck struct {
	Ready                bool                       `json:"ready"`
	MissingCriteria      []int                      `json:"missing_criteria,omitempty"`
	MissingRequirements  []EvidenceKind             `json:"missing_requirements,omitempty"`
	Requirements         []RequirementEvidenceState `json:"requirements,omitempty"`
	VerificationRequired bool                       `json:"verification_required,omitempty"`
	VerificationCurrent  bool                       `json:"verification_current,omitempty"`
	ChangedFilesCapped   bool                       `json:"changed_files_capped,omitempty"`
	Reason               string                     `json:"reason,omitempty"`
}

// Task is the compact state required to resume an execution loop.
type Task struct {
	Version   int    `json:"version"`
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	// Revision is the persisted compare-and-swap generation. A zero value is
	// accepted for legacy or not-yet-saved state; Store.Save advances it only
	// after confirming that the on-disk generation still matches.
	Revision           uint64          `json:"revision,omitempty"`
	Goal               string          `json:"goal"`
	Intent             *IntentContract `json:"intent,omitempty"`
	IntentRevision     uint64          `json:"intent_revision,omitempty"`
	DefinitionOfDone   []Criterion     `json:"definition_of_done,omitempty"`
	Status             Status          `json:"status"`
	Steps              []Step          `json:"steps,omitempty"`
	CurrentStep        int             `json:"current_step"`
	Attempts           int             `json:"attempts"`
	ChangedFiles       []string        `json:"changed_files,omitempty"`
	ChangedFilesCapped bool            `json:"changed_files_capped,omitempty"`
	ChangeSeq          int             `json:"change_seq,omitempty"`
	// VerifiedChangeSeq is the latest change sequence covered by passing
	// verification. A negative value records that the latest evidence did not
	// pass.
	VerifiedChangeSeq int            `json:"verified_change_seq,omitempty"`
	Verification      []Verification `json:"verification,omitempty"`
	Constraints       []string       `json:"constraints,omitempty"`
	Risks             []string       `json:"risks,omitempty"`
	Uncertainty       []string       `json:"uncertainty,omitempty"`
	Evidence          []Evidence     `json:"evidence,omitempty"`
	TurnRevision      uint64         `json:"turn_revision,omitempty"`
	Turns             []TurnRecord   `json:"turns,omitempty"`
	BlockedBy         string         `json:"blocked_by,omitempty"`
	StopReason        StopReason     `json:"stop_reason,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`

	// normalizedFromDone records that Store.Load reopened an unproven terminal
	// marker. It is runtime-only so direct store consumers remain fail-closed;
	// an agent may restore done only after a live workspace proof is re-bound.
	normalizedFromDone bool
}

// New creates a task associated with a persisted chat session.
func New(sessionID, goal string, steps []string) (*Task, error) {
	sessionID = strings.TrimSpace(sessionID)
	goal = compactText(goal, maxTaskGoal)
	if strings.TrimSpace(sessionID) == "" {
		return nil, errors.New("task session id is required")
	}
	if len(sessionID) > maxTaskIdentity {
		return nil, errors.New("task session id is too long")
	}
	if goal == "" {
		return nil, errors.New("task goal is required")
	}
	id, err := randomID()
	if err != nil {
		return nil, fmt.Errorf("create task id: %w", err)
	}
	now := time.Now().UTC()
	t := &Task{
		Version:   CurrentVersion,
		ID:        id,
		SessionID: sessionID,
		Goal:      goal,
		Status:    StatusPlanning,
		CreatedAt: now,
		UpdatedAt: now,
	}
	for _, step := range steps {
		if step = compactText(step, maxStepDescription); step != "" {
			t.Steps = append(t.Steps, Step{Description: step})
		}
	}
	return t, nil
}

// Validate checks persisted state before it crosses the package boundary.
func (t *Task) Validate() error {
	if t == nil {
		return errors.New("task is nil")
	}
	if t.Version != CurrentVersion {
		return fmt.Errorf("unsupported task version %d", t.Version)
	}
	if strings.TrimSpace(t.ID) == "" {
		return errors.New("task id is required")
	}
	if strings.TrimSpace(t.SessionID) == "" {
		return errors.New("task session id is required")
	}
	if strings.TrimSpace(t.Goal) == "" {
		return errors.New("task goal is required")
	}
	if len(t.Goal) > maxTaskGoal {
		return errors.New("task goal is too long")
	}
	if len(t.ID) > maxTaskIdentity || len(t.SessionID) > maxTaskIdentity {
		return errors.New("task identity is too long")
	}
	if t.Intent != nil {
		if strings.TrimSpace(t.Intent.Outcome) == "" {
			return errors.New("task intent outcome is required")
		}
		if len(t.Intent.Outcome) > maxTaskGoal {
			return errors.New("task intent outcome is too long")
		}
		if len(t.Intent.Class) > maxIntentClass || len(t.Intent.Action) > maxIntentAction || len(t.Intent.Completeness) > maxIntentCompleteness || len(t.Intent.Scope) > maxIntentScope || len(t.Intent.Risk) > maxIntentRisk || len(t.Intent.Confidence) > maxIntentConfidence {
			return errors.New("task intent metadata is too long")
		}
	}
	if len(t.Turns) > maxTurnRecords {
		return errors.New("task turn history is too long")
	}
	if len(t.Turns) > 0 && t.TurnRevision < t.Turns[len(t.Turns)-1].Sequence {
		return errors.New("task turn revision is behind turn history")
	}
	for i, turn := range t.Turns {
		if i > 0 && turn.Sequence <= t.Turns[i-1].Sequence {
			return fmt.Errorf("task turn %d sequence is not increasing", i)
		}
		if turn.Sequence == 0 {
			return fmt.Errorf("task turn %d has no sequence", i)
		}
		if !turn.State.Valid() {
			return fmt.Errorf("task turn %d has invalid state %q", i, turn.State)
		}
		if turn.Attempt < 0 || turn.Attempt > maxTaskAttempts {
			return fmt.Errorf("task turn %d attempt is out of range", i)
		}
		if turn.IntentRevision > t.IntentRevision {
			return fmt.Errorf("task turn %d intent revision is ahead of task intent", i)
		}
		if turn.Route == "" || normalizeTurnRoute(TurnRoute(turn.Route)) != turn.Route {
			return fmt.Errorf("task turn %d has invalid route %q", i, turn.Route)
		}
		if len(turn.Route) > maxTurnRoute || len(turn.Hypothesis) > maxTurnHypothesis || len(turn.EvidenceState) > maxTurnEvidence {
			return fmt.Errorf("task turn %d metadata is too long", i)
		}
		if turn.EvidenceState == "" || normalizeTurnEvidence(turn.EvidenceState) != turn.EvidenceState {
			return fmt.Errorf("task turn %d has invalid evidence state %q", i, turn.EvidenceState)
		}
		if !turn.StopReason.Valid() {
			return fmt.Errorf("task turn %d has invalid stop reason %q", i, turn.StopReason)
		}
		if turn.ToolRounds < 0 || turn.ToolRounds > maxTurnToolRounds || turn.MutationCount < 0 || turn.MutationCount > maxTurnMutations {
			return fmt.Errorf("task turn %d counts are out of range", i)
		}
		if len(turn.ChangedFiles) > maxTurnChangedFiles {
			return fmt.Errorf("task turn %d changed-file list is too long", i)
		}
		for pathIndex, path := range turn.ChangedFiles {
			if strings.TrimSpace(path) == "" || len(path) > maxChangedFilePath {
				return fmt.Errorf("task turn %d changed file %d is empty or too long", i, pathIndex)
			}
		}
		if turn.StartedAt.IsZero() {
			return fmt.Errorf("task turn %d has no start time", i)
		}
		if turn.State == TurnActive && turn.FinishedAt != nil {
			return fmt.Errorf("task turn %d active state has finish time", i)
		}
		if turn.State != TurnActive && turn.FinishedAt == nil {
			return fmt.Errorf("task turn %d closed state has no finish time", i)
		}
	}
	if len(t.Steps) > maxTaskSteps {
		return errors.New("task steps are too long")
	}
	if len(t.DefinitionOfDone) > maxTaskSteps {
		return errors.New("task definition of done is too long")
	}
	if len(t.ChangedFiles) > maxChangedFiles {
		return errors.New("task changed-file list is too long")
	}
	for i, path := range t.ChangedFiles {
		if strings.TrimSpace(path) == "" || len(path) > maxChangedFilePath {
			return fmt.Errorf("task changed file %d is empty or too long", i)
		}
	}
	for name, notes := range map[string][]string{
		"constraint":  t.Constraints,
		"risk":        t.Risks,
		"uncertainty": t.Uncertainty,
	} {
		if len(notes) > maxOutcomeNotes {
			return fmt.Errorf("task %s list is too long", name)
		}
		for i, note := range notes {
			if strings.TrimSpace(note) == "" || len(note) > maxOutcomeNote {
				return fmt.Errorf("task %s %d is empty or too long", name, i)
			}
		}
	}
	if len(t.Evidence) > maxEvidence {
		return errors.New("task evidence is too long")
	}
	if len(t.Verification) > maxVerification {
		return errors.New("task verification history is too long")
	}
	for i, verification := range t.Verification {
		if len(verification.Command) > maxVerificationCommand || len(verification.Summary) > maxVerificationSummary || len(verification.Coverage) > maxVerificationCoverage {
			return fmt.Errorf("task verification %d is too long", i)
		}
		if verification.Coverage != "" && verification.Coverage != VerificationCoverageComplete && verification.Coverage != VerificationCoveragePartial && verification.Coverage != VerificationCoverageUnbound {
			return fmt.Errorf("task verification %d has invalid coverage %q", i, verification.Coverage)
		}
		if verification.Observation != nil {
			if err := verification.Observation.Validate(); err != nil {
				return fmt.Errorf("task verification %d observation: %w", i, err)
			}
		}
	}
	for i, evidence := range t.Evidence {
		if strings.TrimSpace(string(evidence.Kind)) == "" || strings.TrimSpace(evidence.Status) == "" {
			return fmt.Errorf("task evidence %d is missing kind or status", i)
		}
		if len(evidence.Kind) > maxEvidenceKind || len(evidence.Status) > maxEvidenceStatus || len(evidence.Source) > maxEvidenceSource || len(evidence.Origin) > maxEvidenceOrigin || len(evidence.Summary) > maxEvidenceSummary || len(evidence.Reference) > maxEvidenceReference || len(evidence.Confidence) > maxEvidenceConfidence {
			return fmt.Errorf("task evidence %d is too long", i)
		}
		if strings.TrimSpace(evidence.Summary) == "" {
			return fmt.Errorf("task evidence %d summary is empty or too long", i)
		}
		if evidence.ChangeSeq < 0 || evidence.ChangeSeq > t.ChangeSeq {
			return fmt.Errorf("task evidence %d change sequence %d is invalid for change sequence %d", i, evidence.ChangeSeq, t.ChangeSeq)
		}
		if evidence.CriterionIndex != nil {
			if *evidence.CriterionIndex < 0 || *evidence.CriterionIndex >= len(t.criteriaDefinition()) {
				return fmt.Errorf("task evidence %d criterion index %d is out of range", i, *evidence.CriterionIndex)
			}
		}
	}
	for i, criterion := range t.DefinitionOfDone {
		if strings.TrimSpace(criterion.Description) == "" || len(criterion.Description) > maxStepDescription {
			return fmt.Errorf("task completion criterion %d is empty", i)
		}
	}
	if !t.Status.Valid() {
		return fmt.Errorf("invalid task status %q", t.Status)
	}
	if !t.StopReason.Valid() {
		return fmt.Errorf("invalid task stop reason %q", t.StopReason)
	}
	if t.StopReason != StopNone && t.Status != StatusBlocked {
		return errors.New("task stop reason requires blocked status")
	}
	if t.CurrentStep < 0 || t.CurrentStep > len(t.Steps) {
		return fmt.Errorf("current step %d out of range", t.CurrentStep)
	}
	if t.Attempts < 0 || t.Attempts > maxTaskAttempts {
		return errors.New("task attempts are out of range")
	}
	if t.ChangeSeq < 0 {
		return errors.New("task change sequence cannot be negative")
	}
	if t.VerifiedChangeSeq < -1 || t.VerifiedChangeSeq > t.ChangeSeq {
		return fmt.Errorf("task verified change sequence %d is invalid for change sequence %d", t.VerifiedChangeSeq, t.ChangeSeq)
	}
	if len(t.BlockedBy) > maxBlockedBy {
		return errors.New("task blocker is too long")
	}
	for i, step := range t.Steps {
		if strings.TrimSpace(step.Description) == "" || len(step.Description) > maxStepDescription {
			return fmt.Errorf("task step %d is empty or too long", i)
		}
	}
	return nil
}

// Valid reports whether s is a supported task phase.
func (s Status) Valid() bool {
	switch s {
	case StatusPlanning, StatusWorking, StatusVerifying, StatusBlocked, StatusDone:
		return true
	default:
		return false
	}
}

// SetStatus applies a legal phase transition. Done is terminal except when
// unverified persisted mutations must be reopened solely for verification.
func (t *Task) SetStatus(next Status) error {
	if t == nil {
		return errors.New("task is nil")
	}
	if !next.Valid() {
		return fmt.Errorf("invalid task status %q", next)
	}
	if t.Status == StatusDone && next != StatusDone && (next != StatusVerifying || !t.NeedsVerification()) {
		return errors.New("done task is terminal")
	}
	if next == StatusDone && !t.CompletionReady() {
		return errors.New("task completion requires current proof")
	}
	if t.Status == StatusPlanning && next == StatusVerifying {
		return errors.New("task must start working before verification")
	}
	t.Status = next
	if next != StatusBlocked {
		t.BlockedBy = ""
		t.StopReason = StopNone
	}
	t.touch()
	return nil
}

// NormalizeLegacyCompletion converts an old or externally-created done marker
// without current durable proof into resumable work. It is intentionally
// idempotent and only changes the terminal-status inconsistency; the evidence
// ledger remains available for diagnosis and revalidation.
func (t *Task) NormalizeLegacyCompletion() bool {
	if t == nil || t.Status != StatusDone || t.CompletionReady() {
		return false
	}
	t.Status = StatusWorking
	t.BlockedBy = ""
	t.StopReason = StopNone
	t.normalizedFromDone = true
	t.touch()
	return true
}

// Current returns the current incomplete step, or nil when none remains.
func (t *Task) Current() *Step {
	if t == nil || t.CurrentStep < 0 || t.CurrentStep >= len(t.Steps) {
		return nil
	}
	return &t.Steps[t.CurrentStep]
}

// Advance completes the current step and moves to the next one.
func (t *Task) Advance() bool {
	if t == nil || t.CurrentStep >= len(t.Steps) {
		return false
	}
	t.Steps[t.CurrentStep].Done = true
	t.CurrentStep++
	t.touch()
	return true
}

// NoteAttempt consumes one bounded autonomous work attempt.
func (t *Task) NoteAttempt() {
	if t == nil {
		return
	}
	t.Attempts++
	t.touch()
}

// AddChangedFiles records normalized, unique changed paths in first-seen order.
// Each nonempty path represents a successful mutation and advances ChangeSeq.
func (t *Task) AddChangedFiles(paths ...string) {
	for _, path := range paths {
		t.RecordChanged(path)
	}
}

// RecordChanged records a successful mutation. ChangedFiles stays compact and
// unique for display while ChangeSeq advances for every nonempty mutation,
// including a later edit to a path that is already listed.
func (t *Task) RecordChanged(path string) {
	if t == nil {
		return
	}
	path = normalizeChangedPath(path)
	if path == "" {
		return
	}
	t.recordActiveTurnChanged(path)
	for _, changed := range t.ChangedFiles {
		if changed == path {
			t.ChangeSeq++
			t.touch()
			return
		}
	}
	if len(t.ChangedFiles) >= maxChangedFiles {
		t.ChangeSeq++
		t.ChangedFilesCapped = true
		t.touch()
		return
	}
	t.ChangedFiles = append(t.ChangedFiles, path)
	t.ChangeSeq++
	t.touch()
}

func normalizeChangedPath(path string) string {
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	return strings.TrimPrefix(path, "./")
}

// recordActiveTurnChanged attributes a successful mutation to the active
// durable turn. The task-level list is cumulative; this bounded per-turn list
// lets recovery distinguish an interrupted file-changing turn from a read-only
// interruption without persisting tool output or checkpoint contents.
func (t *Task) recordActiveTurnChanged(path string) {
	if t == nil || len(t.Turns) == 0 {
		return
	}
	latest := &t.Turns[len(t.Turns)-1]
	if latest.State != TurnActive {
		return
	}
	for _, changed := range latest.ChangedFiles {
		if changed == path {
			return
		}
	}
	if len(latest.ChangedFiles) >= maxTurnChangedFiles {
		latest.ChangedFilesCapped = true
		return
	}
	latest.ChangedFiles = append(latest.ChangedFiles, path)
}

// InitializeChangeSequence represents a legacy changed-file list as one
// unverified mutation generation. It is safe to call repeatedly.
func (t *Task) InitializeChangeSequence() bool {
	if t == nil || t.ChangeSeq != 0 || len(t.ChangedFiles) == 0 {
		return false
	}
	t.ChangeSeq = 1
	t.touch()
	return true
}

// AddVerification appends concise verification evidence.
func (t *Task) AddVerification(command string, passed bool, summary string) {
	t.AddVerificationWithObservation(command, passed, summary, nil)
}

// AddVerificationWithObservation appends verification evidence bound to the
// observed workspace bytes. A nil observation remains valid as historical
// state, but it cannot authorize a fresh completion.
func (t *Task) AddVerificationWithObservation(command string, passed bool, summary string, observation *workspace.Observation) {
	t.addVerification(command, passed, summary, observation, verificationCoverageForObservation(observation), nil)
}

// AddVerificationForCriterion records a check as proof for one bounded
// criterion while retaining the legacy aggregate verification record.
func (t *Task) AddVerificationForCriterion(index int, command string, passed bool, summary string, observation *workspace.Observation) {
	t.addVerification(command, passed, summary, observation, verificationCoverageForObservation(observation), []int{index})
}

// AddVerificationForCriteria records one check as proof for a bounded set of
// criteria. Callers should use this when one deterministic check covers the
// whole requested outcome.
func (t *Task) AddVerificationForCriteria(indices []int, command string, passed bool, summary string, observation *workspace.Observation) {
	t.addVerification(command, passed, summary, observation, verificationCoverageForObservation(observation), indices)
}

// AddVerificationForCriteriaWithCoverage records verification whose complete
// versus partial proof boundary is known to the caller.
func (t *Task) AddVerificationForCriteriaWithCoverage(indices []int, command string, passed bool, summary string, observation *workspace.Observation, coverage string) {
	t.addVerification(command, passed, summary, observation, coverage, indices)
}

func (t *Task) addVerification(command string, passed bool, summary string, observation *workspace.Observation, coverage string, criteria []int) {
	if t == nil {
		return
	}
	coverage = normalizeVerificationCoverage(coverage, observation)
	evidenceStatus := verify.StatusFromEvidence(summary)
	if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(summary)), "VERIFY ") {
		// Preserve the pre-v4 bool-based API for older callers whose summaries
		// did not carry a machine-readable status token.
		if passed {
			evidenceStatus = verify.StatusPass
		} else {
			evidenceStatus = verify.StatusFail
		}
	} else if evidenceStatus != verify.StatusPass {
		// A canonical non-PASS token cannot be upgraded by a contradictory
		// bool supplied by a caller.
		passed = false
	} else if !passed {
		// Contradictory PASS text is not completion evidence.
		evidenceStatus = verify.StatusInconclusive
	}
	if coverage == VerificationCoveragePartial {
		passed = false
		if evidenceStatus == verify.StatusPass {
			evidenceStatus = verify.StatusInconclusive
		}
	}
	verification := Verification{
		Command:     compactText(command, maxVerificationCommand),
		Passed:      passed,
		Summary:     compactText(summary, maxVerificationSummary),
		Coverage:    coverage,
		Observation: cloneObservation(observation),
		At:          time.Now().UTC(),
		trusted:     true,
	}
	if len(t.Verification) >= maxVerification {
		copy(t.Verification, t.Verification[len(t.Verification)-maxVerification+1:])
		t.Verification = t.Verification[:maxVerification-1]
	}
	t.Verification = append(t.Verification, verification)
	// Only the latest check can authorize the current task generation. Drop
	// older bindings so repeated verification cannot grow task state with
	// duplicate bounded workspace snapshots.
	for i := range t.Verification[:len(t.Verification)-1] {
		t.Verification[i].Observation = nil
	}
	if passed {
		t.VerifiedChangeSeq = t.ChangeSeq
	} else {
		t.VerifiedChangeSeq = -1
	}
	t.touch()
	t.addTrustedEvidence(Evidence{
		Kind:       EvidenceKindVerification,
		Status:     string(evidenceStatus),
		Source:     "workspace-tool",
		Origin:     EvidenceOriginVerifier,
		Summary:    summary,
		Reference:  command,
		Confidence: "high",
		ChangeSeq:  t.ChangeSeq,
	})
	for _, index := range uniqueCriterionIndices(criteria) {
		t.addEvidenceForCriterion(index, Evidence{
			Kind:       EvidenceKindVerification,
			Status:     string(evidenceStatus),
			Source:     "workspace-tool",
			Origin:     EvidenceOriginVerifier,
			Summary:    summary,
			Reference:  command,
			Confidence: "high",
			ChangeSeq:  t.ChangeSeq,
		}, true)
	}
}

// InvalidateLatestVerification converts a passing record into explicit
// inconclusive evidence when its workspace binding is stale or unavailable.
// The original observation is retained for diagnosis, but it is no longer
// represented as a passing check.
func (t *Task) InvalidateLatestVerification(reason string) bool {
	if t == nil || len(t.Verification) == 0 {
		return false
	}
	latest := &t.Verification[len(t.Verification)-1]
	if !latest.Passed {
		return false
	}
	reason = compactText(reason, maxVerificationSummary-22)
	summary := "verify INCONCLUSIVE"
	if reason != "" {
		summary += " — " + reason
	}
	latest.Passed = false
	latest.Summary = compactText(summary, maxVerificationSummary)
	latest.At = time.Now().UTC()
	t.VerifiedChangeSeq = -1
	t.touch()
	t.AddEvidence(Evidence{
		Kind:       EvidenceKindVerification,
		Status:     "INCONCLUSIVE",
		Source:     "workspace-observation",
		Origin:     EvidenceOriginVerifier,
		Summary:    latest.Summary,
		Reference:  "workspace.Observation",
		Confidence: "high",
		ChangeSeq:  t.ChangeSeq,
	})
	// A stale latest check cannot leave an older criterion PASS able to prove
	// completion. Invalidation is deliberately conservative when the old
	// verification record did not carry a criterion list.
	for _, index := range t.RequiredCriterionIndices() {
		t.addEvidenceForCriterion(index, Evidence{
			Kind:       EvidenceKindVerification,
			Status:     "INCONCLUSIVE",
			Source:     "workspace-observation",
			Origin:     EvidenceOriginVerifier,
			Summary:    latest.Summary,
			Reference:  "workspace.Observation",
			Confidence: "high",
			ChangeSeq:  t.ChangeSeq,
		}, true)
	}
	return true
}

// InvalidateWorkspaceEvidence clears passing evidence that was bound to the
// current workspace generation. It is used after an external restoration,
// such as /undo, or after a durable outcome contract change, where the task's
// durable change sequence remains useful for history but existing proof no
// longer supports the current completion boundary.
func (t *Task) InvalidateWorkspaceEvidence(reason string) bool {
	return t.invalidateCompletionEvidence(reason)
}

func (t *Task) invalidateCompletionEvidence(reason string) bool {
	if t == nil {
		return false
	}
	reason = compactText(reason, maxVerificationSummary-22)
	summary := "verify INCONCLUSIVE"
	if reason != "" {
		summary += " — " + reason
	}
	changed := false
	if len(t.Verification) > 0 {
		latest := &t.Verification[len(t.Verification)-1]
		if latest.Passed {
			latest.Passed = false
			latest.Summary = compactText(summary, maxVerificationSummary)
			latest.At = time.Now().UTC()
			changed = true
			t.AddEvidence(Evidence{
				Kind:       EvidenceKindVerification,
				Status:     "INCONCLUSIVE",
				Source:     "workspace-observation",
				Origin:     EvidenceOriginVerifier,
				Summary:    latest.Summary,
				Reference:  "workspace restoration",
				Confidence: "high",
				ChangeSeq:  t.ChangeSeq,
			})
		}
	}
	if t.VerifiedChangeSeq >= 0 {
		t.VerifiedChangeSeq = -1
		changed = true
	}
	for index := range t.criteriaDefinition() {
		status, current := t.CriterionEvidenceState(index)
		if !current || status != "PASS" {
			continue
		}
		t.addEvidenceForCriterion(index, Evidence{
			Kind:       EvidenceKindVerification,
			Status:     "INCONCLUSIVE",
			Source:     "workspace-observation",
			Origin:     EvidenceOriginVerifier,
			Summary:    summary,
			Reference:  "workspace restoration",
			Confidence: "high",
			ChangeSeq:  t.ChangeSeq,
		}, true)
		changed = true
	}
	for _, kind := range t.RequiredEvidenceKinds() {
		status, current, _ := t.RequirementEvidenceState(kind)
		if !current || !evidenceStatusPasses(status) {
			continue
		}
		t.AddEvidence(Evidence{
			Kind:       kind,
			Status:     "INCONCLUSIVE",
			Source:     "workspace-observation",
			Origin:     EvidenceOriginSystem,
			Summary:    summary,
			Reference:  "workspace restoration",
			Confidence: "high",
			ChangeSeq:  t.ChangeSeq,
		})
		changed = true
	}
	if changed {
		t.touch()
	}
	return changed
}

// AddEvidence appends one bounded advisory evidence record. Repeating the
// exact latest record is ignored so retry loops do not inflate durable context.
// A caller-supplied Origin is retained for audit display but is never enough
// to establish completion proof.
func (t *Task) AddEvidence(e Evidence) {
	t.addEvidence(e, false)
}

// addTrustedEvidence is used only by typed runtime producers in this package.
// The trust bit is deliberately not serialized; loaded evidence must be
// re-established by a live producer rather than trusted from JSON labels.
func (t *Task) addTrustedEvidence(e Evidence) {
	t.addEvidence(e, true)
}

func (t *Task) addEvidence(e Evidence, trusted bool) {
	if t == nil {
		return
	}
	e.trusted = trusted
	e.Kind = normalizeEvidenceKind(EvidenceKind(compactText(string(e.Kind), maxEvidenceKind)))
	e.Status = compactText(e.Status, maxEvidenceStatus)
	e.Source = compactText(e.Source, maxEvidenceSource)
	e.Origin = EvidenceOrigin(compactText(string(e.Origin), maxEvidenceOrigin))
	e.Summary = compactText(e.Summary, maxEvidenceSummary)
	e.Reference = compactText(e.Reference, maxEvidenceReference)
	e.Confidence = compactText(e.Confidence, maxEvidenceConfidence)
	if e.Kind == "" || e.Status == "" || e.Summary == "" {
		return
	}
	if e.ChangeSeq < 0 {
		e.ChangeSeq = 0
	}
	if e.ChangeSeq > t.ChangeSeq {
		e.ChangeSeq = t.ChangeSeq
	}
	if e.CriterionIndex != nil {
		index := *e.CriterionIndex
		if index < 0 || index >= len(t.criteriaDefinition()) {
			return
		}
		e.CriterionIndex = &index
	}
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	if len(t.Evidence) > 0 && sameEvidence(t.Evidence[len(t.Evidence)-1], e) {
		return
	}
	if len(t.Evidence) >= maxEvidence {
		copy(t.Evidence, t.Evidence[len(t.Evidence)-maxEvidence+1:])
		t.Evidence = t.Evidence[:maxEvidence-1]
	}
	t.Evidence = append(t.Evidence, e)
	t.touch()
}

// AddRequirementEvidence records advisory evidence for one inferred quality
// requirement. The origin is retained as a provenance claim for diagnosis,
// but this compatibility API cannot establish proof. Use one of the typed
// runtime producer methods below when a concrete producer has actually run.
func (t *Task) AddRequirementEvidence(kind EvidenceKind, status string, origin EvidenceOrigin, summary, reference string) {
	if t == nil {
		return
	}
	t.AddEvidence(Evidence{
		Kind:       kind,
		Status:     status,
		Source:     string(origin),
		Origin:     origin,
		Summary:    summary,
		Reference:  reference,
		Confidence: "high",
		ChangeSeq:  t.ChangeSeq,
	})
}

func (t *Task) addTrustedRequirementEvidence(kind EvidenceKind, origin EvidenceOrigin, status, summary, reference string) {
	if t == nil {
		return
	}
	t.addTrustedEvidence(Evidence{
		Kind:       kind,
		Status:     status,
		Source:     string(origin),
		Origin:     origin,
		Summary:    summary,
		Reference:  reference,
		Confidence: "high",
		ChangeSeq:  t.ChangeSeq,
	})
}

// RecordResearchEvidence records proof emitted by the supported research
// producer. Repository/model narration should use AddRequirementEvidence and
// remains advisory.
func (t *Task) RecordResearchEvidence(status, summary, reference string) {
	t.addTrustedRequirementEvidence(EvidenceKindResearch, EvidenceOriginResearchTool, status, summary, reference)
}

// RecordMeasurementEvidence records proof emitted by the supported measurement
// or benchmark producer.
func (t *Task) RecordMeasurementEvidence(status, summary, reference string) {
	t.addTrustedRequirementEvidence(EvidenceKindMeasurement, EvidenceOriginMeasurementTool, status, summary, reference)
}

// RecordVisualEvidence records proof emitted by the supported visual
// inspection producer.
func (t *Task) RecordVisualEvidence(status, summary, reference string) {
	t.addTrustedRequirementEvidence(EvidenceKindVisual, EvidenceOriginVisualInspection, status, summary, reference)
}

// RecordBrowserEvidence records proof emitted by a live, typed browser
// screenshot producer. Browser evidence is distinct from a local visual review
// so the durable ledger preserves which runtime supplied the observation.
func (t *Task) RecordBrowserEvidence(status, summary, reference string) {
	t.addTrustedRequirementEvidence(EvidenceKindVisual, EvidenceOriginBrowser, status, summary, reference)
}

// RecordTestsEvidence records proof emitted by the supported test runner.
func (t *Task) RecordTestsEvidence(status, summary, reference string) {
	t.addTrustedRequirementEvidence(EvidenceKindTests, EvidenceOriginTestRunner, status, summary, reference)
}

// RecordApprovalEvidence records an explicit user approval. It is intentionally
// separate from generic model/tool evidence.
func (t *Task) RecordApprovalEvidence(status, summary, reference string) {
	t.addTrustedRequirementEvidence(EvidenceKindApproval, EvidenceOriginUserApproval, status, summary, reference)
}

// RecordCriterionVerification records criterion proof emitted by the live
// verifier without accepting a caller-supplied origin label.
func (t *Task) RecordCriterionVerification(index int, status, summary, reference string) {
	if t == nil {
		return
	}
	t.addEvidenceForCriterion(index, Evidence{
		Kind:       EvidenceKindVerification,
		Status:     status,
		Source:     "workspace-tool",
		Origin:     EvidenceOriginVerifier,
		Summary:    summary,
		Reference:  reference,
		Confidence: "high",
		ChangeSeq:  t.ChangeSeq,
	}, true)
}

// RecordCriterionTestsEvidence records criterion proof emitted by the live
// test runner without accepting a caller-supplied origin label.
func (t *Task) RecordCriterionTestsEvidence(index int, status, summary, reference string) {
	if t == nil {
		return
	}
	t.addEvidenceForCriterion(index, Evidence{
		Kind:       EvidenceKindTests,
		Status:     status,
		Source:     "test-runner",
		Origin:     EvidenceOriginTestRunner,
		Summary:    summary,
		Reference:  reference,
		Confidence: "high",
		ChangeSeq:  t.ChangeSeq,
	}, true)
}

func sameEvidence(left, right Evidence) bool {
	leftCriterion, rightCriterion := -1, -1
	if left.CriterionIndex != nil {
		leftCriterion = *left.CriterionIndex
	}
	if right.CriterionIndex != nil {
		rightCriterion = *right.CriterionIndex
	}
	left.At = time.Time{}
	right.At = time.Time{}
	left.CriterionIndex = nil
	right.CriterionIndex = nil
	return left == right && leftCriterion == rightCriterion
}

// RequiredEvidenceKinds returns the proof kinds implied by the durable intent
// in stable order. A missing intent has no inferred quality requirements, which
// preserves the legacy criterion/verification contract.
func (t *Task) RequiredEvidenceKinds() []EvidenceKind {
	if t == nil || t.Intent == nil {
		return nil
	}
	var kinds []EvidenceKind
	if t.Intent.NeedsResearch {
		kinds = append(kinds, EvidenceKindResearch)
	}
	if t.Intent.NeedsMeasurement || t.Intent.Class == "performance" {
		kinds = append(kinds, EvidenceKindMeasurement)
	}
	if t.Intent.NeedsVisual {
		kinds = append(kinds, EvidenceKindVisual)
	}
	if t.Intent.NeedsTests {
		kinds = append(kinds, EvidenceKindTests)
	}
	if t.Intent.NeedsApproval {
		kinds = append(kinds, EvidenceKindApproval)
	}
	return kinds
}

// RequirementEvidenceState returns the latest current status from a trusted
// origin for one required proof kind. A mismatched kind, stale change sequence,
// or untrusted origin is deliberately reported as not current.
func (t *Task) RequirementEvidenceState(kind EvidenceKind) (string, bool, EvidenceOrigin) {
	if t == nil {
		return "UNVERIFIED", false, ""
	}
	kind = normalizeEvidenceKind(kind)
	if !isRequirementKind(kind) {
		return "UNVERIFIED", false, ""
	}
	for i := len(t.Evidence) - 1; i >= 0; i-- {
		evidence := t.Evidence[i]
		if !evidenceMatchesRequirement(evidence.Kind, kind) {
			continue
		}
		status := normalizeEvidenceStatus(evidence.Status)
		if evidence.ChangeSeq != t.ChangeSeq || !evidence.trusted || !evidence.Origin.TrustedFor(kind) {
			return status, false, evidence.Origin
		}
		return status, true, evidence.Origin
	}
	return "UNVERIFIED", false, ""
}

// CompletionCheck evaluates all durable proof boundaries without mutating the
// task. It is intentionally explainable: callers can show or route on exactly
// which criteria and inferred requirements remain unproven.
func (t *Task) CompletionCheck() CompletionCheck {
	check := CompletionCheck{}
	if t == nil {
		check.Reason = "no durable task"
		return check
	}
	if t.Status == StatusBlocked {
		check.Reason = "durable task is blocked"
		return check
	}

	for _, index := range t.RequiredCriterionIndices() {
		status, current := t.CriterionEvidenceState(index)
		if !current || status != "PASS" {
			check.MissingCriteria = append(check.MissingCriteria, index)
		}
	}
	for _, kind := range t.RequiredEvidenceKinds() {
		status, current, origin := t.RequirementEvidenceState(kind)
		check.Requirements = append(check.Requirements, RequirementEvidenceState{
			Kind:    kind,
			Status:  status,
			Origin:  origin,
			Current: current,
		})
		if !current || !evidenceStatusPasses(status) {
			check.MissingRequirements = append(check.MissingRequirements, kind)
		}
	}
	check.ChangedFilesCapped = t.ChangedFilesCapped
	if t.ChangedFilesCapped || t.latestVerificationHasPartialCoverage() {
		check.VerificationRequired = true
		check.Reason = "workspace verification coverage is incomplete"
	} else if len(check.MissingCriteria) > 0 {
		check.Reason = "required criterion evidence is incomplete"
	} else if len(check.MissingRequirements) > 0 {
		check.Reason = "required quality evidence is incomplete"
	}

	criteria := t.criteriaDefinition()
	if (len(criteria) == 0 || len(t.RequiredCriterionIndices()) == 0) && len(t.RequiredEvidenceKinds()) == 0 {
		if len(check.MissingRequirements) == 0 {
			check.VerificationRequired = true
			check.VerificationCurrent = t.workspaceBoundVerificationReady()
			if !check.VerificationCurrent && check.Reason == "" {
				check.Reason = "current workspace-bound verification is required"
			}
		}
	}
	check.Ready = !check.ChangedFilesCapped && len(check.MissingCriteria) == 0 && len(check.MissingRequirements) == 0 && (!check.VerificationRequired || check.VerificationCurrent || (len(criteria) > 0 && len(t.RequiredCriterionIndices()) > 0))
	if check.Ready {
		check.Reason = "all required criteria and quality evidence are current"
	}
	return check
}

func normalizeEvidenceKind(kind EvidenceKind) EvidenceKind {
	switch strings.ToLower(strings.TrimSpace(string(kind))) {
	case string(EvidenceKindVerification):
		return EvidenceKindVerification
	case string(EvidenceKindResearch):
		return EvidenceKindResearch
	case string(EvidenceKindMeasurement), "measure", "benchmark":
		return EvidenceKindMeasurement
	case string(EvidenceKindVisual), "visual_inspection":
		return EvidenceKindVisual
	case string(EvidenceKindTests), string(EvidenceKindTest), "test_runner":
		return EvidenceKindTests
	case string(EvidenceKindApproval), "approved":
		return EvidenceKindApproval
	case string(EvidenceKindInspection):
		return EvidenceKindInspection
	default:
		return EvidenceKind(strings.ToLower(strings.TrimSpace(string(kind))))
	}
}

func normalizeEvidenceStatus(status string) string {
	return strings.ToUpper(strings.TrimSpace(status))
}

func evidenceStatusPasses(status string) bool {
	switch normalizeEvidenceStatus(status) {
	case "PASS", "APPROVED", "CONFIRMED":
		return true
	default:
		return false
	}
}

func isRequirementKind(kind EvidenceKind) bool {
	switch normalizeEvidenceKind(kind) {
	case EvidenceKindResearch, EvidenceKindMeasurement, EvidenceKindVisual, EvidenceKindTests, EvidenceKindApproval:
		return true
	default:
		return false
	}
}

func evidenceMatchesRequirement(actual, required EvidenceKind) bool {
	actual = normalizeEvidenceKind(actual)
	required = normalizeEvidenceKind(required)
	if required == EvidenceKindTests {
		return actual == EvidenceKindTests || actual == EvidenceKindVerification
	}
	return actual == required
}

// AddEvidenceForCriterion binds one advisory evidence record to a bounded
// criterion. Invalid indexes are ignored so untrusted model/tool data cannot
// create a durable reference outside the task definition. A caller-supplied
// verifier origin is not a runtime proof capability.
func (t *Task) AddEvidenceForCriterion(index int, evidence Evidence) {
	t.addEvidenceForCriterion(index, evidence, false)
}

func (t *Task) addEvidenceForCriterion(index int, evidence Evidence, trusted bool) {
	if t == nil || index < 0 || index >= len(t.criteriaDefinition()) {
		return
	}
	evidence.CriterionIndex = &index
	t.addEvidence(evidence, trusted)
}

// RequiredCriterionIndices returns the required definition-of-done indexes in
// stable order. Legacy step-only tasks treat every step as required.
func (t *Task) RequiredCriterionIndices() []int {
	if t == nil {
		return nil
	}
	criteria := t.criteriaDefinition()
	indices := make([]int, 0, len(criteria))
	for index, criterion := range criteria {
		if criterion.Required {
			indices = append(indices, index)
		}
	}
	return indices
}

// FirstMissingRequiredCriterion returns the first required criterion whose
// latest criterion-bound evidence is not a current PASS.
func (t *Task) FirstMissingRequiredCriterion() int {
	if t == nil {
		return -1
	}
	for _, index := range t.RequiredCriterionIndices() {
		if status, current := t.CriterionEvidenceState(index); !current || status != "PASS" {
			return index
		}
	}
	return -1
}

// CriterionEvidenceState returns the latest bounded evidence status for one
// criterion and whether it is current trusted proof for the task's ChangeSeq.
func (t *Task) CriterionEvidenceState(index int) (string, bool) {
	if t == nil || index < 0 || index >= len(t.criteriaDefinition()) {
		return "UNVERIFIED", false
	}
	for i := len(t.Evidence) - 1; i >= 0; i-- {
		evidence := t.Evidence[i]
		if evidence.CriterionIndex == nil || *evidence.CriterionIndex != index {
			continue
		}
		status := strings.ToUpper(strings.TrimSpace(evidence.Status))
		// Criterion proof is a completion boundary, not merely a status
		// annotation. Keep arbitrary/model-origin records visible in the
		// ledger, but never let them satisfy a required criterion. The
		// verifier records criterion proof as verification/tests evidence
		// from an allowlisted producer; other kinds are advisory only.
		kind := normalizeEvidenceKind(evidence.Kind)
		trusted := evidence.trusted && (kind == EvidenceKindVerification || kind == EvidenceKindTests) && evidence.Origin.TrustedFor(kind)
		if evidence.ChangeSeq != t.ChangeSeq || !trusted {
			// A non-passing record is useful failure/inconclusive context even when
			// its producer cannot be re-established after reload. Only positive
			// claims are hidden behind the runtime trust bit.
			if !evidenceStatusPasses(status) {
				return status, false
			}
			return "UNVERIFIED", false
		}
		// The second result reports freshness, not success. A current FAIL or
		// INCONCLUSIVE record must remain visible to callers instead of being
		// collapsed into the same state as missing evidence.
		return status, true
	}
	return "UNVERIFIED", false
}

// CompletionReady is the compatibility boolean for the durable completion
// check. Callers that need to explain a refusal should use CompletionCheck.
func (t *Task) CompletionReady() bool {
	return t != nil && t.CompletionCheck().Ready
}

// ReestablishWorkspaceVerification restores runtime trust after a persisted
// verification has been compared with a fresh live workspace observation.
// Serialized provenance is never trusted by itself; the caller must provide
// the observation it just captured, and this method requires it to match the
// stored complete proof boundary before re-enabling the associated evidence.
func (t *Task) ReestablishWorkspaceVerification(observation *workspace.Observation) bool {
	if t == nil || observation == nil || len(t.Verification) == 0 {
		return false
	}
	latest := &t.Verification[len(t.Verification)-1]
	if !latest.Passed || latest.Observation == nil || latest.Observation.FilesTruncated || t.VerifiedChangeSeq != t.ChangeSeq {
		return false
	}
	if normalizeVerificationCoverage(latest.Coverage, latest.Observation) != VerificationCoverageComplete {
		return false
	}
	if comparison := workspace.Compare(*latest.Observation, *observation); !comparison.Fresh {
		return false
	}

	changed := !latest.trusted
	latest.trusted = true
	for i := range t.Evidence {
		evidence := &t.Evidence[i]
		if evidence.trusted || evidence.ChangeSeq != t.ChangeSeq || !evidenceStatusPasses(evidence.Status) {
			continue
		}
		if normalizeEvidenceKind(evidence.Kind) != EvidenceKindVerification || evidence.Origin != EvidenceOriginVerifier {
			continue
		}
		if evidence.Summary != latest.Summary || evidence.Reference != latest.Command || evidence.At.Before(latest.At) {
			continue
		}
		evidence.trusted = true
		changed = true
	}
	if t.normalizedFromDone && t.Status == StatusWorking && t.allStepsComplete() && t.CompletionReady() {
		t.Status = StatusDone
		t.BlockedBy = ""
		t.StopReason = StopNone
		changed = true
	}
	if changed {
		t.touch()
	}
	return changed
}

func (t *Task) allStepsComplete() bool {
	if t == nil || len(t.Steps) == 0 {
		return true
	}
	if t.CurrentStep < len(t.Steps) {
		return false
	}
	for _, step := range t.Steps {
		if !step.Done {
			return false
		}
	}
	return true
}

func (t *Task) workspaceBoundVerificationReady() bool {
	if t == nil || len(t.Verification) == 0 || t.NeedsVerification() {
		return false
	}
	latest := t.Verification[len(t.Verification)-1]
	if normalizeVerificationCoverage(latest.Coverage, latest.Observation) != VerificationCoverageComplete {
		return false
	}
	return latest.trusted && latest.Passed && latest.Observation != nil && !latest.Observation.FilesTruncated && t.VerifiedChangeSeq == t.ChangeSeq
}

func (t *Task) latestVerificationHasPartialCoverage() bool {
	if t == nil || len(t.Verification) == 0 {
		return false
	}
	latest := t.Verification[len(t.Verification)-1]
	return normalizeVerificationCoverage(latest.Coverage, latest.Observation) == VerificationCoveragePartial
}

func verificationCoverageForObservation(observation *workspace.Observation) string {
	if observation == nil {
		return VerificationCoverageUnbound
	}
	if observation.FilesTruncated {
		return VerificationCoveragePartial
	}
	return VerificationCoverageComplete
}

func normalizeVerificationCoverage(coverage string, observation *workspace.Observation) string {
	switch strings.ToLower(strings.TrimSpace(coverage)) {
	case "":
		return verificationCoverageForObservation(observation)
	case VerificationCoveragePartial:
		return VerificationCoveragePartial
	case VerificationCoverageUnbound:
		return VerificationCoverageUnbound
	case VerificationCoverageComplete:
		return VerificationCoverageComplete
	default:
		return VerificationCoverageUnbound
	}
}

func (t *Task) criteriaDefinition() []Criterion {
	if t == nil {
		return nil
	}
	if len(t.DefinitionOfDone) > 0 {
		return t.DefinitionOfDone
	}
	if len(t.Steps) == 0 {
		return nil
	}
	criteria := make([]Criterion, 0, len(t.Steps))
	for _, step := range t.Steps {
		criteria = append(criteria, Criterion{Description: step.Description, Required: true})
	}
	return criteria
}

func uniqueCriterionIndices(indices []int) []int {
	if len(indices) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(indices))
	out := make([]int, 0, len(indices))
	for _, index := range indices {
		if index < 0 || index >= maxTaskSteps {
			continue
		}
		if _, ok := seen[index]; ok {
			continue
		}
		seen[index] = struct{}{}
		out = append(out, index)
	}
	return out
}

func cloneObservation(observation *workspace.Observation) *workspace.Observation {
	if observation == nil {
		return nil
	}
	clone := *observation
	clone.Files = append([]workspace.FileObservation(nil), observation.Files...)
	return &clone
}

// AddConstraint records a compact user or project boundary.
func (t *Task) AddConstraint(note string) {
	if t != nil {
		addOutcomeNote(&t.Constraints, note)
	}
}

// AddRisk records a compact risk discovered during work.
func (t *Task) AddRisk(note string) {
	if t != nil {
		addOutcomeNote(&t.Risks, note)
	}
}

// AddUncertainty records an unresolved fact that should not be presented as
// confirmed completion evidence.
func (t *Task) AddUncertainty(note string) {
	if t != nil {
		addOutcomeNote(&t.Uncertainty, note)
	}
}

func addOutcomeNote(dst *[]string, note string) {
	if dst == nil {
		return
	}
	note = compactText(note, maxOutcomeNote)
	if note == "" {
		return
	}
	for _, existing := range *dst {
		if existing == note {
			return
		}
	}
	if len(*dst) >= maxOutcomeNotes {
		copy(*dst, (*dst)[1:])
		*dst = (*dst)[:maxOutcomeNotes-1]
	}
	*dst = append(*dst, note)
}

// NeedsVerification reports whether the most recent successful mutation has
// not been covered by a passing check. Older task files did not carry a change
// sequence, so changed paths in that shape remain conservatively unverified.
func (t *Task) NeedsVerification() bool {
	if t == nil {
		return false
	}
	if len(t.ChangedFiles) > 0 && t.ChangeSeq == 0 {
		return true
	}
	if len(t.Verification) > 0 && (!t.Verification[len(t.Verification)-1].Passed || !t.Verification[len(t.Verification)-1].trusted) {
		return true
	}
	return t.VerifiedChangeSeq != t.ChangeSeq
}

// ConsecutiveVerificationFailures counts failures since the latest passing check.
func (t *Task) ConsecutiveVerificationFailures() int {
	if t == nil {
		return 0
	}
	n := 0
	for i := len(t.Verification) - 1; i >= 0; i-- {
		if t.Verification[i].Passed {
			break
		}
		n++
	}
	return n
}

// Block records why autonomous progress stopped.
func (t *Task) Block(reason string) {
	if t == nil || t.Status == StatusDone {
		return
	}
	t.Status = StatusBlocked
	t.BlockedBy = compactText(reason, maxBlockedBy)
	t.StopReason = stopReasonFor(t.BlockedBy)
	t.touch()
}

func stopReasonFor(reason string) StopReason {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case "task budget exhausted":
		return StopBudgetExhausted
	case "permission needed":
		return StopPermissionNeeded
	case "verification repeatedly failed":
		return StopVerificationFailures
	case "resource unavailable", "tool or resource unavailable":
		return StopResourceUnavailable
	case "user choice required", "user choice genuinely required":
		return StopUserChoiceRequired
	default:
		return StopNone
	}
}

func (t *Task) touch() {
	t.UpdatedAt = time.Now().UTC()
}

func randomID() (string, error) {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func compactText(s string, limit int) string {
	s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	if len(s) > limit {
		s = s[:limit]
	}
	return s
}
