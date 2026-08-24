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

// Task is the compact state required to resume an execution loop.
type Task struct {
	Version      int      `json:"version"`
	ID           string   `json:"id"`
	SessionID    string   `json:"session_id"`
	Goal         string   `json:"goal"`
	Status       Status   `json:"status"`
	Steps        []Step   `json:"steps,omitempty"`
	CurrentStep  int      `json:"current_step"`
	Attempts     int      `json:"attempts"`
	ChangedFiles []string `json:"changed_files,omitempty"`
	ChangeSeq    int      `json:"change_seq,omitempty"`
	// VerifiedChangeSeq is the latest change sequence covered by passing
	// verification. A negative value records that the latest evidence did not
	// pass.
	VerifiedChangeSeq int            `json:"verified_change_seq,omitempty"`
	Verification      []Verification `json:"verification,omitempty"`
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
	if !t.Status.Valid() {
		return fmt.Errorf("invalid task status %q", t.Status)
	}
	if t.CurrentStep < 0 || t.CurrentStep > len(t.Steps) {
		return fmt.Errorf("current step %d out of range", t.CurrentStep)
	}
	if t.Attempts < 0 {
		return errors.New("task attempts cannot be negative")
	}
	if t.ChangeSeq < 0 {
		return errors.New("task change sequence cannot be negative")
	}
	if t.VerifiedChangeSeq < -1 || t.VerifiedChangeSeq > t.ChangeSeq {
		return fmt.Errorf("task verified change sequence %d is invalid for change sequence %d", t.VerifiedChangeSeq, t.ChangeSeq)
	}
	for i, step := range t.Steps {
		if strings.TrimSpace(step.Description) == "" {
			return fmt.Errorf("task step %d is empty", i)
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
	t.Verification = append(t.Verification, Verification{
		Command: compactText(command, 300),
		Passed:  passed,
		Summary: compactText(summary, 800),
		At:      time.Now().UTC(),
	})
	if passed {
		t.VerifiedChangeSeq = t.ChangeSeq
	} else {
		t.VerifiedChangeSeq = -1
	}
	t.touch()
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
