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

var ErrNotFound = errors.New("task state not found")

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

// Save atomically persists task state with private file permissions.
func (s *Store) Save(task *Task) error {
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
	task.touch()
	tmp, err := os.CreateTemp(s.dir, ".task-*.tmp")
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
	if err := os.Rename(tmpName, path); err != nil {
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
