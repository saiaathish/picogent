package taskstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	ErrNotFound         = errors.New("task state not found")
	ErrRevisionConflict = errors.New("task state revision conflict")
)

// Store persists one task per chat session. Files stay separate from chat history.
type Store struct {
	dir string
	mu  sync.Mutex
}

// NewStore creates a store rooted at dir.
func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

// WorkspaceStore stores task state inside the workspace-local Picogent folder.
func WorkspaceStore(workspace string) *Store {
	return NewStore(filepath.Join(workspace, ".picogent", "tasks"))
}

// Path returns the task file for sessionID.
func (s *Store) Path(sessionID string) (string, error) {
	if s == nil || strings.TrimSpace(s.dir) == "" {
		return "", errors.New("task store directory is required")
	}
	if !safeSessionID(sessionID) {
		return "", errors.New("invalid task session id")
	}
	return filepath.Join(s.dir, sessionID+".json"), nil
}

// Save atomically persists task state with private file permissions. The
// task's current Revision is the expected on-disk generation; a successful
// save advances it by one. This makes ordinary Save calls compare-and-swap
// operations rather than last-writer-wins updates.
func (s *Store) Save(task *Task) error {
	if task == nil {
		return errors.New("task is nil")
	}
	return s.SaveIfRevision(task, task.Revision)
}

// SaveIfRevision atomically persists task state only when expected matches
// both the caller's task generation and the current on-disk generation. A
// conflict leaves the caller's task unchanged and returns ErrRevisionConflict.
func (s *Store) SaveIfRevision(task *Task, expected uint64) error {
	if task == nil {
		return errors.New("task is nil")
	}
	if task.Revision != expected {
		return fmt.Errorf("%w: task revision %d does not match expected %d", ErrRevisionConflict, task.Revision, expected)
	}
	if err := task.Validate(); err != nil {
		return err
	}
	path, err := s.Path(task.SessionID)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create task store: %w", err)
	}
	unlock, err := acquireTaskStoreLock(s.dir)
	if err != nil {
		return fmt.Errorf("lock task store: %w", err)
	}
	defer unlock()

	current, err := loadTaskFile(path, task.SessionID)
	if errors.Is(err, ErrNotFound) {
		if expected != 0 {
			return revisionConflict(expected, 0)
		}
	} else if err != nil {
		return fmt.Errorf("read current task state: %w", err)
	} else if current.Revision != expected {
		return revisionConflict(expected, current.Revision)
	}
	if expected == ^uint64(0) {
		return errors.New("task state revision exhausted")
	}
	candidate := *task
	candidate.Revision = expected + 1
	candidate.touch()
	if err := candidate.Validate(); err != nil {
		return err
	}
	if err := saveTaskFile(path, &candidate); err != nil {
		return err
	}
	*task = candidate
	return nil
}

func revisionConflict(expected, actual uint64) error {
	return fmt.Errorf("%w: expected %d, found %d", ErrRevisionConflict, expected, actual)
}

func saveTaskFile(path string, task *Task) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".task-*.tmp")
	if err != nil {
		return fmt.Errorf("create task temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("protect task temp file: %w", err)
	}
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(task); err != nil {
		tmp.Close()
		return fmt.Errorf("encode task state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync task state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close task state: %w", err)
	}
	if err := replaceFile(tmpName, path); err != nil {
		return fmt.Errorf("replace task state: %w", err)
	}
	return nil
}

// Load restores task state associated with sessionID.
func (s *Store) Load(sessionID string) (*Task, error) {
	path, err := s.Path(sessionID)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return nil, fmt.Errorf("create task store: %w", err)
	}
	unlock, err := acquireTaskStoreLock(s.dir)
	if err != nil {
		return nil, fmt.Errorf("lock task store: %w", err)
	}
	defer unlock()
	return loadTaskFile(path, sessionID)
}

func loadTaskFile(path, sessionID string) (*Task, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("open task state: %w", err)
	}
	defer f.Close()
	dec := json.NewDecoder(io.LimitReader(f, 1<<20))
	dec.DisallowUnknownFields()
	var task Task
	if err := dec.Decode(&task); err != nil {
		return nil, fmt.Errorf("decode task state: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return nil, fmt.Errorf("decode task state: %w", err)
	}
	if task.SessionID != sessionID {
		return nil, errors.New("task state session mismatch")
	}
	if err := task.Validate(); err != nil {
		return nil, fmt.Errorf("validate task state: %w", err)
	}
	return &task, nil
}

// Delete removes task state for sessionID. Missing state is already deleted.
func (s *Store) Delete(sessionID string) error {
	path, err := s.Path(sessionID)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create task store: %w", err)
	}
	unlock, err := acquireTaskStoreLock(s.dir)
	if err != nil {
		return fmt.Errorf("lock task store: %w", err)
	}
	defer unlock()
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete task state: %w", err)
	}
	return nil
}

// InferAndSave creates durable state only for task-like prompts.
func (s *Store) InferAndSave(sessionID, prompt string) (*Task, bool, error) {
	task, ok, err := NewFromPrompt(sessionID, prompt)
	if err != nil || !ok {
		return task, ok, err
	}
	if err := s.Save(task); err != nil {
		return nil, true, err
	}
	return task, true, nil
}

func safeSessionID(id string) bool {
	if id == "" || id == "." || id == ".." || len(id) > 200 {
		return false
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}
