package goal

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/projects"
)

// State is the durable goal text together with a monotonically increasing
// revision. The revision lets a completion callback prove that it is retiring
// the exact goal instance it ran, even when a user replaces a goal with the
// same text while that run is in flight.
type State struct {
	Text     string
	Revision uint64
}

func readPath(workspace string) (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "goals", projects.IDForPath(workspace)+".txt"), nil
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

func revisionPath(path string) string {
	return path + ".revision"
}

func readRevision(path string) (uint64, error) {
	data, err := os.ReadFile(revisionPath(path))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return 0, nil
	}
	revision, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid goal revision: %w", err)
	}
	return revision, nil
}

const stateMagic = "picogent-goal-v1"

func stateBackupPath(path string) string {
	return path + ".bak"
}

func syncParent(path string) {
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return
	}
	_ = dir.Sync()
	_ = dir.Close()
}

func encodeState(state State) []byte {
	return []byte(stateMagic + "\n" + strconv.FormatUint(state.Revision, 10) + "\n" + state.Text)
}

func decodeState(data []byte) (State, error) {
	parts := strings.SplitN(string(data), "\n", 3)
	if len(parts) != 3 || parts[0] != stateMagic {
		return State{}, fmt.Errorf("invalid goal state record")
	}
	revision, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 64)
	if err != nil {
		return State{}, fmt.Errorf("invalid goal revision: %w", err)
	}
	text := strings.TrimSpace(parts[2])
	if text != "" && revision == 0 {
		revision = 1
	}
	return State{Text: text, Revision: revision}, nil
}

// writeAtomic replaces one state record as a transaction. The backup path is
// used only for platforms that cannot rename over an existing file; if a
// process dies between the two renames, loadLocked recovers the old record.
func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".goal-state-")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err == nil {
		syncParent(path)
		return nil
	}
	backup := stateBackupPath(path)
	if err := os.Remove(backup); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(path, backup); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Rename(backup, path)
		return err
	}
	_ = os.Remove(backup)
	syncParent(path)
	return nil
}

func recoverBackup(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	backup := stateBackupPath(path)
	if _, err := os.Stat(backup); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.Rename(backup, path); err != nil {
		return err
	}
	syncParent(path)
	return nil
}

func loadLocked(path string) (State, error) {
	if err := recoverBackup(path); err != nil {
		return State{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			revision, revisionErr := readRevision(path)
			if revisionErr != nil {
				return State{}, revisionErr
			}
			return State{Revision: revision}, nil
		}
		return State{}, err
	}
	if strings.HasPrefix(string(data), stateMagic+"\n") {
		return decodeState(data)
	}
	// Goals written before revisions were introduced were plain text. Give
	// them a real identity and atomically migrate them before returning.
	text := strings.TrimSpace(string(data))
	if text == "" {
		return State{}, nil
	}
	revision, revisionErr := readRevision(path)
	if revisionErr != nil {
		return State{}, revisionErr
	}
	if revision == 0 {
		revision = 1
	}
	state := State{Text: text, Revision: revision}
	if err := writeAtomic(path, encodeState(state)); err != nil {
		return State{}, err
	}
	_ = os.Remove(revisionPath(path))
	return state, nil
}

// LoadState returns the active goal and its durable identity. It intentionally
// does not create the Picogent state directory when no goal exists.
func LoadState(workspace string) (State, error) {
	path, err := readPath(workspace)
	if err != nil {
		return State{}, err
	}
	// Avoid creating a state directory for a read-only status check. If the
	// file exists, its parent necessarily exists and can host the lock file.
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			if _, backupErr := os.Stat(stateBackupPath(path)); os.IsNotExist(backupErr) {
				return State{}, nil
			}
		} else {
			return State{}, err
		}
	}
	unlock, err := acquireGoalLock(path)
	if err != nil {
		return State{}, err
	}
	defer unlock()
	return loadLocked(path)
}

// Load returns the active goal for a workspace, or "" if none.
func Load(workspace string) (string, error) {
	state, err := LoadState(workspace)
	return state.Text, err
}

// Set persists a goal for the workspace.
func Set(workspace, text string) error {
	_, err := SetState(workspace, text)
	return err
}

// SetState persists a goal and returns its new durable revision.
func SetState(workspace, text string) (uint64, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, Clear(workspace)
	}
	path, err := storePath(workspace)
	if err != nil {
		return 0, err
	}
	unlock, err := acquireGoalLock(path)
	if err != nil {
		return 0, err
	}
	defer unlock()
	previousState, err := loadLocked(path)
	if err != nil {
		return 0, err
	}
	revision := previousState.Revision + 1
	if revision == 0 {
		return 0, fmt.Errorf("goal revision overflow")
	}
	if err := writeAtomic(path, encodeState(State{Text: text, Revision: revision})); err != nil {
		return 0, err
	}
	_ = os.Remove(revisionPath(path))
	return revision, nil
}

// Clear removes the workspace goal.
func Clear(workspace string) error {
	path, err := storePath(workspace)
	if err != nil {
		return err
	}
	unlock, err := acquireGoalLock(path)
	if err != nil {
		return err
	}
	defer unlock()
	state, err := loadLocked(path)
	if err != nil {
		return err
	}
	if state.Revision == 0 {
		if legacyRevision, revisionErr := readRevision(path); revisionErr != nil {
			return revisionErr
		} else if legacyRevision > 0 {
			state.Revision = legacyRevision
		}
	}
	if state.Revision > 0 {
		if err := writeAtomic(path, encodeState(State{Revision: state.Revision})); err != nil {
			return err
		}
	}
	_ = os.Remove(revisionPath(path))
	return nil
}

func clearIfState(workspace, expected string, expectedRevision *uint64) (bool, error) {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return false, nil
	}
	path, err := readPath(workspace)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			if _, backupErr := os.Stat(stateBackupPath(path)); os.IsNotExist(backupErr) {
				return false, nil
			}
		} else {
			return false, err
		}
	}
	unlock, err := acquireGoalLock(path)
	if err != nil {
		return false, err
	}
	defer unlock()
	state, err := loadLocked(path)
	if err != nil {
		return false, err
	}
	if state.Text != expected || (expectedRevision != nil && state.Revision != *expectedRevision) {
		return false, nil
	}
	if state.Revision > 0 {
		if err := writeAtomic(path, encodeState(State{Revision: state.Revision})); err != nil {
			return false, err
		}
	}
	_ = os.Remove(revisionPath(path))
	return true, nil
}

// ClearIfState removes the workspace goal only when both its text and durable
// revision match expected. Revision zero is a real identity, not a wildcard.
func ClearIfState(workspace, expected string, expectedRevision uint64) (bool, error) {
	return clearIfState(workspace, expected, &expectedRevision)
}

// ClearIf removes the workspace goal only when it still matches expected.
// A completed turn must not erase a newer goal that was set while it was
// running. It reports whether the expected goal was removed.
func ClearIf(workspace, expected string) (bool, error) {
	return clearIfState(workspace, expected, nil)
}

// WorkPrompt kicks off agent work on a newly set goal.
func WorkPrompt(text string) string {
	return `Active goal (stays until done):

` + text + `

Work toward this now with tools. Keep going until it is fully met or you are blocked.
When complete, start your reply with "Goal complete:" and summarize what was done.`
}

// PromptSuffix is injected into the system prompt while a goal is active.
func PromptSuffix(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return `

Active goal (keep working until fully complete; then start with "Goal complete:"):
` + text
}

// LooksComplete reports whether the assistant marked the goal done.
func LooksComplete(text string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	return strings.HasPrefix(t, "goal complete:") || strings.HasPrefix(t, "goal complete —")
}
