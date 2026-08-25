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
)

const CurrentVersion = 1

const (
	maxTaskSteps    = 8
	maxChangedFiles = 128
	maxVerification = 32
	maxOutcomeNotes = 8
	maxEvidence     = 16
	maxTaskAttempts = 128
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
	Command string    `json:"command,omitempty"`
	Passed  bool      `json:"passed"`
	Summary string    `json:"summary,omitempty"`
	At      time.Time `json:"at"`
}

// Evidence is a compact, source-labelled fact used to reason about an
// outcome. Raw command output stays outside durable state; Summary is a
// bounded distillation and Reference points at the useful source.
type Evidence struct {
	Kind       string    `json:"kind"`
	Status     string    `json:"status"`
	Source     string    `json:"source,omitempty"`
	Summary    string    `json:"summary"`
	Reference  string    `json:"reference,omitempty"`
	Confidence string    `json:"confidence,omitempty"`
	ChangeSeq  int       `json:"change_seq,omitempty"`
	At         time.Time `json:"at"`
}

// IntentContract is the compact, internal interpretation of a user request.
// It keeps vague intent and its risk/proof implications durable without
// exposing a planning mode or requiring the user to name agent concepts.
type IntentContract struct {
	Outcome       string `json:"outcome"`
	Class         string `json:"class,omitempty"`
	Action        string `json:"action,omitempty"`
	Completeness  string `json:"completeness,omitempty"`
	Scope         string `json:"scope,omitempty"`
	Risk          string `json:"risk,omitempty"`
	NeedsResearch bool   `json:"needs_research,omitempty"`
	NeedsVisual   bool   `json:"needs_visual,omitempty"`
	NeedsTests    bool   `json:"needs_tests,omitempty"`
	NeedsApproval bool   `json:"needs_approval,omitempty"`
	Confidence    string `json:"confidence,omitempty"`
}

// Criterion is one compact, internal definition-of-done item. Evidence is
// recorded separately in Verification so raw model narration does not become
// durable task state.
type Criterion struct {
	Description string `json:"description"`
	Required    bool   `json:"required,omitempty"`
}

// Task is the compact state required to resume an execution loop.
type Task struct {
	Version            int             `json:"version"`
	ID                 string          `json:"id"`
	SessionID          string          `json:"session_id"`
	Goal               string          `json:"goal"`
	Intent             *IntentContract `json:"intent,omitempty"`
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
	BlockedBy         string         `json:"blocked_by,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

// New creates a task associated with a persisted chat session.
func New(sessionID, goal string, steps []string) (*Task, error) {
	goal = compactText(goal, 600)
	if strings.TrimSpace(sessionID) == "" {
		return nil, errors.New("task session id is required")
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
		if step = compactText(step, 300); step != "" {
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
	if len(t.Goal) > 600 {
		return errors.New("task goal is too long")
	}
	if len(t.ID) > 200 || len(t.SessionID) > 200 {
		return errors.New("task identity is too long")
	}
	if t.Intent != nil {
		if strings.TrimSpace(t.Intent.Outcome) == "" {
			return errors.New("task intent outcome is required")
		}
		if len(t.Intent.Outcome) > 600 {
			return errors.New("task intent outcome is too long")
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
		if strings.TrimSpace(path) == "" || len(path) > 500 {
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
			if strings.TrimSpace(note) == "" || len(note) > 500 {
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
		if len(verification.Command) > 300 || len(verification.Summary) > 800 {
			return fmt.Errorf("task verification %d is too long", i)
		}
	}
	for i, evidence := range t.Evidence {
		if strings.TrimSpace(evidence.Kind) == "" || strings.TrimSpace(evidence.Status) == "" {
			return fmt.Errorf("task evidence %d is missing kind or status", i)
		}
		if strings.TrimSpace(evidence.Summary) == "" || len(evidence.Summary) > 800 {
			return fmt.Errorf("task evidence %d summary is empty or too long", i)
		}
		if evidence.ChangeSeq < 0 || evidence.ChangeSeq > t.ChangeSeq {
			return fmt.Errorf("task evidence %d change sequence %d is invalid for change sequence %d", i, evidence.ChangeSeq, t.ChangeSeq)
		}
	}
	for i, criterion := range t.DefinitionOfDone {
		if strings.TrimSpace(criterion.Description) == "" || len(criterion.Description) > 300 {
			return fmt.Errorf("task completion criterion %d is empty", i)
		}
	}
	if !t.Status.Valid() {
		return fmt.Errorf("invalid task status %q", t.Status)
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
	for i, step := range t.Steps {
		if strings.TrimSpace(step.Description) == "" || len(step.Description) > 300 {
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
	if t.Status == StatusPlanning && next == StatusVerifying {
		return errors.New("task must start working before verification")
	}
	t.Status = next
	if next != StatusBlocked {
		t.BlockedBy = ""
	}
	t.touch()
	return nil
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
	path = strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	path = strings.TrimPrefix(path, "./")
	if path == "" {
		return
	}
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
	if t == nil {
		return
	}
	verification := Verification{
		Command: compactText(command, 300),
		Passed:  passed,
		Summary: compactText(summary, 800),
		At:      time.Now().UTC(),
	}
	if len(t.Verification) >= maxVerification {
		copy(t.Verification, t.Verification[len(t.Verification)-maxVerification+1:])
		t.Verification = t.Verification[:maxVerification-1]
	}
	t.Verification = append(t.Verification, verification)
	if passed {
		t.VerifiedChangeSeq = t.ChangeSeq
	} else {
		t.VerifiedChangeSeq = -1
	}
	t.touch()
	t.AddEvidence(Evidence{
		Kind:       "verification",
		Status:     map[bool]string{true: "PASS", false: "FAIL"}[passed],
		Source:     "workspace-tool",
		Summary:    summary,
		Reference:  command,
		Confidence: "high",
		ChangeSeq:  t.ChangeSeq,
	})
}

// AddEvidence appends one bounded evidence record. Repeating the exact latest
// record is ignored so retry loops do not inflate durable context.
func (t *Task) AddEvidence(e Evidence) {
	if t == nil {
		return
	}
	e.Kind = compactText(e.Kind, 48)
	e.Status = compactText(e.Status, 32)
	e.Source = compactText(e.Source, 64)
	e.Summary = compactText(e.Summary, 800)
	e.Reference = compactText(e.Reference, 300)
	e.Confidence = compactText(e.Confidence, 24)
	if e.Kind == "" || e.Status == "" || e.Summary == "" {
		return
	}
	if e.ChangeSeq < 0 {
		e.ChangeSeq = 0
	}
	if e.ChangeSeq > t.ChangeSeq {
		e.ChangeSeq = t.ChangeSeq
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

func sameEvidence(left, right Evidence) bool {
	left.At = time.Time{}
	right.At = time.Time{}
	return left == right
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
	note = compactText(note, 500)
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
	if len(t.Verification) > 0 && !t.Verification[len(t.Verification)-1].Passed {
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
	t.BlockedBy = compactText(reason, 500)
	t.touch()
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
