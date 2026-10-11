package agent

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
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
	data, err := readUndoWorkspaceFile(root, marker, before, maxUndoWorkspaceMarkerBytes)
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
	return ensureUndoWorkspaceInstanceWithHooks(workspacePath, undoWorkspaceInitHooks{})
}

type undoWorkspaceInitHooks struct {
	AfterIdentity func()
	AfterLock     func()
}

func ensureUndoWorkspaceInstanceWithHooks(workspacePath string, hooks undoWorkspaceInitHooks) (undoWorkspaceInstance, error) {
	root, marker, err := undoWorkspaceMarkerPath(workspacePath)
	if err != nil {
		return undoWorkspaceInstance{}, err
	}
	before, err := workspace.DirectoryIdentity(root)
	if err != nil {
		return undoWorkspaceInstance{}, fmt.Errorf("identify undo workspace: %w", err)
	}
	if hooks.AfterIdentity != nil {
		hooks.AfterIdentity()
	}
	lock, err := workspace.OpenLockWithRootIdentity(root, marker+".lock", before)
	if err != nil {
		return undoWorkspaceInstance{}, fmt.Errorf("open undo workspace marker lock: %w", err)
	}
	defer lock.Close()
	unlock, err := securefile.LockFile(lock, true)
	if err != nil {
		return undoWorkspaceInstance{}, fmt.Errorf("lock undo workspace marker: %w", err)
	}
	defer unlock()
	if hooks.AfterLock != nil {
		hooks.AfterLock()
	}

	data, err := readUndoWorkspaceFile(root, marker, before, maxUndoWorkspaceMarkerBytes)
	if errors.Is(err, os.ErrNotExist) {
		token, tokenErr := newUndoWorkspaceToken()
		if tokenErr != nil {
			return undoWorkspaceInstance{}, tokenErr
		}
		// Publish the first marker atomically. A direct exclusive write can be
		// left truncated if the process exits while writing its first token.
		writeHooks := workspace.WriteHooks{
			CreateParentMode: 0o700,
			CheckRootIdentity: func(actual workspace.Identity) error {
				if !actual.Known || actual != before {
					return workspace.ErrRootIdentityChanged
				}
				return nil
			},
		}
		if writeErr := workspace.WriteAtomicDurableWithModeAndHooks(root, marker, []byte(token+"\n"), 0o600, writeHooks); writeErr != nil {
			return undoWorkspaceInstance{}, fmt.Errorf("create undo workspace marker: %w", writeErr)
		}
		data, err = readUndoWorkspaceFile(root, marker, before, maxUndoWorkspaceMarkerBytes)
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

// readUndoWorkspaceFile never creates or changes permissions. Root identity is
// checked on the native directory handle used to reach the regular file.
func readUndoWorkspaceFile(root, path string, expected workspace.Identity, maxBytes int) ([]byte, error) {
	return readUndoWorkspaceFileWithHooks(root, path, expected, maxBytes, nil)
}

func readUndoWorkspaceFileWithHooks(root, path string, expected workspace.Identity, maxBytes int, afterLock func()) ([]byte, error) {
	file, err := workspace.OpenReadWithRootIdentity(root, path, expected)
	if err != nil {
		return nil, err
	}
	unlock, err := securefile.LockFile(file, false)
	if err != nil {
		return nil, errors.Join(err, file.Close())
	}
	if afterLock != nil {
		afterLock()
	}
	// Lock acquisition can wait while a writer replaces the named inode.
	// Reopen through the same expected root and refuse a stale descriptor.
	openedInfo, statErr := file.Stat()
	if statErr != nil {
		return nil, errors.Join(statErr, unlock(), file.Close())
	}
	named, nameErr := workspace.OpenReadWithRootIdentity(root, path, expected)
	if nameErr != nil {
		return nil, errors.Join(nameErr, unlock(), file.Close())
	}
	namedInfo, statErr := named.Stat()
	closeErr := named.Close()
	if statErr == nil && !os.SameFile(openedInfo, namedInfo) {
		statErr = securefile.ErrReadChanged
	}
	if statErr != nil || closeErr != nil {
		return nil, errors.Join(statErr, closeErr, unlock(), file.Close())
	}
	data, readErr := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	if len(data) > maxBytes {
		data = nil
		readErr = securefile.ErrReadLimit
	}
	err = errors.Join(readErr, unlock(), file.Close())
	return data, err
}

func newUndoWorkspaceToken() (string, error) {
	data := make([]byte, undoWorkspaceInstanceBytes)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("generate undo workspace marker: %w", err)
	}
	return hex.EncodeToString(data), nil
}
