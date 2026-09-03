// Package checkpoint captures and restores the files changed by one agent turn.
package checkpoint

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/saiaathish/picogent/internal/workspace"
)

var (
	ErrNotSealed       = errors.New("checkpoint is not sealed")
	ErrAlreadySealed   = errors.New("checkpoint is already sealed")
	ErrAlreadyRestored = errors.New("checkpoint is already restored")
	ErrConflict        = errors.New("checkpoint restore conflicts with newer changes")
)

const (
	// RecordVersion is the serialized checkpoint format understood by this
	// package. Unsupported versions fail closed during import.
	RecordVersion = 1
	// MaxRecordEntries bounds one durable undo record to the same practical
	// path budget used by workspace observations.
	MaxRecordEntries = 128
	// MaxRecordFileBytes bounds the pre-turn bytes retained for one path.
	MaxRecordFileBytes = 1 << 20
	// MaxRecordBytes bounds the total pre-turn payload before JSON/base64
	// serialization expands it in the journal.
	MaxRecordBytes = 8 << 20
)

// Record is the portable, validated form of a sealed checkpoint. It omits the
// workspace root so an importing process must explicitly bind it to the
// current workspace before any restore can occur.
type Record struct {
	Version int           `json:"version"`
	Entries []RecordEntry `json:"entries"`
}

// RecordEntry contains the pre-turn state and the post-seal fingerprint for
// one workspace-relative regular file. Published is only used by a pending
// record when a later same-path write was prepared but not yet published.
type RecordEntry struct {
	Path         string `json:"path"`
	BeforeExists bool   `json:"before_exists"`
	BeforeMode   uint32 `json:"before_mode,omitempty"`
	BeforeData   []byte `json:"before_data,omitempty"`
	Expected     string `json:"expected"`
	Published    string `json:"published,omitempty"`
}

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

	// restoreBeforeApply is only used by package tests to exercise an
	// interleaving between restore preflight and publication.
	restoreBeforeApply func(string)
}

type entry struct {
	path         string
	before       fileState
	expected     fingerprint
	expectedSet  bool
	published    fingerprint
	publishedSet bool
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

// Drop removes a captured path that is known not to belong to this turn's
// undo set. It is used when a native edit detects a content conflict before
// publication: the current bytes are a newer workspace edit, not an agent
// mutation that undo may safely replace. Paths may not be dropped after the
// checkpoint is sealed.
func (c *Checkpoint) Drop(path string) error {
	if c == nil {
		return errors.New("checkpoint is nil")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sealed {
		return ErrAlreadySealed
	}
	rel, err := normalizePath(c.rootInput, c.root, path)
	if err != nil {
		return fmt.Errorf("checkpoint path %q: %w", path, err)
	}
	for i := range c.entries {
		if pathIdentity(c.entries[i].path) != pathIdentity(rel) {
			continue
		}
		copy(c.entries[i:], c.entries[i+1:])
		c.entries = c.entries[:len(c.entries)-1]
		return nil
	}
	return fmt.Errorf("checkpoint path %q was not captured", path)
}

func (c *Checkpoint) add(paths []string) error {
	if len(paths) == 0 {
		return errors.New("checkpoint requires at least one path")
	}
	seen := make(map[string]struct{}, len(c.entries)+len(paths))
	for i := range c.entries {
		seen[pathIdentity(c.entries[i].path)] = struct{}{}
	}
	entries := make([]entry, 0, len(paths))
	for _, requested := range paths {
		rel, err := normalizePath(c.rootInput, c.root, requested)
		if err != nil {
			return fmt.Errorf("checkpoint path %q: %w", requested, err)
		}
		key := pathIdentity(rel)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
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
		c.entries[i].expectedSet = true
		c.entries[i].published = fingerprint{}
		c.entries[i].publishedSet = false
	}
	c.sealed = true
	return nil
}

// PrepareExpected records the exact regular-file state that an imminent
// atomic write will publish. It is used by durable undo to publish a pending
// recovery record before the workspace rename. The checkpoint remains
// unsealed so later tool writes can update their own expected state. Before
// replacing an earlier expectation, it records whether that expectation was
// actually published or whether the workspace is still at the pre-turn state.
func (c *Checkpoint) PrepareExpected(path string, data []byte, mode fs.FileMode) (bool, error) {
	if c == nil {
		return false, ErrNotSealed
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sealed {
		return false, ErrAlreadySealed
	}
	rel, err := normalizePath(c.rootInput, c.root, path)
	if err != nil {
		return false, fmt.Errorf("checkpoint path %q: %w", path, err)
	}
	for i := range c.entries {
		if pathIdentity(c.entries[i].path) != pathIdentity(rel) {
			continue
		}
		current, err := readWorkspaceFile(c.root, rel)
		if err != nil {
			return false, fmt.Errorf("inspect checkpoint path %q: %w", path, err)
		}
		published := c.entries[i].before.sum
		if current.sum != c.entries[i].before.sum {
			if !c.entries[i].expectedSet || current.sum != c.entries[i].expected {
				return false, fmt.Errorf("%w: checkpoint path %q changed before publication", ErrConflict, path)
			}
			published = current.sum
		}
		state := fileState{exists: true, mode: restorableMode(mode), data: append([]byte(nil), data...)}
		state.sum = fingerprintFor(state)
		c.entries[i].published = published
		c.entries[i].publishedSet = true
		c.entries[i].expected = state.sum
		c.entries[i].expectedSet = true
		return c.entries[i].before.sum != state.sum, nil
	}
	return false, fmt.Errorf("checkpoint path %q was not captured", path)
}

// Export returns the changed, expected states prepared on this checkpoint.
// It accepts a sealed checkpoint or a pending checkpoint with at least one
// prepared write. Unprepared paths are omitted so a crash between two writes
// can recover only the paths that were actually published or being published.
func (c *Checkpoint) Export() (Record, error) {
	if c == nil {
		return Record{}, ErrNotSealed
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.sealed {
		prepared := false
		for _, item := range c.entries {
			prepared = prepared || item.expectedSet
		}
		if !prepared {
			return Record{}, ErrNotSealed
		}
	}
	return exportEntries(c.entries)
}

func exportEntries(entries []entry) (Record, error) {
	record := Record{Version: RecordVersion}
	if len(entries) > MaxRecordEntries {
		return Record{}, fmt.Errorf("checkpoint has too many entries: %d", len(entries))
	}
	bytes := 0
	for _, item := range entries {
		if !item.expectedSet || (item.before.sum == item.expected && (!item.publishedSet || item.published == item.before.sum)) {
			continue
		}
		if len(item.before.data) > MaxRecordFileBytes {
			return Record{}, fmt.Errorf("checkpoint file %q exceeds the %d-byte durable undo limit", filepath.ToSlash(item.path), MaxRecordFileBytes)
		}
		bytes += len(item.before.data)
		if bytes > MaxRecordBytes {
			return Record{}, fmt.Errorf("checkpoint exceeds the %d-byte durable undo limit", MaxRecordBytes)
		}
		published := ""
		if item.publishedSet && item.published != item.before.sum {
			published = hex.EncodeToString(item.published[:])
		}
		record.Entries = append(record.Entries, RecordEntry{
			Path:         filepath.ToSlash(item.path),
			BeforeExists: item.before.exists,
			BeforeMode:   uint32(item.before.mode),
			BeforeData:   append([]byte(nil), item.before.data...),
			Expected:     hex.EncodeToString(item.expected[:]),
			Published:    published,
		})
	}
	return record, nil
}

// Import binds a validated serialized record to a current workspace. No
// workspace file is changed during import; Restore performs the later
// fingerprint checks before publishing any pre-turn state.
func Import(workspace string, record Record) (*Checkpoint, error) {
	if record.Version != RecordVersion {
		return nil, fmt.Errorf("unsupported checkpoint record version %d", record.Version)
	}
	if len(record.Entries) == 0 {
		return nil, errors.New("checkpoint record has no entries")
	}
	if len(record.Entries) > MaxRecordEntries {
		return nil, fmt.Errorf("checkpoint record has too many entries: %d", len(record.Entries))
	}
	rootInput, root, err := resolveWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	entries := make([]entry, 0, len(record.Entries))
	seen := make(map[string]struct{}, len(record.Entries))
	bytes := 0
	for _, item := range record.Entries {
		if filepath.IsAbs(item.Path) {
			return nil, fmt.Errorf("checkpoint record path %q must be relative", item.Path)
		}
		rel, err := normalizePath(rootInput, root, item.Path)
		if err != nil {
			return nil, fmt.Errorf("checkpoint record path %q: %w", item.Path, err)
		}
		if _, ok := seen[pathIdentity(rel)]; ok {
			return nil, fmt.Errorf("checkpoint record repeats path %q", item.Path)
		}
		seen[pathIdentity(rel)] = struct{}{}
		if len(item.BeforeData) > MaxRecordFileBytes {
			return nil, fmt.Errorf("checkpoint record file %q exceeds the %d-byte durable undo limit", item.Path, MaxRecordFileBytes)
		}
		bytes += len(item.BeforeData)
		if bytes > MaxRecordBytes {
			return nil, fmt.Errorf("checkpoint record exceeds the %d-byte durable undo limit", MaxRecordBytes)
		}
		if !item.BeforeExists && (len(item.BeforeData) != 0 || item.BeforeMode != 0) {
			return nil, fmt.Errorf("checkpoint record absent path %q has file state", item.Path)
		}
		expectedBytes, err := hex.DecodeString(item.Expected)
		if err != nil || len(expectedBytes) != sha256.Size {
			return nil, fmt.Errorf("checkpoint record path %q has invalid expected fingerprint", item.Path)
		}
		var expected fingerprint
		copy(expected[:], expectedBytes)
		before := fileState{exists: item.BeforeExists, mode: restorableMode(fs.FileMode(item.BeforeMode)), data: append([]byte(nil), item.BeforeData...)}
		before.sum = fingerprintFor(before)
		published := before.sum
		publishedSet := false
		if item.Published != "" {
			publishedBytes, publishedErr := hex.DecodeString(item.Published)
			if publishedErr != nil || len(publishedBytes) != sha256.Size {
				return nil, fmt.Errorf("checkpoint record path %q has invalid published fingerprint", item.Path)
			}
			copy(published[:], publishedBytes)
			publishedSet = true
		}
		entries = append(entries, entry{path: rel, before: before, expected: expected, expectedSet: true, published: published, publishedSet: publishedSet})
	}
	return &Checkpoint{rootInput: rootInput, root: root, entries: entries, sealed: true}, nil
}

// PublishedSubset returns a checkpoint containing only entries whose current
// workspace state matches the expected post-write fingerprint. Entries still
// at their pre-turn state are omitted, which lets a fresh process recover a
// crash before the final rename of a later path. For repeated writes to one
// path, a pending record also carries the last known published fingerprint;
// that state is accepted and becomes the subset's expected state. Any other
// state is a conflict and returns no subset so recovery remains fail closed.
func (c *Checkpoint) PublishedSubset() (*Checkpoint, bool, error) {
	if c == nil {
		return nil, false, ErrNotSealed
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.sealed {
		return nil, false, ErrNotSealed
	}
	entries := make([]entry, 0, len(c.entries))
	for i := range c.entries {
		current, err := readWorkspaceFile(c.root, c.entries[i].path)
		if err != nil {
			return nil, false, err
		}
		if current.sum == c.entries[i].expected {
			item := c.entries[i]
			item.published = fingerprint{}
			item.publishedSet = false
			entries = append(entries, item)
			continue
		}
		if current.sum == c.entries[i].before.sum {
			continue
		}
		if c.entries[i].publishedSet && current.sum == c.entries[i].published {
			item := c.entries[i]
			item.expected = item.published
			item.expectedSet = true
			item.published = fingerprint{}
			item.publishedSet = false
			entries = append(entries, item)
			continue
		}
		return nil, false, ErrConflict
	}
	if len(entries) == 0 {
		return nil, false, nil
	}
	return &Checkpoint{rootInput: c.rootInput, root: c.root, entries: entries, sealed: true}, true, nil
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
// checks all fingerprints, so a normal conflict changes nothing. Each write
// and removal is performed through the secure workspace primitive; if a later
// operation fails, the in-memory post-turn states are replayed on a best-effort
// basis and the result says whether rollback succeeded.
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
		// A process can die after publishing a restore for this path but before
		// the durable journal advances from sealed to restored. Treat the exact
		// pre-turn state as already complete so a fresh process can resume the
		// remaining paths instead of misclassifying the restored path as a newer
		// conflicting edit.
		if current.sum == c.entries[i].before.sum {
			result.Unchanged = append(result.Unchanged, filepath.ToSlash(c.entries[i].path))
			continue
		}
		if current.sum != c.entries[i].expected {
			result.Conflicts = append(result.Conflicts, Conflict{
				Path:   filepath.ToSlash(c.entries[i].path),
				Reason: "file changed after checkpoint was sealed",
			})
			continue
		}
		mutations = append(mutations, mutation{entry: &c.entries[i], after: current})
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

	for i := range mutations {
		if opErr := applyMutation(c.root, &mutations[i], c.restoreBeforeApply); opErr != nil {
			if errors.Is(opErr.err, ErrConflict) {
				result.Conflicts = append(result.Conflicts, Conflict{
					Path: filepath.ToSlash(opErr.path), Reason: opErr.err.Error(),
				})
			} else {
				result.Failures = append(result.Failures, failure(opErr.path, opErr.operation, opErr.err))
			}
			result.RolledBack = rollback(c.root, mutations[:i+1], &result)
			if len(result.Conflicts) > 0 {
				return result, ErrConflict
			}
			return result, fmt.Errorf("checkpoint restore failed: %w", opErr.err)
		}
	}

	for i := range mutations {
		path := filepath.ToSlash(mutations[i].entry.path)
		if mutations[i].alreadyRestored {
			result.Unchanged = append(result.Unchanged, path)
		} else if mutations[i].entry.before.exists {
			result.Restored = append(result.Restored, path)
		} else {
			result.Removed = append(result.Removed, path)
		}
	}
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
	entry           *entry
	after           fileState
	applied         bool
	alreadyRestored bool
}

type operationError struct {
	path      string
	operation string
	err       error
}

func applyMutation(root string, m *mutation, beforeWrite func(string)) *operationError {
	// Restore preflight already captured the post-turn state in m.after. The
	// workspace compare-and-publish primitive below performs the required final
	// content, mode, and path-identity check immediately before publication;
	// avoid reading the same file a second time in this layer. If that final
	// check reports a content conflict, one follow-up read distinguishes an
	// already-completed restore from a newer state; arbitrary conflicts still
	// fail closed.
	if beforeWrite != nil {
		beforeWrite(filepath.ToSlash(m.entry.path))
	}
	if err := writeWorkspaceState(root, m.entry.path, m.after, m.entry.before); err != nil {
		if errors.Is(err, workspace.ErrContentConflict) {
			if current, inspectErr := readWorkspaceFile(root, m.entry.path); inspectErr == nil && current.sum == m.entry.before.sum {
				m.alreadyRestored = true
				return nil
			}
			return &operationError{m.entry.path, "conflict", ErrConflict}
		}
		operation := "remove"
		if m.entry.before.exists {
			operation = "write"
		}
		return &operationError{m.entry.path, operation, err}
	}
	m.applied = true
	return nil
}

func rollback(root string, mutations []mutation, result *RestoreResult) bool {
	ok := true
	attempted := false
	for i := len(mutations) - 1; i >= 0; i-- {
		m := &mutations[i]
		if !m.applied {
			continue
		}
		attempted = true
		current, err := readWorkspaceFile(root, m.entry.path)
		if err != nil {
			result.Failures = append(result.Failures, failure(m.entry.path, "rollback inspect", err))
			ok = false
			continue
		}
		if current.sum != m.entry.before.sum {
			result.Failures = append(result.Failures, failure(m.entry.path, "rollback conflict", ErrConflict))
			ok = false
			continue
		}
		if err := writeWorkspaceState(root, m.entry.path, m.entry.before, m.after); err != nil {
			result.Failures = append(result.Failures, failure(m.entry.path, "rollback restore", err))
			ok = false
		}
	}
	return attempted && ok
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

func pathIdentity(rel string) string {
	if runtime.GOOS == "windows" {
		return strings.ToLower(rel)
	}
	return rel
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
	f, err := workspace.OpenRead(root, rel)
	if errors.Is(err, fs.ErrNotExist) {
		state := fileState{}
		state.sum = fingerprintFor(state)
		return state, nil
	}
	if err != nil {
		return fileState{}, err
	}
	defer f.Close()
	return readRegularFileHandle(f)
}

func writeWorkspaceState(root, rel string, expected, state fileState) error {
	if !state.exists {
		if expected.exists {
			return workspace.RemoveIfUnchanged(root, rel, expected.data, expected.mode)
		}
		err := workspace.Remove(root, rel)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if expected.exists {
		return workspace.WriteAtomicIfUnchangedWithMode(root, rel, expected.data, expected.mode, state.data, state.mode)
	}
	return workspace.WriteAtomicIfMissingWithMode(root, rel, state.data, state.mode)
}

func readRegularFileHandle(f *os.File) (fileState, error) {
	info, err := f.Stat()
	if err != nil {
		return fileState{}, err
	}
	if !info.Mode().IsRegular() {
		return fileState{}, errors.New("checkpoint path is not a regular file")
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return fileState{}, err
	}
	state := fileState{exists: true, mode: restorableMode(info.Mode()), data: data}
	state.sum = fingerprintFor(state)
	return state, nil
}

func restorableMode(mode fs.FileMode) fs.FileMode {
	if runtime.GOOS == "windows" {
		// Windows exposes Unix permission bits only as a writable/read-only
		// approximation. Canonicalize that projection so a requested 0644 mode
		// and the 0666 mode reported by os.FileInfo fingerprint identically.
		perm := mode.Perm()
		switch {
		case perm == 0:
			return 0
		case perm&0o222 == 0:
			return 0o444
		default:
			return 0o666
		}
	}
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

func failure(path, operation string, err error) Failure {
	return Failure{Path: filepath.ToSlash(path), Operation: operation, Message: err.Error()}
}

func sortConflicts(conflicts []Conflict) {
	sort.Slice(conflicts, func(i, j int) bool { return conflicts[i].Path < conflicts[j].Path })
}
