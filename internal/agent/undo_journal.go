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
)

const (
	undoJournalVersion  = 1
	undoJournalSealed   = "sealed"
	undoJournalPending  = "recovery-pending"
	undoJournalRestored = "restored"
	undoJournalMaxBytes = 12 << 20
)

// undoJournal is deliberately separate from task state. Task revisions
// describe outcome progress; this record owns the native-file bytes needed for
// one latest-turn undo and survives a process restart.
type undoJournal struct {
	Version      int               `json:"version"`
	State        string            `json:"state"`
	Workspace    string            `json:"workspace"`
	SessionID    string            `json:"session_id"`
	TurnSequence uint64            `json:"turn_sequence"`
	Checkpoint   checkpoint.Record `json:"checkpoint"`
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
	if journal.Version != undoJournalVersion {
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
	identity, err := undoWorkspaceIdentity(workspace)
	if err != nil {
		return err
	}
	if journal.Workspace != identity {
		return errors.New("undo journal workspace mismatch")
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

func saveUndoJournal(workspace, sessionID string, journal undoJournal, pending bool) error {
	if err := validateUndoJournal(journal, workspace, sessionID); err != nil {
		return err
	}
	sealedPath, pendingPath, err := undoJournalPaths(workspace, sessionID)
	if err != nil {
		return err
	}
	data, err := encodeUndoJournal(journal)
	if err != nil {
		return err
	}
	dir := filepath.Dir(sealedPath)
	if err := securefile.EnsureDir(dir, 0o700); err != nil {
		return fmt.Errorf("create undo journal directory: %w", err)
	}
	path := sealedPath
	if pending {
		path = pendingPath
	}
	if err := securefile.WriteAtomicDurable(path, data, 0o600); err != nil {
		return fmt.Errorf("write undo journal: %w", err)
	}
	return nil
}

func loadUndoJournal(workspace, sessionID string, pending bool) (*undoJournal, error) {
	sealedPath, pendingPath, err := undoJournalPaths(workspace, sessionID)
	if err != nil {
		return nil, err
	}
	path := sealedPath
	if pending {
		path = pendingPath
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
	if err := validateUndoJournal(journal, workspace, sessionID); err != nil {
		return nil, err
	}
	return &journal, nil
}

func removeUndoJournal(workspace, sessionID string, pending bool) error {
	sealedPath, pendingPath, err := undoJournalPaths(workspace, sessionID)
	if err != nil {
		return err
	}
	path := sealedPath
	if pending {
		path = pendingPath
	}
	if err := securefile.RemoveFile(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove undo journal: %w", err)
	}
	return nil
}

func removeAllUndoJournals(workspace, sessionID string) error {
	return errors.Join(removeUndoJournal(workspace, sessionID, false), removeUndoJournal(workspace, sessionID, true))
}

func loadLatestDurableUndo(workspace, sessionID string, generation uint64) (*turnUndo, error) {
	pending, pendingErr := loadUndoJournal(workspace, sessionID, true)
	if pendingErr == nil {
		if pending.State != undoJournalPending && pending.State != undoJournalRestored {
			return nil, fmt.Errorf("pending undo journal has invalid state %q", pending.State)
		}
		cp, err := checkpoint.Import(workspace, pending.Checkpoint)
		if err != nil {
			return nil, err
		}
		u := &turnUndo{
			workspace:         workspace,
			checkpoint:        cp,
			sessionID:         sessionID,
			sessionGeneration: generation,
			turnSequence:      pending.TurnSequence,
			durable:           true,
			journalSlot:       undoJournalPending,
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
			if err := removeUndoJournal(workspace, sessionID, true); err != nil {
				return nil, err
			}
			pending = nil
		} else if subsetErr == nil && published != nil {
			u.checkpoint = published
		}
		if pending != nil {
			return u, nil
		}
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
	if sealed.State != undoJournalSealed && sealed.State != undoJournalRestored {
		return nil, fmt.Errorf("sealed undo journal has invalid state %q", sealed.State)
	}
	cp, err := checkpoint.Import(workspace, sealed.Checkpoint)
	if err != nil {
		return nil, err
	}
	u := &turnUndo{
		workspace:         workspace,
		checkpoint:        cp,
		sessionID:         sessionID,
		sessionGeneration: generation,
		turnSequence:      sealed.TurnSequence,
		durable:           true,
		journalSlot:       undoJournalSealed,
	}
	if sealed.State == undoJournalRestored {
		u.restored = true
		u.restoreMessage = "last turn workspace was restored; retrying durable task recovery"
	}
	return u, nil
}
