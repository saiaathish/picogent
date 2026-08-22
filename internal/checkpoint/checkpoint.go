// Package checkpoint captures and restores the files changed by one agent turn.
package checkpoint

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

var (
	ErrNotSealed       = errors.New("checkpoint is not sealed")
	ErrAlreadySealed   = errors.New("checkpoint is already sealed")
	ErrAlreadyRestored = errors.New("checkpoint is already restored")
	ErrConflict        = errors.New("checkpoint restore conflicts with newer changes")
)

// Conflict identifies a path changed after the checkpoint was sealed.
type Conflict struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// Failure records a restore or rollback operation that could not be completed.
type Failure struct {
	Path         string `json:"path"`
	Operation    string `json:"operation"`
	Message      string `json:"message"`
	RecoveryPath string `json:"recovery_path,omitempty"`
}

// RestoreResult describes every effect of a Restore call.
type RestoreResult struct {
	Restored   []string   `json:"restored,omitempty"`
	Removed    []string   `json:"removed,omitempty"`
	Unchanged  []string   `json:"unchanged,omitempty"`
	Conflicts  []Conflict `json:"conflicts,omitempty"`
	Failures   []Failure  `json:"failures,omitempty"`
	Complete   bool       `json:"complete"`
	RolledBack bool       `json:"rolled_back,omitempty"`
}

// Checkpoint holds the pre-turn state for an explicit set of workspace files.
// Call Seal after the turn's edits and before offering Restore to the user.
type Checkpoint struct {
	mu        sync.Mutex
	rootInput string
	root      string
	entries   []entry
	sealed    bool
	restored  bool
}

type entry struct {
	path     string
	before   fileState
	expected fingerprint
}

type fileState struct {
	exists bool
	mode   fs.FileMode
	data   []byte
	sum    fingerprint
}

type fingerprint [sha256.Size]byte

// Capture snapshots only paths. Paths may be workspace-relative or absolute
// paths inside workspace. Directories and symlinks are rejected.
func Capture(workspace string, paths []string) (*Checkpoint, error) {
	rootInput, root, err := resolveWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, errors.New("checkpoint requires at least one path")
	}

	cp := &Checkpoint{rootInput: rootInput, root: root}
	if err := cp.add(paths); err != nil {
		return nil, err
	}
	return cp, nil
}

// Add snapshots additional paths before they are changed. Existing paths are
// deduplicated by normalized workspace-relative name and retain their original
// pre-turn snapshot. Paths cannot be added after Seal.
func (c *Checkpoint) Add(paths []string) error {
	if c == nil {
		return errors.New("checkpoint is nil")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sealed {
		return ErrAlreadySealed
	}
	return c.add(paths)
}

func (c *Checkpoint) add(paths []string) error {
	if len(paths) == 0 {
		return errors.New("checkpoint requires at least one path")
	}
	seen := make(map[string]struct{}, len(c.entries)+len(paths))
	for i := range c.entries {
		seen[c.entries[i].path] = struct{}{}
	}
	entries := make([]entry, 0, len(paths))
	for _, requested := range paths {
		rel, err := normalizePath(c.rootInput, c.root, requested)
		if err != nil {
			return fmt.Errorf("checkpoint path %q: %w", requested, err)
		}
		if _, ok := seen[rel]; ok {
			continue
		}
		seen[rel] = struct{}{}
		state, err := readWorkspaceFile(c.root, rel)
		if err != nil {
			return fmt.Errorf("checkpoint path %q: %w", requested, err)
		}
		entries = append(entries, entry{path: rel, before: state})
	}
	c.entries = append(c.entries, entries...)
	return nil
}

// Paths returns the normalized workspace-relative paths in this checkpoint.
func (c *Checkpoint) Paths() []string {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.entries))
	for i := range c.entries {
		out[i] = filepath.ToSlash(c.entries[i].path)
	}
	return out
}

// Seal fingerprints the files produced by the turn. Restore later refuses to
// replace any path whose bytes, existence, or mode no longer matches this seal.
func (c *Checkpoint) Seal() error {
	if c == nil {
		return ErrNotSealed
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sealed {
		return ErrAlreadySealed
	}

	states := make([]fileState, len(c.entries))
	for i := range c.entries {
		state, err := readWorkspaceFile(c.root, c.entries[i].path)
		if err != nil {
			return fmt.Errorf("seal %q: %w", filepath.ToSlash(c.entries[i].path), err)
		}
		states[i] = state
	}
	for i := range c.entries {
		c.entries[i].expected = states[i].sum
	}
	c.sealed = true
	return nil
}

// ChangedPaths returns paths whose sealed state differs from their captured
// pre-turn state. It is valid only after Seal.
func (c *Checkpoint) ChangedPaths() ([]string, error) {
	if c == nil {
		return nil, ErrNotSealed
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.sealed {
		return nil, ErrNotSealed
	}
	paths := make([]string, 0, len(c.entries))
	for i := range c.entries {
		if c.entries[i].before.sum != c.entries[i].expected {
			paths = append(paths, filepath.ToSlash(c.entries[i].path))
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// Restore puts every checkpointed path back to its pre-turn state. It first
// checks all fingerprints, so a normal conflict changes nothing. File installs
// are staged beside their destination and published without overwriting an
// existing name. If a later operation fails, completed operations are rolled
// back on a best-effort basis and the result says whether rollback succeeded.
func (c *Checkpoint) Restore() (RestoreResult, error) {
	var result RestoreResult
	if c == nil {
		return result, ErrNotSealed
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.sealed {
		return result, ErrNotSealed
	}
	if c.restored {
		return result, ErrAlreadyRestored
	}

	mutations := make([]mutation, 0, len(c.entries))
	for i := range c.entries {
		current, err := readWorkspaceFile(c.root, c.entries[i].path)
		if err != nil {
			result.Failures = append(result.Failures, failure(c.entries[i].path, "inspect", err))
			continue
		}
		if current.sum != c.entries[i].expected {
			result.Conflicts = append(result.Conflicts, Conflict{
				Path:   filepath.ToSlash(c.entries[i].path),
				Reason: "file changed after checkpoint was sealed",
			})
			continue
		}
		if current.sum == c.entries[i].before.sum {
			result.Unchanged = append(result.Unchanged, filepath.ToSlash(c.entries[i].path))
			continue
		}
		mutations = append(mutations, mutation{entry: &c.entries[i]})
	}
	if len(result.Conflicts) > 0 {
		sortConflicts(result.Conflicts)
		return result, ErrConflict
	}
	if len(result.Failures) > 0 {
		return result, errors.New("checkpoint restore preflight failed")
	}
	if len(mutations) == 0 {
		result.Complete = true
		c.restored = true
		sort.Strings(result.Unchanged)
		return result, nil
	}

	if err := stageMutations(c.root, mutations); err != nil {
		cleanupStages(mutations)
		result.Failures = append(result.Failures, failure(err.path, err.operation, err.err))
		return result, fmt.Errorf("checkpoint restore staging failed: %w", err.err)
	}

	for i := range mutations {
		if opErr := applyMutation(c.root, &mutations[i]); opErr != nil {
			if errors.Is(opErr.err, ErrConflict) {
				result.Conflicts = append(result.Conflicts, Conflict{
					Path: filepath.ToSlash(opErr.path), Reason: opErr.err.Error(),
				})
			} else {
				result.Failures = append(result.Failures, failure(opErr.path, opErr.operation, opErr.err))
			}
			result.RolledBack = rollback(c.root, mutations[:i+1], &result)
			appendRecoveryPaths(mutations[:i+1], &result)
			cleanupStages(mutations)
			if len(result.Conflicts) > 0 {
				return result, ErrConflict
			}
			return result, fmt.Errorf("checkpoint restore failed: %w", opErr.err)
		}
	}

	for i := range mutations {
		path := filepath.ToSlash(mutations[i].entry.path)
		if mutations[i].entry.before.exists {
			result.Restored = append(result.Restored, path)
		} else {
			result.Removed = append(result.Removed, path)
		}
		if mutations[i].backup != "" {
			if err := os.Remove(mutations[i].backup); err != nil && !errors.Is(err, fs.ErrNotExist) {
				result.Failures = append(result.Failures, failure(mutations[i].entry.path, "cleanup", err))
			}
			mutations[i].backup = ""
		}
	}
	cleanupStages(mutations)
	sort.Strings(result.Restored)
	sort.Strings(result.Removed)
	sort.Strings(result.Unchanged)
	result.Complete = true
	c.restored = true
	if len(result.Failures) > 0 {
		return result, errors.New("checkpoint restored but temporary file cleanup failed")
	}
	return result, nil
}

type mutation struct {
	entry   *entry
	stage   string
	backup  string
	claimed bool
	applied bool
}

type operationError struct {
	path      string
	operation string
	err       error
}

func stageMutations(root string, mutations []mutation) *operationError {
	for i := range mutations {
		path, err := securePath(root, mutations[i].entry.path)
		if err != nil {
			return &operationError{mutations[i].entry.path, "validate", err}
		}
		parent := filepath.Dir(path)
		probe, err := os.CreateTemp(parent, ".picogent-undo-probe-*")
		if err != nil {
			return &operationError{mutations[i].entry.path, "stage", err}
		}
		probeName := probe.Name()
		if err := probe.Close(); err != nil {
			_ = os.Remove(probeName)
			return &operationError{mutations[i].entry.path, "stage", err}
		}
		if err := os.Remove(probeName); err != nil {
			return &operationError{mutations[i].entry.path, "stage", err}
		}

		if !mutations[i].entry.before.exists {
			continue
		}
		f, err := os.CreateTemp(parent, ".picogent-undo-stage-*")
		if err != nil {
			return &operationError{mutations[i].entry.path, "stage", err}
		}
		mutations[i].stage = f.Name()
		if _, err = f.Write(mutations[i].entry.before.data); err == nil {
			err = f.Sync()
		}
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Chmod(mutations[i].stage, mutations[i].entry.before.mode)
		}
		if err != nil {
			return &operationError{mutations[i].entry.path, "stage", err}
		}
	}
	return nil
}

func applyMutation(root string, m *mutation) *operationError {
	path, err := securePath(root, m.entry.path)
	if err != nil {
		return &operationError{m.entry.path, "validate", err}
	}
	current, err := readRegularFile(path)
	if err != nil {
		return &operationError{m.entry.path, "inspect", err}
	}
	if current.sum != m.entry.expected {
		return &operationError{m.entry.path, "conflict", ErrConflict}
	}

	if current.exists {
		backup, err := unusedTempName(filepath.Dir(path), ".picogent-undo-backup-*")
		if err != nil {
			return &operationError{m.entry.path, "backup", err}
		}
		if err := os.Rename(path, backup); err != nil {
			return &operationError{m.entry.path, "backup", err}
		}
		m.backup = backup
		m.claimed = true
		claimed, err := readRegularFile(backup)
		if err != nil || claimed.sum != m.entry.expected {
			if err == nil {
				err = ErrConflict
			}
			if restoreErr := os.Rename(backup, path); restoreErr == nil {
				m.backup = ""
				m.claimed = false
			}
			return &operationError{m.entry.path, "conflict", err}
		}
	}

	if m.entry.before.exists {
		if err := os.Link(m.stage, path); err != nil {
			return &operationError{m.entry.path, "publish", err}
		}
		m.applied = true
		if err := os.Remove(m.stage); err != nil {
			return &operationError{m.entry.path, "publish", err}
		}
		m.stage = ""
	} else {
		m.applied = true
	}
	return nil
}

func rollback(root string, mutations []mutation, result *RestoreResult) bool {
	ok := true
	for i := len(mutations) - 1; i >= 0; i-- {
		m := &mutations[i]
		path, err := securePath(root, m.entry.path)
		if err != nil {
			result.Failures = append(result.Failures, failure(m.entry.path, "rollback validate", err))
			ok = false
			continue
		}
		if m.applied && m.entry.before.exists {
			state, inspectErr := readRegularFile(path)
			if inspectErr != nil || state.sum != m.entry.before.sum {
				if inspectErr == nil {
					inspectErr = ErrConflict
				}
				result.Failures = append(result.Failures, failure(m.entry.path, "rollback conflict", inspectErr))
				ok = false
				continue
			}
			if err := os.Remove(path); err != nil {
				result.Failures = append(result.Failures, failure(m.entry.path, "rollback remove", err))
				ok = false
				continue
			}
		}
		if m.claimed && m.backup != "" {
			if _, err := os.Lstat(path); err == nil {
				result.Failures = append(result.Failures, failure(m.entry.path, "rollback conflict", ErrConflict))
				ok = false
				continue
			} else if !errors.Is(err, fs.ErrNotExist) {
				result.Failures = append(result.Failures, failure(m.entry.path, "rollback inspect", err))
				ok = false
				continue
			}
			if err := os.Rename(m.backup, path); err != nil {
				result.Failures = append(result.Failures, failure(m.entry.path, "rollback restore", err))
				ok = false
				continue
			}
			m.backup = ""
			m.claimed = false
		}
	}
	return ok
}

func cleanupStages(mutations []mutation) {
	for i := range mutations {
		if mutations[i].stage != "" {
			_ = os.Remove(mutations[i].stage)
		}
	}
}

func appendRecoveryPaths(mutations []mutation, result *RestoreResult) {
	for i := range mutations {
		if mutations[i].backup == "" {
			continue
		}
		result.Failures = append(result.Failures, Failure{
			Path:         filepath.ToSlash(mutations[i].entry.path),
			Operation:    "recovery",
			Message:      "displaced contents preserved after incomplete rollback",
			RecoveryPath: mutations[i].backup,
		})
	}
}

func resolveWorkspace(workspace string) (string, string, error) {
	if strings.TrimSpace(workspace) == "" {
		return "", "", errors.New("workspace path is empty")
	}
	input, err := filepath.Abs(workspace)
	if err != nil {
		return "", "", err
	}
	root, err := filepath.EvalSymlinks(input)
	if err != nil {
		return "", "", fmt.Errorf("resolve workspace: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", "", err
	}
	if !info.IsDir() {
		return "", "", errors.New("workspace is not a directory")
	}
	return filepath.Clean(input), filepath.Clean(root), nil
}

func normalizePath(rootInput, root, requested string) (string, error) {
	if strings.TrimSpace(requested) == "" {
		return "", errors.New("path is empty")
	}
	var rel string
	var err error
	if filepath.IsAbs(requested) {
		rel, err = filepath.Rel(rootInput, filepath.Clean(requested))
		if err != nil || escapes(rel) {
			rel, err = filepath.Rel(root, filepath.Clean(requested))
		}
	} else {
		rel = filepath.Clean(requested)
	}
	if err != nil {
		return "", err
	}
	if rel == "." || escapes(rel) || filepath.IsAbs(rel) {
		return "", errors.New("path escapes workspace or names workspace root")
	}
	if _, err := securePath(root, rel); err != nil {
		return "", err
	}
	return rel, nil
}

func escapes(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func securePath(root, rel string) (string, error) {
	if rel == "." || escapes(rel) || filepath.IsAbs(rel) {
		return "", errors.New("path escapes workspace")
	}
	path := root
	parts := strings.Split(filepath.Clean(rel), string(filepath.Separator))
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", errors.New("invalid workspace path")
		}
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return "", errors.New("symlink paths are not checkpointable")
		}
		if i < len(parts)-1 && !info.IsDir() {
			return "", errors.New("path parent is not a directory")
		}
	}
	return path, nil
}

func readWorkspaceFile(root, rel string) (fileState, error) {
	path, err := securePath(root, rel)
	if err != nil {
		return fileState{}, err
	}
	return readRegularFile(path)
}

func readRegularFile(path string) (fileState, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		state := fileState{}
		state.sum = fingerprintFor(state)
		return state, nil
	}
	if err != nil {
		return fileState{}, err
	}
	if !info.Mode().IsRegular() {
		return fileState{}, errors.New("checkpoint path is not a regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fileState{}, err
	}
	state := fileState{exists: true, mode: restorableMode(info.Mode()), data: data}
	state.sum = fingerprintFor(state)
	return state, nil
}

func restorableMode(mode fs.FileMode) fs.FileMode {
	return mode.Perm() | mode&(fs.ModeSetuid|fs.ModeSetgid|fs.ModeSticky)
}

func fingerprintFor(state fileState) fingerprint {
	h := sha256.New()
	if state.exists {
		_, _ = h.Write([]byte{1})
	} else {
		_, _ = h.Write([]byte{0})
	}
	var mode [4]byte
	binary.LittleEndian.PutUint32(mode[:], uint32(state.mode))
	_, _ = h.Write(mode[:])
	_, _ = h.Write(state.data)
	var sum fingerprint
	copy(sum[:], h.Sum(nil))
	return sum
}

func unusedTempName(dir, pattern string) (string, error) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	if err := os.Remove(name); err != nil {
		return "", err
	}
	return name, nil
}

func failure(path, operation string, err error) Failure {
	return Failure{Path: filepath.ToSlash(path), Operation: operation, Message: err.Error()}
}

func sortConflicts(conflicts []Conflict) {
	sort.Slice(conflicts, func(i, j int) bool { return conflicts[i].Path < conflicts[j].Path })
}
