package session

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/llm"
)

func latestUndoFixture(t *testing.T, id string) *Session {
	t.Helper()
	s := &Session{ID: id, Workspace: t.TempDir(), Messages: []llm.Message{{Role: "user", Content: "before concurrent save"}}}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDeleteWithUndoJournalsLatestAcknowledgedTranscript(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	original := latestUndoFixture(t, "latest-delete")
	stale, err := Load(original.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Another process's acknowledged save between GUI read and deletion.
	original.Title = "latest title"
	original.Messages = append(original.Messages, llm.Message{Role: "assistant", Content: "acknowledged newer message"})
	if err := original.Save(); err != nil {
		t.Fatal(err)
	}
	undo := &DeleteUndo{UndoID: "latest-token", Session: stale, Workspace: stale.Workspace, ExpiresAt: time.Now().UTC().Add(time.Minute)}
	if err := DeleteWithUndo(original.ID, undo); err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreDeleteUndo(undo.UndoID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Session.Title != "latest title" || len(restored.Session.Messages) != 2 ||
		restored.Session.Messages[1].Content != "acknowledged newer message" {
		t.Fatalf("undo lost acknowledged concurrent save: %#v", restored.Session)
	}
}

func TestDeleteWithUndoRejectsConcurrentWorkspaceChange(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	original := latestUndoFixture(t, "moved-delete")
	stale, err := Load(original.ID)
	if err != nil {
		t.Fatal(err)
	}
	original.Workspace = t.TempDir()
	if err := original.Save(); err != nil {
		t.Fatal(err)
	}
	undo := &DeleteUndo{UndoID: "stale-workspace-token", Session: stale, Workspace: stale.Workspace, ExpiresAt: time.Now().UTC().Add(time.Minute)}
	if err := DeleteWithUndo(original.ID, undo); err == nil {
		t.Fatal("stale workspace was allowed to delete the moved session")
	}
	if _, err := Load(original.ID); err != nil {
		t.Fatalf("rejected deletion lost session: %v", err)
	}
	if _, err := LoadDeleteUndo(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected deletion created recovery: %v", err)
	}
}

func TestDeleteWithUndoRetiresExpiredCrashStageWithoutPriorRead(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	first := latestUndoFixture(t, "expired-stage")
	expired := &DeleteUndo{UndoID: "expired-stage-token", Session: first, Workspace: first.Workspace, ExpiresAt: time.Now().UTC().Add(-time.Second)}
	if err := DeleteWithUndo(first.ID, expired); err != nil {
		t.Fatal(err)
	}
	// No commit and no journal read: simulate a crash and a later API delete.
	second := latestUndoFixture(t, "after-expired-stage")
	next := &DeleteUndo{UndoID: "next-stage-token", Session: second, Workspace: second.Workspace, ExpiresAt: time.Now().UTC().Add(time.Minute)}
	if err := DeleteWithUndo(second.ID, next); err != nil {
		t.Fatalf("expired crash stage blocked new deletion: %v", err)
	}
	if _, err := RestoreDeleteUndo(next.UndoID); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDeleteUndo(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired recovery reappeared: %v", err)
	}
}
