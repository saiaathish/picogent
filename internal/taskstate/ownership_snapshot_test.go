package taskstate

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func TestOwnershipSnapshotDoesNotNormalizeOrTrustCompletion(t *testing.T) {
	store := NewStore(t.TempDir())
	task, err := New("ownership-snapshot", "finish safely", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.Status = StatusDone // simulate a persisted, untrusted terminal marker
	data, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	path, err := store.Path(task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		current, err := store.OwnershipSnapshot(task.SessionID)
		if err != nil || current.Status != StatusDone || current.Revision != task.Revision || current.CompletionReady() {
			t.Fatalf("ownership read rewrote or trusted completion: task=%+v err=%v", current, err)
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(data, after) {
			t.Fatalf("ownership read changed durable bytes: %v", err)
		}
	}
	loaded, err := store.Load(task.SessionID)
	if err != nil || loaded.Status != StatusWorking || loaded.Revision != task.Revision+1 {
		t.Fatalf("ordinary recovery loader lost normalization: task=%+v err=%v", loaded, err)
	}
}
