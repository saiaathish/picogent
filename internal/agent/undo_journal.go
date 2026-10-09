package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/saiaathish/picogent/internal/checkpoint"
	"github.com/saiaathish/picogent/internal/securefile"
	"github.com/saiaathish/picogent/internal/workspace"
)

const (
	undoJournalVersion          = 3
	undoJournalTaskOwnerVersion = 2
	undoJournalLegacyVersion    = 1
	undoJournalSealed           = "sealed"
	undoJournalPending          = "recovery-pending"
	undoJournalRestored         = "restored"
	undoJournalMaxBytes         = 12 << 20
)

var (
	errLegacyUndoJournal          = errors.New("legacy undo journal lacks task owner identity")
	errLegacyUndoWorkspaceBinding = errors.New("legacy undo journal lacks workspace instance identity")
)

func isLegacyUndoUnavailable(err error) bool {
	return errors.Is(err, errLegacyUndoJournal) || errors.Is(err, errLegacyUndoWorkspaceBinding)
}

// undoJournal is deliberately separate from task state. Task revisions
// describe outcome progress; this record owns the native-file bytes needed for
// one latest-turn undo and survives a process restart.
type undoJournal struct {
	Version           int                   `json:"version"`
	State             string                `json:"state"`
	Workspace         string                `json:"workspace"`
	WorkspaceInstance undoWorkspaceInstance `json:"workspace_instance"`
	SessionID         string                `json:"session_id"`
	TurnSequence      uint64                `json:"turn_sequence"`
	TaskID            string                `json:"task_id,omitempty"`
	IntentRevision    uint64                `json:"turn_intent_revision"`
	Checkpoint        checkpoint.Record     `json:"checkpoint"`
}

func undoWorkspaceIdentity(workspace string) (string, error) {
	if strings.TrimSpace(workspace) == "" {
		return "", errors.New("undo workspace is empty")
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return "", fmt.Errorf("resolve undo workspace: %w", err)
	}
	identity, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve undo workspace: %w", err)
	}
	info, err := os.Stat(identity)
	if err != nil {
		return "", fmt.Errorf("stat undo workspace: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("undo workspace is not a directory")
	}
	return filepath.Clean(identity), nil
}

func sameUndoWorkspaceIdentity(left, right string) bool {
	leftEmpty := strings.TrimSpace(left) == ""
	rightEmpty := strings.TrimSpace(right) == ""
	if leftEmpty || rightEmpty {
		return leftEmpty && rightEmpty
	}
	leftIdentity, err := undoWorkspaceIdentity(left)
	if err != nil {
		return false
	}
	rightIdentity, err := undoWorkspaceIdentity(right)
	if err != nil {
		return false
	}
	leftInfo, err := os.Stat(leftIdentity)
	if err != nil {
		return false
	}
	rightInfo, err := os.Stat(rightIdentity)
	return err == nil && os.SameFile(leftInfo, rightInfo)
}

func safeUndoSessionID(id string) bool {
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

func undoJournalPaths(workspace, sessionID string) (string, string, error) {
	if !safeUndoSessionID(sessionID) {
		return "", "", errors.New("invalid undo session id")
	}
	root, err := undoWorkspaceIdentity(workspace)
	if err != nil {
		return "", "", err
	}
	dir := filepath.Join(root, ".picogent", "undo")
	sealed := filepath.Join(dir, sessionID+".json")
	pending := filepath.Join(dir, sessionID+".pending.json")
	return sealed, pending, nil
}

func validateUndoJournal(journal undoJournal, workspace, sessionID string) error {
	if journal.Version != undoJournalVersion && journal.Version != undoJournalTaskOwnerVersion && journal.Version != undoJournalLegacyVersion {
		return fmt.Errorf("unsupported undo journal version %d", journal.Version)
	}
	if journal.State != undoJournalSealed && journal.State != undoJournalPending && journal.State != undoJournalRestored {
		return fmt.Errorf("unsupported undo journal state %q", journal.State)
	}
	if journal.SessionID != sessionID || !safeUndoSessionID(journal.SessionID) {
		return errors.New("undo journal session mismatch")
	}
	if journal.TurnSequence == 0 {
		return errors.New("undo journal turn sequence is empty")
	}
	if journal.Version >= undoJournalTaskOwnerVersion && strings.TrimSpace(journal.TaskID) == "" {
		return errors.New("undo journal task owner identity is empty")
	}
	if _, err := undoWorkspaceIdentity(workspace); err != nil {
		return err
	}
	if !sameUndoWorkspaceIdentity(workspace, journal.Workspace) {
		return errors.New("undo journal workspace mismatch")
	}
	if journal.Version == undoJournalVersion {
		if !journal.WorkspaceInstance.valid() {
			return errors.New("undo journal workspace instance is missing or malformed")
		}
		if err := validateUndoWorkspaceInstance(workspace, journal.WorkspaceInstance); err != nil {
			return fmt.Errorf("undo journal workspace instance mismatch: %w", err)
		}
	}
	if _, err := checkpoint.Import(workspace, journal.Checkpoint); err != nil {
		return fmt.Errorf("validate undo journal checkpoint: %w", err)
	}
	return nil
}

func encodeUndoJournal(journal undoJournal) ([]byte, error) {
	data, err := json.MarshalIndent(journal, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode undo journal: %w", err)
	}
	data = append(data, '\n')
	if len(data) > undoJournalMaxBytes {
		return nil, fmt.Errorf("undo journal exceeds the %d-byte limit", undoJournalMaxBytes)
	}
	return data, nil
}

func saveUndoJournal(workspacePath, sessionID string, journal undoJournal, pending bool) error {
	return saveUndoJournalWithHooks(workspacePath, sessionID, journal, pending, workspace.WriteHooks{})
}

func saveUndoJournalWithHooks(workspacePath, sessionID string, journal undoJournal, pending bool, hooks workspace.WriteHooks) error {
	if journal.Version != undoJournalVersion {
		return fmt.Errorf("cannot write undo journal version %d", journal.Version)
	}
	if err := validateUndoJournal(journal, workspacePath, sessionID); err != nil {
		return err
	}
	sealedPath, pendingPath, err := undoJournalPaths(workspacePath, sessionID)
	if err != nil {
		return err
	}
	data, err := encodeUndoJournal(journal)
	if err != nil {
		return err
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(sealedPath)))
	path := sealedPath
	if pending {
		path = pendingPath
	}
	check := hooks.Check
	hooks.Check = func() error {
		if check != nil {
			if err := check(); err != nil {
				return err
			}
		}
		return validateUndoWorkspaceInstance(root, journal.WorkspaceInstance)
	}
	checkRootIdentity := hooks.CheckRootIdentity
	hooks.CheckRootIdentity = func(actual workspace.Identity) error {
		if !actual.Known || actual != journal.WorkspaceInstance.Directory {
			return workspace.ErrRootIdentityChanged
		}
		if checkRootIdentity != nil {
			return checkRootIdentity(actual)
		}
		return nil
	}
	hooks.CreateParentMode = 0o700
	hooks.PrivateParentDirectory = true
	if err := workspace.WriteAtomicDurableWithModeAndHooks(root, path, data, 0o600, hooks); err != nil {
		return fmt.Errorf("write undo journal: %w", err)
	}
	return nil
}

func loadUndoJournal(workspacePath, sessionID string, pending bool) (*undoJournal, error) {
	sealedPath, pendingPath, err := undoJournalPaths(workspacePath, sessionID)
	if err != nil {
		return nil, err
	}
	path := sealedPath
	if pending {
		path = pendingPath
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(sealedPath)))
	if err := workspace.SecurePrivateDirectoryAndFiles(root, filepath.Dir(path)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("secure undo journal storage: %w", err)
	}
	data, err := securefile.ReadFileLimited(path, undoJournalMaxBytes)
	if errors.Is(err, os.ErrNotExist) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, fmt.Errorf("read undo journal: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var journal undoJournal
	if err := dec.Decode(&journal); err != nil {
		return nil, fmt.Errorf("decode undo journal: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("multiple JSON values")
		}
		return nil, fmt.Errorf("decode undo journal: %w", err)
	}
	if journal.Version >= undoJournalTaskOwnerVersion {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(data, &fields); err != nil {
			return nil, fmt.Errorf("decode undo journal fields: %w", err)
		}
		revision, present := fields["turn_intent_revision"]
		if !present || bytes.Equal(bytes.TrimSpace(revision), []byte("null")) {
			return nil, fmt.Errorf("version-%d undo journal is missing turn intent revision", journal.Version)
		}
		var parsed uint64
		if err := json.Unmarshal(revision, &parsed); err != nil {
			return nil, fmt.Errorf("decode undo journal turn intent revision: %w", err)
		}
	}
	if err := validateUndoJournal(journal, workspacePath, sessionID); err != nil {
		return nil, err
	}
	return &journal, nil
}

func removeUndoJournal(workspacePath, sessionID string, pending bool, expected undoWorkspaceInstance) error {
	if !safeUndoSessionID(sessionID) {
		return errors.New("invalid undo session id")
	}
	root, err := undoWorkspaceIdentity(workspacePath)
	if err != nil {
		return err
	}
	name := sessionID + ".json"
	if pending {
		name = sessionID + ".pending.json"
	}
	path := filepath.Join(".picogent", "undo", name)
	checkInstance := func() error { return validateUndoWorkspaceInstance(workspacePath, expected) }
	checkRootIdentity := func(actual workspace.Identity) error {
		if !expected.valid() || !actual.Known || actual != expected.Directory {
			return errors.New("durable undo workspace instance directory changed")
		}
		return nil
	}
	if err := workspace.RemoveWithChecks(root, path, checkInstance, checkRootIdentity); err != nil {
		if isMissingUndoJournal(err) {
			return nil
		}
		return fmt.Errorf("remove undo journal: %w", err)
	}
	return nil
}

func isMissingUndoJournal(err error) bool {
	return errors.Is(err, os.ErrNotExist) && !errors.Is(err, workspace.ErrWorkspaceParentNotExist)
}

func removeAllUndoJournals(workspacePath, sessionID string, expected undoWorkspaceInstance) error {
	return errors.Join(removeUndoJournal(workspacePath, sessionID, false, expected), removeUndoJournal(workspacePath, sessionID, true, expected))
}

func loadLatestDurableUndo(workspace, sessionID string, generation uint64, authorities ...undoTaskStoreAuthority) (*turnUndo, error) {
	pending, pendingErr := loadUndoJournal(workspace, sessionID, true)
	if pendingErr == nil {
		if pending.Version == undoJournalLegacyVersion || strings.TrimSpace(pending.TaskID) == "" {
			return nil, errLegacyUndoJournal
		}
		if pending.Version < undoJournalVersion {
			return nil, errLegacyUndoWorkspaceBinding
		}
		if pending.State != undoJournalPending && pending.State != undoJournalRestored {
			return nil, fmt.Errorf("pending undo journal has invalid state %q", pending.State)
		}
		cp, err := checkpoint.ImportBound(workspace, pending.Checkpoint, pending.WorkspaceInstance.Directory)
		if err != nil {
			return nil, err
		}
		u := &turnUndo{
			workspace:          workspace,
			checkpoint:         cp,
			sessionID:          sessionID,
			sessionGeneration:  generation,
			turnSequence:       pending.TurnSequence,
			taskID:             pending.TaskID,
			turnIntentRevision: pending.IntentRevision,
			workspaceInstance:  pending.WorkspaceInstance,
			durable:            true,
			journalSlot:        undoJournalPending,
		}
		if len(authorities) > 0 {
			u.bindTaskStore(authorities[0].store, authorities[0].epoch)
		}
		if pending.State == undoJournalRestored {
			u.restored = true
			u.restoreMessage = "last turn workspace was restored; retrying durable task recovery"
			return u, nil
		}
		published, found, subsetErr := cp.PublishedSubset()
		if subsetErr != nil && !errors.Is(subsetErr, checkpoint.ErrConflict) {
			return nil, fmt.Errorf("inspect pending undo journal: %w", subsetErr)
		}
		if subsetErr == nil && !found {
			u.pendingUnpublished = true
		} else if subsetErr == nil && published != nil {
			u.checkpoint = published
		}
		return u, nil
	} else if !errors.Is(pendingErr, os.ErrNotExist) {
		return nil, pendingErr
	}

	sealed, sealedErr := loadUndoJournal(workspace, sessionID, false)
	if errors.Is(sealedErr, os.ErrNotExist) {
		return nil, nil
	}
	if sealedErr != nil {
		return nil, sealedErr
	}
	if sealed.Version == undoJournalLegacyVersion || strings.TrimSpace(sealed.TaskID) == "" {
		return nil, errLegacyUndoJournal
	}
	if sealed.Version < undoJournalVersion {
		return nil, errLegacyUndoWorkspaceBinding
	}
	if sealed.State != undoJournalSealed && sealed.State != undoJournalRestored {
		return nil, fmt.Errorf("sealed undo journal has invalid state %q", sealed.State)
	}
	cp, err := checkpoint.ImportBound(workspace, sealed.Checkpoint, sealed.WorkspaceInstance.Directory)
	if err != nil {
		return nil, err
	}
	u := &turnUndo{
		workspace:          workspace,
		checkpoint:         cp,
		sessionID:          sessionID,
		sessionGeneration:  generation,
		turnSequence:       sealed.TurnSequence,
		taskID:             sealed.TaskID,
		turnIntentRevision: sealed.IntentRevision,
		workspaceInstance:  sealed.WorkspaceInstance,
		durable:            true,
		journalSlot:        undoJournalSealed,
	}
	if len(authorities) > 0 {
		u.bindTaskStore(authorities[0].store, authorities[0].epoch)
	}
	if sealed.State == undoJournalRestored {
		u.restored = true
		u.restoreMessage = "last turn workspace was restored; retrying durable task recovery"
	}
	return u, nil
}
