package taskstate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/saiaathish/picogent/internal/securefile"
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

// AcquireRunLock serializes one complete agent run for this project. The lock
// is blocking so a second process waits for the active run to finish instead
// of interleaving workspace mutations. The kernel-backed file lock also
// releases automatically if the owning process exits.
func (s *Store) AcquireRunLock() (func() error, error) {
	dir, err := s.runLockDir()
	if err != nil {
		return nil, err
	}
	key, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve task run lock directory: %w", err)
	}
	entry := acquireRunProcessLock(key)
	releaseProcess := true
	defer func() {
		if releaseProcess {
			releaseRunProcessLock(key, entry)
		}
	}()
	if err := securefile.EnsureDir(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create task run lock directory: %w", err)
	}
	file, err := securefile.OpenLockFile(filepath.Join(dir, ".run.lock"))
	if err != nil {
		return nil, fmt.Errorf("open task run lock: %w", err)
	}
	unlock, err := securefile.LockFile(file, true)
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock task run: %w", err)
	}
	releaseProcess = false
	var once sync.Once
	var releaseErr error
	return func() error {
		once.Do(func() {
			releaseErr = errors.Join(unlock(), file.Close())
			releaseRunProcessLock(key, entry)
		})
		return releaseErr
	}, nil
}

func (s *Store) runLockDir() (string, error) {
	if s == nil || strings.TrimSpace(s.dir) == "" {
		return "", errors.New("task store directory is required")
	}
	return filepath.Clean(s.dir), nil
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
	path, err := s.Path(task.SessionID)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := securefile.EnsureDir(s.dir, 0o700); err != nil {
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
	// Never write a legacy terminal marker that lacks the authoritative v4
	// completion predicate. Keep the caller's value unchanged until the CAS
	// write succeeds, just as with any other failed save.
	candidate.NormalizeLegacyCompletion()
	if err := candidate.Validate(); err != nil {
		return err
	}
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
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return fmt.Errorf("encode task state: %w", err)
	}
	data = append(data, '\n')
	if err := securefile.WriteAtomic(path, data, 0o600); err != nil {
		return fmt.Errorf("write task state: %w", err)
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
	if err := securefile.EnsureDir(s.dir, 0o700); err != nil {
		return nil, fmt.Errorf("create task store: %w", err)
	}
	unlock, err := acquireTaskStoreLock(s.dir)
	if err != nil {
		return nil, fmt.Errorf("lock task store: %w", err)
	}
	defer unlock()
	task, err := loadTaskFile(path, sessionID)
	if err != nil {
		return nil, err
	}
	if !task.NormalizeLegacyCompletion() {
		return task, nil
	}
	if task.Revision == ^uint64(0) {
		return nil, errors.New("task state revision exhausted while normalizing legacy completion")
	}
	task.Revision++
	if err := task.Validate(); err != nil {
		return nil, fmt.Errorf("validate normalized task state: %w", err)
	}
	if err := saveTaskFile(path, task); err != nil {
		return nil, fmt.Errorf("persist normalized task state: %w", err)
	}
	return task, nil
}

func loadTaskFile(path, sessionID string) (*Task, error) {
	data, err := securefile.ReadFileLimited(path, 1<<20)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read task state: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
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
	if err := securefile.EnsureDir(s.dir, 0o700); err != nil {
		return fmt.Errorf("create task store: %w", err)
	}
	unlock, err := acquireTaskStoreLock(s.dir)
	if err != nil {
		return fmt.Errorf("lock task store: %w", err)
	}
	defer unlock()
	if err := securefile.RemoveFile(path); err != nil && !errors.Is(err, os.ErrNotExist) {
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
