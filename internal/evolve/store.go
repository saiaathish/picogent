// Package evolve is Picogent's lightweight self-evolution loop.
//
// Inspired by Hermes Agent's skill + memory + curator ideas, but sized for a
// pocket agent: no DSPy/GEPA stack, no command to "be my assistant" — Picogent
// quietly remembers habits and short playbooks from successful turns and
// injects them into the next system prompt.
package evolve

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/projects"
)

const (
	maxHabits    = 5
	maxPlaybooks = 4
	maxFailures  = 6
	maxRoutes    = 6
	maxHabitLen  = 100
	maxBodyLen   = 320
	staleDays    = 45
)

// Habit is a short durable preference (Hermes-style memory fact).
type Habit struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	Source    string    `json:"source,omitempty"` // turn | reflect | heuristic
	Hits      int       `json:"hits"`
	Pinned    bool      `json:"pinned,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Playbook is a short procedural note (Hermes-style agent-created skill, tiny).
type Playbook struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Class     string    `json:"class,omitempty"` // task class for class-first updates
	Source    string    `json:"source,omitempty"`
	Hits      int       `json:"hits"`
	Pinned    bool      `json:"pinned,omitempty"`
	Archived  bool      `json:"archived,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FailureMemory keeps one compact cause/effect relationship from a failed or
// unavailable verification. It is a hypothesis for the next turn, not an
// authority that can bypass current evidence or permissions.
type FailureMemory struct {
	ID           string     `json:"id"`
	Class        string     `json:"class,omitempty"`
	Trigger      string     `json:"trigger"`
	Consequence  string     `json:"consequence"`
	Evidence     string     `json:"evidence,omitempty"`
	Confidence   string     `json:"confidence,omitempty"`
	Hits         int        `json:"hits"`
	Failures     int        `json:"failures"`
	Resolutions  int        `json:"resolutions,omitempty"`
	LastSeen     time.Time  `json:"last_seen"`
	LastResolved *time.Time `json:"last_resolved,omitempty"`
}

// VerificationRoute is a learned, bounded relationship between a task class
// and paths covered by a passing verification. It informs context and target
// selection only; commands are still built by verify at runtime.
type VerificationRoute struct {
	ID       string    `json:"id"`
	Class    string    `json:"class,omitempty"`
	Targets  []string  `json:"targets,omitempty"`
	Stages   []string  `json:"stages,omitempty"`
	Hits     int       `json:"hits"`
	Passes   int       `json:"passes"`
	LastUsed time.Time `json:"last_used"`
}

// Store is per-workspace learned memory.
type Store struct {
	Workspace          string              `json:"workspace"`
	Habits             []Habit             `json:"habits"`
	Playbooks          []Playbook          `json:"playbooks"`
	Failures           []FailureMemory     `json:"failures,omitempty"`
	VerificationRoutes []VerificationRoute `json:"verification_routes,omitempty"`
	UpdatedAt          time.Time           `json:"updated_at"`
}

func readPath(workspace string) (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "evolve", projects.IDForPath(workspace)+".json"), nil
}

func storePath(workspace string) (string, error) {
	path, err := readPath(workspace)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return path, nil
}

func missingStatePathError(path string) error {
	dir := filepath.Dir(path)
	for {
		info, err := os.Stat(dir)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("evolve state parent %s is not a directory", dir)
			}
			return nil
		}
		if !os.IsNotExist(err) {
			return err
		}
		next := filepath.Dir(dir)
		if next == dir {
			return nil
		}
		dir = next
	}
}

func loadLocked(path, workspace string) (Store, error) {
	s := Store{Workspace: workspace}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return Store{}, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return Store{}, err
	}
	if s.Workspace == "" {
		s.Workspace = workspace
	}
	return s, nil
}

func Load(workspace string) (Store, error) {
	path, err := readPath(workspace)
	if err != nil {
		return Store{}, err
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if parentErr := missingStatePathError(path); parentErr != nil {
				return Store{}, parentErr
			}
			return Store{Workspace: workspace}, nil
		}
		return Store{}, err
	}
	unlock, err := acquireStoreLock(path)
	if err != nil {
		return Store{}, err
	}
	defer unlock()
	return loadLocked(path, workspace)
}

func saveLocked(path string, s Store) (Store, error) {
	s = Curate(s)
	s.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return Store{}, err
	}
	if err := writeAtomic(path, data); err != nil {
		return Store{}, err
	}
	return s, nil
}

// Save atomically persists a store. Callers that derive a new store from a
// prior Load should prefer Update so the read and write happen under one
// cross-process lock.
func Save(s Store) error {
	path, err := storePath(s.Workspace)
	if err != nil {
		return err
	}
	unlock, err := acquireStoreLock(path)
	if err != nil {
		return err
	}
	defer unlock()
	_, err = saveLocked(path, s)
	return err
}

// Update loads, mutates, curates, and atomically persists one workspace store
// while holding the same lock for the entire transaction. It prevents a
// concurrent reflection or verification-memory update from losing another
// update between Load and Save.
func Update(workspace string, fn func(Store) (Store, error)) (Store, error) {
	if fn == nil {
		return Store{}, errors.New("evolve update callback is required")
	}
	path, err := storePath(workspace)
	if err != nil {
		return Store{}, err
	}
	unlock, err := acquireStoreLock(path)
	if err != nil {
		return Store{}, err
	}
	defer unlock()

	current, err := loadLocked(path, workspace)
	if err != nil {
		return Store{}, err
	}
	next, err := fn(current)
	if err != nil {
		return Store{}, err
	}
	// The workspace argument selects the path and is authoritative. A
	// callback cannot redirect a transaction to a different workspace.
	next.Workspace = workspace
	return saveLocked(path, next)
}

func idFor(parts ...string) string {
	h := sha1.Sum([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(h[:8])
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func normTitle(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.Join(strings.Fields(s), " ")
	return s
}
