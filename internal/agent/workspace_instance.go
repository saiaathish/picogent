package agent

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/saiaathish/picogent/internal/securefile"
	"github.com/saiaathish/picogent/internal/workspace"
)

const (
	undoWorkspaceInstanceMarker = ".picogent/workspace-instance"
	undoWorkspaceInstanceBytes  = 32
	maxUndoWorkspaceMarkerBytes = 128
)

// undoWorkspaceInstance combines a filesystem directory identity with a
// random marker stored in the workspace. The directory identity rejects a
// copied workspace; the marker reduces the chance that a recycled filesystem
// ID can authorize an old journal.
type undoWorkspaceInstance struct {
	Directory workspace.Identity `json:"directory"`
	Token     string             `json:"token"`
}

func (i undoWorkspaceInstance) valid() bool {
	if !i.Directory.Known || (i.Directory.Volume == 0 && i.Directory.File == 0) || len(i.Token) != undoWorkspaceInstanceBytes*2 {
		return false
	}
	decoded, err := hex.DecodeString(i.Token)
	return err == nil && len(decoded) == undoWorkspaceInstanceBytes
}

func undoWorkspaceMarkerPath(workspacePath string) (string, string, error) {
	root, err := undoWorkspaceIdentity(workspacePath)
	if err != nil {
		return "", "", err
	}
	marker := filepath.Join(root, filepath.FromSlash(undoWorkspaceInstanceMarker))
	return root, marker, nil
}

func readUndoWorkspaceInstance(workspacePath string) (undoWorkspaceInstance, error) {
	root, marker, err := undoWorkspaceMarkerPath(workspacePath)
	if err != nil {
		return undoWorkspaceInstance{}, err
	}
	before, err := workspace.DirectoryIdentity(root)
	if err != nil {
		return undoWorkspaceInstance{}, fmt.Errorf("identify undo workspace: %w", err)
	}
	data, err := securefile.ReadFileLimited(marker, maxUndoWorkspaceMarkerBytes)
	if err != nil {
		return undoWorkspaceInstance{}, fmt.Errorf("read undo workspace marker: %w", err)
	}
	token := strings.TrimSpace(string(data))
	instance := undoWorkspaceInstance{Directory: before, Token: token}
	if !instance.valid() {
		return undoWorkspaceInstance{}, errors.New("undo workspace marker is malformed")
	}
	after, err := workspace.DirectoryIdentity(root)
	if err != nil {
		return undoWorkspaceInstance{}, fmt.Errorf("recheck undo workspace: %w", err)
	}
	if before != after {
		return undoWorkspaceInstance{}, errors.New("undo workspace changed while reading its marker")
	}
	return instance, nil
}

func validateUndoWorkspaceInstance(workspacePath string, expected undoWorkspaceInstance) error {
	if !expected.valid() {
		return errors.New("durable undo workspace instance is unavailable")
	}
	current, err := readUndoWorkspaceInstance(workspacePath)
	if err != nil {
		return err
	}
	if current != expected {
		return errors.New("durable undo workspace instance changed")
	}
	return nil
}

func ensureUndoWorkspaceInstance(workspacePath string) (undoWorkspaceInstance, error) {
	root, marker, err := undoWorkspaceMarkerPath(workspacePath)
	if err != nil {
		return undoWorkspaceInstance{}, err
	}
	before, err := workspace.DirectoryIdentity(root)
	if err != nil {
		return undoWorkspaceInstance{}, fmt.Errorf("identify undo workspace: %w", err)
	}
	if err := securefile.EnsureDir(filepath.Dir(marker), 0o700); err != nil {
		return undoWorkspaceInstance{}, fmt.Errorf("prepare undo workspace marker: %w", err)
	}
	lock, err := securefile.OpenLockFile(marker + ".lock")
	if err != nil {
		return undoWorkspaceInstance{}, fmt.Errorf("open undo workspace marker lock: %w", err)
	}
	defer lock.Close()
	unlock, err := securefile.LockFile(lock, true)
	if err != nil {
		return undoWorkspaceInstance{}, fmt.Errorf("lock undo workspace marker: %w", err)
	}
	defer unlock()

	data, err := securefile.ReadFileLimited(marker, maxUndoWorkspaceMarkerBytes)
	if errors.Is(err, os.ErrNotExist) {
		token, tokenErr := newUndoWorkspaceToken()
		if tokenErr != nil {
			return undoWorkspaceInstance{}, tokenErr
		}
		// Publish the first marker atomically. A direct exclusive write can be
		// left truncated if the process exits while writing its first token.
		if writeErr := securefile.WriteAtomicDurable(marker, []byte(token+"\n"), 0o600); writeErr != nil {
			return undoWorkspaceInstance{}, fmt.Errorf("create undo workspace marker: %w", writeErr)
		}
		data, err = securefile.ReadFileLimited(marker, maxUndoWorkspaceMarkerBytes)
	}
	if err != nil {
		return undoWorkspaceInstance{}, fmt.Errorf("read undo workspace marker: %w", err)
	}
	instance := undoWorkspaceInstance{Directory: before, Token: strings.TrimSpace(string(data))}
	if !instance.valid() {
		return undoWorkspaceInstance{}, errors.New("undo workspace marker is malformed")
	}
	after, err := workspace.DirectoryIdentity(root)
	if err != nil {
		return undoWorkspaceInstance{}, fmt.Errorf("recheck undo workspace: %w", err)
	}
	if before != after {
		return undoWorkspaceInstance{}, errors.New("undo workspace changed while initializing its marker")
	}
	return instance, nil
}

func newUndoWorkspaceToken() (string, error) {
	data := make([]byte, undoWorkspaceInstanceBytes)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("generate undo workspace marker: %w", err)
	}
	return hex.EncodeToString(data), nil
}
