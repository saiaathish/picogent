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
	ErrNotSealed        = errors.New("checkpoint is not sealed")
	ErrAlreadySealed    = errors.New("checkpoint is already sealed")
	ErrAlreadyRestored  = errors.New("checkpoint is already restored")
	ErrConflict         = errors.New("checkpoint restore conflicts with newer changes")
	ErrWorkspaceChanged = errors.New("checkpoint workspace directory changed")
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

// RecordEntry contains the pre-turn state and the expected publication
// fingerprint for one workspace-relative regular file. Published retains a
// known earlier publication when a later same-path write was prepared and its
// publication is unresolved, including a sealed record with a live conflict.
type RecordEntry struct {
	Path         string `json:"path"`
	BeforeExists bool   `json:"before_exists"`
	BeforeMode   uint32 `json:"before_mode,omitempty"`
	BeforeData   []byte `json:"before_data,omitempty"`
	Expected     string `json:"expected"`
	Published    string `json:"published,omitempty"`
}

// Conflict identifies a path that no longer matches the checkpoint's expected state.
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
	mu           sync.Mutex
	rootInput    string
	root         string
	rootIdentity workspace.Identity
	entries      []entry
	sealed       bool
	restored     bool

	// restoreBeforeApply is only used by package tests to exercise an
	// interleaving between restore preflight and publication.
	restoreBeforeApply func(string)
	// These hooks are only used by package tests to exercise root replacement
	// during the read-only restore preflight.
	restoreAfterInitialIdentityCheck func()
	restoreBeforePreflightComplete   func()
	restoreReadHook                  func(stage, path string, after bool)
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
	rootIdentity, err := rootDirectoryIdentity(root)
	if err != nil {
		return nil, fmt.Errorf("identify checkpoint workspace: %w", err)
	}
	if len(paths) == 0 {
		return nil, errors.New("checkpoint requires at least one path")
	}

	cp := &Checkpoint{rootInput: rootInput, root: root, rootIdentity: rootIdentity}
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
	return c.drop(path, false)
}

// DropUnprepared removes a rejected native path only when it has no known
// publication. An earlier prepared write remains available for conflict-aware
// undo even if a later same-path edit is rejected.
func (c *Checkpoint) DropUnprepared(path string) error {
	return c.drop(path, true)
}

func (c *Checkpoint) drop(path string, unpreparedOnly bool) error {
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
		if unpreparedOnly && c.entries[i].expectedSet {
			return nil
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

// RootIdentity returns the filesystem identity captured when this checkpoint
// was created or imported. It is immutable and intended for equality checks.
func (c *Checkpoint) RootIdentity() workspace.Identity {
	if c == nil {
		return workspace.Identity{}
	}
	return c.rootIdentity
}

// Seal finalizes the expected states for the turn. Prepared writes retain their
// exact publication fingerprint unless the live state matches the captured
// pre-turn state or a known earlier publication. An unfamiliar live state must
// remain a restore conflict, including when it was written before Seal. Paths
// with an unresolved later write also retain a known earlier publication for
// an exact-match restore retry. Paths without a prepared write are
// fingerprinted here.
func (c *Checkpoint) Seal() error {
	return c.seal(false)
}

// SealPrepared seals native-file undo without adopting live bytes for a path
// whose write never reached PrepareExpected. A rejected pre-publication write
// may be followed by a user edit; its original capture is not ownership proof.
// Generic callers that own their mutation boundary may still use Seal.
func (c *Checkpoint) SealPrepared() error {
	return c.seal(true)
}

func (c *Checkpoint) seal(preparedOnly bool) error {
	if c == nil {
		return ErrNotSealed
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sealed {
		return ErrAlreadySealed
	}

	entries := c.entries
	if preparedOnly {
		entries = make([]entry, 0, len(c.entries))
		for _, item := range c.entries {
			if item.expectedSet {
				entries = append(entries, item)
			}
		}
	}
	states := make([]fileState, len(entries))
	for i := range entries {
		state, err := readWorkspaceFile(c.root, entries[i].path)
		if err != nil {
			return fmt.Errorf("seal %q: %w", filepath.ToSlash(entries[i].path), err)
		}
		states[i] = state
	}
	for i := range entries {
		item := &entries[i]
		current := states[i].sum
		retainPublished := false
		switch {
		case !item.expectedSet:
			item.expected = current
		case current == item.expected:
			// The exact prepared state reached publication, even if the tool
			// subsequently reported a close or directory-sync error.
		case current == item.before.sum:
			item.expected = item.before.sum
		case item.publishedSet && current == item.published:
			// A later same-path write failed before its rename. Retain the
			// earlier publication so it can still be undone after restart.
			item.expected = item.published
		default:
			// Never adopt arbitrary live bytes as an agent publication. If
			// the prepared write would return to the pre-turn state, retain
			// the earlier publication instead of silently dropping its undo.
			if item.expected == item.before.sum && item.publishedSet {
				item.expected = item.published
			}
			// A user edit leaves the later publication unresolved. Keep the
			// exact known earlier state so a conflict retry can undo it, even
			// after export/import; never fingerprint the unfamiliar live bytes.
			retainPublished = item.publishedSet && item.published != item.before.sum && item.published != item.expected
		}
		item.expectedSet = true
		if !retainPublished {
			item.published = fingerprint{}
			item.publishedSet = false
		}
	}
	c.entries = entries
	c.sealed = true
	return nil
}

// PrepareExpected records the exact regular-file state that an imminent
// atomic write will publish. Native undo uses it to protect its in-memory
// expectation and, for durable turns, to publish a pending recovery record
// before the workspace rename. The checkpoint remains
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
	rootIdentity, err := rootDirectoryIdentity(root)
	if err != nil {
		return nil, fmt.Errorf("identify checkpoint workspace: %w", err)
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
	return &Checkpoint{rootInput: rootInput, root: root, rootIdentity: rootIdentity, entries: entries, sealed: true}, nil
}

// ImportBound imports a portable record only when the current workspace root
// is the same directory object captured by its durable owner.
func ImportBound(workspace string, record Record, expected workspace.Identity) (*Checkpoint, error) {
	if !expected.Known {
		return nil, fmt.Errorf("%w: saved identity is unknown", ErrWorkspaceChanged)
	}
	cp, err := Import(workspace, record)
	if err != nil {
		return nil, err
	}
	if cp.rootIdentity != expected {
		return nil, ErrWorkspaceChanged
	}
	return cp, nil
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
	return &Checkpoint{rootInput: c.rootInput, root: c.root, rootIdentity: c.rootIdentity, entries: entries, sealed: true}, true, nil
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
	return c.RestoreWithWorkspaceGuard(nil)
}

// RestoreWithWorkspaceGuard restores the checkpoint while also checking a
// caller-owned durable workspace binding at preflight and publication
// boundaries. The guard pairs filesystem identity with an independent token.
func (c *Checkpoint) RestoreWithWorkspaceGuard(guard func() error) (RestoreResult, error) {
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
	if err := checkRestoreWorkspace(c.root, c.rootIdentity, guard); err != nil {
		result.Failures = append(result.Failures, failure(".", "workspace identity", err))
		return result, err
	}
	if c.restoreAfterInitialIdentityCheck != nil {
		c.restoreAfterInitialIdentityCheck()
	}

	mutations := make([]mutation, 0, len(c.entries))
	for i := range c.entries {
		current, err := c.readRestoreWorkspaceFile(c.entries[i].path, "preflight")
		if err != nil {
			result.Failures = append(result.Failures, failure(c.entries[i].path, "inspect", err))
			if errors.Is(err, ErrWorkspaceChanged) {
				return result, err
			}
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
		// A sealed conflict may retain an earlier known publication alongside
		// the unresolved prepared expectation. Either exact state can be
		// undone; arbitrary user bytes still block the entire restore.
		if current.sum != c.entries[i].expected && (!c.entries[i].publishedSet || current.sum != c.entries[i].published) {
			result.Conflicts = append(result.Conflicts, Conflict{
				Path:   filepath.ToSlash(c.entries[i].path),
				Reason: "file changed after checkpoint was sealed",
			})
			continue
		}
		mutations = append(mutations, mutation{entry: &c.entries[i], after: current})
	}
	if c.restoreBeforePreflightComplete != nil {
		c.restoreBeforePreflightComplete()
	}
	if err := checkRestoreWorkspace(c.root, c.rootIdentity, guard); err != nil {
		result.Failures = append(result.Failures, failure(".", "workspace identity", err))
		return result, err
	}
	if len(result.Conflicts) > 0 {
		sortConflicts(result.Conflicts)
		return result, ErrConflict
	}
	if len(result.Failures) > 0 {
		return result, errors.New("checkpoint restore preflight failed")
	}
	if len(mutations) == 0 {
		// Re-read before consuming a no-op checkpoint. A transient replacement
		// can expose pre-turn bytes during the first preflight and then restore
		// the original root before the authority recheck.
		for i := range c.entries {
			current, err := c.readRestoreWorkspaceFile(c.entries[i].path, "noop-confirmation")
			if err != nil {
				result.Failures = append(result.Failures, failure(c.entries[i].path, "confirm unchanged", err))
				return result, fmt.Errorf("checkpoint restore confirmation failed: %w", err)
			}
			if current.sum != c.entries[i].before.sum {
				result.Conflicts = append(result.Conflicts, Conflict{
					Path: filepath.ToSlash(c.entries[i].path), Reason: "file changed during restore preflight",
				})
			}
		}
		if err := checkRestoreWorkspace(c.root, c.rootIdentity, guard); err != nil {
			result.Failures = append(result.Failures, failure(".", "workspace identity", err))
			return result, err
		}
		if len(result.Conflicts) > 0 {
			result.Unchanged = nil
			sortConflicts(result.Conflicts)
			return result, ErrConflict
		}
		result.Complete = true
		c.restored = true
		sort.Strings(result.Unchanged)
		return result, nil
	}

	for i := range mutations {
		if opErr := applyMutation(c.root, &mutations[i], c.restoreBeforeApply, c.rootIdentity, guard, c.readRestoreWorkspaceFile); opErr != nil {
			if errors.Is(opErr.err, ErrConflict) {
				result.Conflicts = append(result.Conflicts, Conflict{
					Path: filepath.ToSlash(opErr.path), Reason: opErr.err.Error(),
				})
			} else {
				result.Failures = append(result.Failures, failure(opErr.path, opErr.operation, opErr.err))
			}
			if errors.Is(opErr.err, ErrWorkspaceChanged) {
				return result, fmt.Errorf("checkpoint restore stopped after workspace replacement: %w", opErr.err)
			}
			result.RolledBack = rollback(c.root, mutations[:i+1], &result, c.rootIdentity, guard, c.readRestoreWorkspaceFile)
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

func applyMutation(root string, m *mutation, beforeWrite func(string), expectedRootIdentity workspace.Identity, guard func() error, readRestoreFile func(string, string) (fileState, error)) *operationError {
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
	if err := checkRestoreWorkspace(root, expectedRootIdentity, guard); err != nil {
		return &operationError{m.entry.path, "verify workspace identity", err}
	}
	if err := writeWorkspaceState(root, m.entry.path, m.after, m.entry.before, expectedRootIdentity, guard); err != nil {
		if errors.Is(err, workspace.ErrContentConflict) {
			if guardErr := checkRestoreWorkspace(root, expectedRootIdentity, guard); guardErr != nil {
				return &operationError{m.entry.path, "verify workspace identity", guardErr}
			}
			current, inspectErr := readRestoreFile(m.entry.path, "conflict-confirmation")
			if guardErr := checkRestoreWorkspace(root, expectedRootIdentity, guard); guardErr != nil {
				return &operationError{m.entry.path, "verify workspace identity", guardErr}
			}
			if errors.Is(inspectErr, ErrWorkspaceChanged) {
				return &operationError{m.entry.path, "verify workspace identity", inspectErr}
			}
			if inspectErr == nil && current.sum == m.entry.before.sum {
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

func rollback(root string, mutations []mutation, result *RestoreResult, expectedRootIdentity workspace.Identity, guard func() error, readRestoreFile func(string, string) (fileState, error)) bool {
	ok := true
	attempted := false
	for i := len(mutations) - 1; i >= 0; i-- {
		m := &mutations[i]
		if !m.applied {
			continue
		}
		attempted = true
		if err := checkRestoreWorkspace(root, expectedRootIdentity, guard); err != nil {
			result.Failures = append(result.Failures, failure(m.entry.path, "rollback workspace identity", err))
			return false
		}
		current, err := readRestoreFile(m.entry.path, "rollback")
		if err != nil {
			result.Failures = append(result.Failures, failure(m.entry.path, "rollback inspect", err))
			ok = false
			continue
		}
		if err := checkRestoreWorkspace(root, expectedRootIdentity, guard); err != nil {
			result.Failures = append(result.Failures, failure(m.entry.path, "rollback workspace identity", err))
			return false
		}
		if current.sum != m.entry.before.sum {
			result.Failures = append(result.Failures, failure(m.entry.path, "rollback conflict", ErrConflict))
			ok = false
			continue
		}
		if err := writeWorkspaceState(root, m.entry.path, m.entry.before, m.after, expectedRootIdentity, guard); err != nil {
			result.Failures = append(result.Failures, failure(m.entry.path, "rollback restore", err))
			ok = false
			if errors.Is(err, ErrWorkspaceChanged) {
				return false
			}
		}
	}
	return attempted && ok
}

func rootDirectoryIdentity(root string) (workspace.Identity, error) {
	identity, err := workspace.DirectoryIdentity(root)
	if err != nil {
		return workspace.Identity{}, err
	}
	if !identity.Known {
		return workspace.Identity{}, ErrWorkspaceChanged
	}
	return identity, nil
}

func checkWorkspaceIdentity(root string, expected workspace.Identity) error {
	if !expected.Known {
		return fmt.Errorf("%w: checkpoint identity is unknown", ErrWorkspaceChanged)
	}
	current, err := rootDirectoryIdentity(root)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrWorkspaceChanged, err)
	}
	if current != expected {
		return ErrWorkspaceChanged
	}
	return nil
}

func checkWorkspaceRootHandleIdentity(expected workspace.Identity) workspace.RootIdentityCheck {
	return func(actual workspace.Identity) error {
		if !expected.Known || !actual.Known || actual != expected {
			return fmt.Errorf("%w: opened root descriptor differs from checkpoint", ErrWorkspaceChanged)
		}
		return nil
	}
}

func checkRestoreWorkspace(root string, expected workspace.Identity, guard func() error) error {
	if err := checkWorkspaceIdentity(root, expected); err != nil {
		return err
	}
	if guard != nil {
		if err := guard(); err != nil {
			return fmt.Errorf("%w: durable workspace guard: %v", ErrWorkspaceChanged, err)
		}
	}
	return nil
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

func readWorkspaceFileWithRootIdentity(root, rel string, expectedRootIdentity workspace.Identity) (fileState, error) {
	f, err := workspace.OpenReadWithRootIdentity(root, rel, expectedRootIdentity)
	if errors.Is(err, workspace.ErrRootIdentityChanged) {
		return fileState{}, fmt.Errorf("%w: %w", ErrWorkspaceChanged, err)
	}
	if errors.Is(err, workspace.ErrWorkspaceParentNotExist) {
		return fileState{}, fmt.Errorf("%w: %w", ErrWorkspaceChanged, err)
	}
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

func (c *Checkpoint) readRestoreWorkspaceFile(rel, stage string) (fileState, error) {
	if c.restoreReadHook != nil {
		c.restoreReadHook(stage, filepath.ToSlash(rel), false)
	}
	state, err := readWorkspaceFileWithRootIdentity(c.root, rel, c.rootIdentity)
	if c.restoreReadHook != nil {
		c.restoreReadHook(stage, filepath.ToSlash(rel), true)
	}
	return state, err
}

func writeWorkspaceState(root, rel string, expected, state fileState, expectedRootIdentity workspace.Identity, guard func() error) error {
	checkRoot := func() error { return checkRestoreWorkspace(root, expectedRootIdentity, guard) }
	checkRootIdentity := checkWorkspaceRootHandleIdentity(expectedRootIdentity)
	hooks := workspace.WriteHooks{Check: checkRoot, CheckRootIdentity: checkRootIdentity}
	if !state.exists {
		if expected.exists {
			return workspace.RemoveIfUnchangedWithChecks(root, rel, expected.data, expected.mode, checkRoot, checkRootIdentity)
		}
		err := workspace.RemoveWithChecks(root, rel, checkRoot, checkRootIdentity)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if expected.exists {
		return workspace.WriteAtomicIfUnchangedWithModeAndHooks(root, rel, expected.data, expected.mode, state.data, state.mode, hooks)
	}
	return workspace.WriteAtomicIfMissingWithModeAndHooks(root, rel, state.data, state.mode, hooks)
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
