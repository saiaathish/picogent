package session

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/securefile"
)

func TestDeleteUndoSurvivesTheDeleteToCommitCrashBoundary(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	original := &Session{
		ID:        "delete-undo-crash-boundary",
		Title:     "recover after restart",
		Workspace: workspace,
		Messages:  []llm.Message{{Role: "user", Content: "keep this durable chat"}},
	}
	if err := original.Save(); err != nil {
		t.Fatal(err)
	}
	pending := &DeleteUndo{
		UndoID:     "restart-undo-token",
		Session:    original,
		Workspace:  workspace,
		WasCurrent: true,
		ExpiresAt:  time.Now().UTC().Add(time.Minute),
	}
	if err := DeleteWithUndo(original.ID, pending); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(original.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted session load = %v, want not found", err)
	}

	// No commit models a process exit after the durable session removal but
	// before the GUI could publish its replacement chat. The staged record must
	// be enough for a fresh process to recover the exact session.
	loaded, err := LoadDeleteUndo()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.UndoID != pending.UndoID || loaded.Session.ID != original.ID || !loaded.WasCurrent {
		t.Fatalf("staged delete undo = %#v", loaded)
	}
	if len(loaded.Session.Messages) != 1 || loaded.Session.Messages[0].Content != "keep this durable chat" {
		t.Fatalf("staged delete undo messages = %#v", loaded.Session.Messages)
	}
	restored, err := RestoreDeleteUndo(loaded.UndoID)
	if err != nil {
		t.Fatal(err)
	}
	if restored == nil || restored.Session.ID != original.ID {
		t.Fatalf("restored delete undo = %#v", restored)
	}
	if _, err := LoadDeleteUndo(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cleared delete undo = %v, want not found", err)
	}
}

func TestDeleteUndoRejectsSecondStagedDeleteWithoutOverwritingFirst(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	first := &Session{ID: "first-staged-delete", Workspace: workspace, Messages: []llm.Message{{Role: "user", Content: "first"}}}
	second := &Session{ID: "second-staged-delete", Workspace: workspace, Messages: []llm.Message{{Role: "user", Content: "second"}}}
	if err := first.Save(); err != nil {
		t.Fatal(err)
	}
	if err := second.Save(); err != nil {
		t.Fatal(err)
	}
	firstUndo := &DeleteUndo{
		UndoID:    "first-staged-token",
		Session:   first,
		Workspace: workspace,
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	}
	if err := DeleteWithUndo(first.ID, firstUndo); err != nil {
		t.Fatal(err)
	}

	secondUndo := &DeleteUndo{
		UndoID:    "second-staged-token",
		Session:   second,
		Workspace: workspace,
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	}
	if err := DeleteWithUndo(second.ID, secondUndo); !errors.Is(err, ErrDeleteUndoInFlight) {
		t.Fatalf("second staged delete error = %v, want ErrDeleteUndoInFlight", err)
	}
	if _, err := Load(second.ID); err != nil {
		t.Fatalf("rejected second delete removed its session: %v", err)
	}
	loaded, err := LoadDeleteUndo()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.UndoID != firstUndo.UndoID || loaded.Session.ID != first.ID {
		t.Fatalf("first staged undo was overwritten: %#v", loaded)
	}
	if _, err := RestoreDeleteUndo(firstUndo.UndoID); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteUndoStageCleanupOnlyRemovesMatchingToken(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	staged := &DeleteUndo{
		UndoID:    "other-staged-token",
		Session:   &Session{ID: "other-staged-session", Workspace: workspace},
		Workspace: workspace,
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	}
	data, _, err := marshalDeleteUndo(staged)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := securefile.EnsureDir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := securefile.WriteAtomic(deleteUndoStagePath(dir), data, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := clearDeleteUndoStageIfCurrentLocked(dir, "caller-token"); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadDeleteUndo()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.UndoID != staged.UndoID {
		t.Fatalf("mismatched cleanup removed staged token: %#v", loaded)
	}
}

func TestDeleteUndoAbortPreservesEarlierCommittedUndo(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	first := &Session{ID: "first-delete-undo", Workspace: workspace, Messages: []llm.Message{{Role: "user", Content: "first"}}}
	if err := first.Save(); err != nil {
		t.Fatal(err)
	}
	firstUndo := &DeleteUndo{UndoID: "first-token", Session: first, Workspace: workspace, ExpiresAt: time.Now().UTC().Add(time.Minute)}
	if err := DeleteWithUndo(first.ID, firstUndo); err != nil {
		t.Fatal(err)
	}
	if err := CommitDeleteUndo(firstUndo.UndoID); err != nil {
		t.Fatal(err)
	}

	second := &Session{ID: "second-delete-undo", Workspace: workspace, Messages: []llm.Message{{Role: "user", Content: "second"}}}
	if err := second.Save(); err != nil {
		t.Fatal(err)
	}
	secondUndo := &DeleteUndo{UndoID: "second-token", Session: second, Workspace: workspace, ExpiresAt: time.Now().UTC().Add(time.Minute)}
	if err := DeleteWithUndo(second.ID, secondUndo); err != nil {
		t.Fatal(err)
	}
	if err := AbortDeleteUndo(secondUndo.UndoID); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadDeleteUndo()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.UndoID != firstUndo.UndoID || loaded.Session.ID != first.ID {
		t.Fatalf("earlier undo was not preserved: %#v", loaded)
	}
}

func TestDeleteUndoSurvivesFreshProcess(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	workspace := filepath.Join(root, "workspace")
	t.Setenv("PICOGENT_HOME", home)
	original := &Session{ID: "delete-undo-fresh-process", Workspace: workspace, Messages: []llm.Message{{Role: "user", Content: "reload from a fresh process"}}}
	if err := original.Save(); err != nil {
		t.Fatal(err)
	}
	pending := &DeleteUndo{UndoID: "fresh-process-token", Session: original, Workspace: workspace, ExpiresAt: time.Now().UTC().Add(time.Minute)}
	if err := DeleteWithUndo(original.ID, pending); err != nil {
		t.Fatal(err)
	}
	if err := CommitDeleteUndo(pending.UndoID); err != nil {
		t.Fatal(err)
	}

	result := filepath.Join(root, "fresh-process-result.json")
	cmd := exec.Command(os.Args[0], "-test.run", "^TestDeleteUndoFreshProcessHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(),
		"PICOGENT_DELETE_UNDO_HELPER=1",
		"PICOGENT_HOME="+home,
		"PICOGENT_DELETE_UNDO_RESULT="+result,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fresh-process delete undo load: %v\n%s", err, out)
	}
	data, err := os.ReadFile(result)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		UndoID string `json:"undo_id"`
		ID     string `json:"id"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.UndoID != pending.UndoID || got.ID != original.ID {
		t.Fatalf("fresh-process delete undo = %#v", got)
	}
}

func TestDeleteUndoFreshProcessHelper(t *testing.T) {
	if os.Getenv("PICOGENT_DELETE_UNDO_HELPER") != "1" {
		return
	}
	pending, err := LoadDeleteUndo()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(struct {
		UndoID string `json:"undo_id"`
		ID     string `json:"id"`
	}{UndoID: pending.UndoID, ID: pending.Session.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("PICOGENT_DELETE_UNDO_RESULT"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestExpiredDeleteUndoIsRetiredWithoutRestoringTheSession(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	original := &Session{ID: "expired-delete-undo", Workspace: workspace, Messages: []llm.Message{{Role: "user", Content: "expired"}}}
	if err := original.Save(); err != nil {
		t.Fatal(err)
	}
	pending := &DeleteUndo{UndoID: "expired-token", Session: original, Workspace: workspace, ExpiresAt: time.Now().UTC().Add(-time.Second)}
	if err := DeleteWithUndo(original.ID, pending); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDeleteUndo(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired delete undo = %v, want not found", err)
	}
	if _, err := Load(original.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired delete restored session = %v, want not found", err)
	}
}

func TestDeleteUndoConditionalOperationsPreserveNewerJournal(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	first := &Session{ID: "older-delete-undo", Workspace: workspace, Messages: []llm.Message{{Role: "user", Content: "older"}}}
	if err := first.Save(); err != nil {
		t.Fatal(err)
	}
	firstUndo := &DeleteUndo{UndoID: "older-token", Session: first, Workspace: workspace, ExpiresAt: time.Now().UTC().Add(time.Minute)}
	if err := DeleteWithUndo(first.ID, firstUndo); err != nil {
		t.Fatal(err)
	}
	if err := CommitDeleteUndo(firstUndo.UndoID); err != nil {
		t.Fatal(err)
	}

	second := &Session{ID: "newer-delete-undo", Workspace: workspace, Messages: []llm.Message{{Role: "user", Content: "newer"}}}
	if err := second.Save(); err != nil {
		t.Fatal(err)
	}
	secondUndo := &DeleteUndo{UndoID: "newer-token", Session: second, Workspace: workspace, ExpiresAt: time.Now().UTC().Add(time.Minute)}
	if err := DeleteWithUndo(second.ID, secondUndo); err != nil {
		t.Fatal(err)
	}
	if err := CommitDeleteUndo(secondUndo.UndoID); err != nil {
		t.Fatal(err)
	}

	cleared, err := ClearDeleteUndoIfCurrent(firstUndo.UndoID)
	if err != nil || cleared {
		t.Fatalf("clear stale undo = (%t, %v), want (false, nil)", cleared, err)
	}
	if restored, err := RestoreDeleteUndo(firstUndo.UndoID); !errors.Is(err, ErrDeleteUndoNotCurrent) || restored != nil {
		t.Fatalf("restore stale undo = (%#v, %v), want (nil, ErrDeleteUndoNotCurrent)", restored, err)
	}
	if _, err := Load(first.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale undo restored older session: %v", err)
	}
	pending, err := LoadDeleteUndo()
	if err != nil {
		t.Fatal(err)
	}
	if pending.UndoID != secondUndo.UndoID || pending.Session.ID != second.ID {
		t.Fatalf("newer undo was not preserved: %#v", pending)
	}

	restored, err := RestoreDeleteUndo(secondUndo.UndoID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Session.ID != second.ID {
		t.Fatalf("restored id = %q, want %q", restored.Session.ID, second.ID)
	}
	if _, err := LoadDeleteUndo(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("restored undo journal = %v, want not found", err)
	}
}
