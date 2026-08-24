package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/llm"
)

func TestNewSessionIDsDoNotCollideWithinASecond(t *testing.T) {
	a := New(t.TempDir())
	b := New(t.TempDir())
	if a.ID == b.ID {
		t.Fatalf("session IDs collided: %q", a.ID)
	}
	if !validID(a.ID) || !validID(b.ID) {
		t.Fatalf("generated unsafe IDs: %q, %q", a.ID, b.ID)
	}
}

func TestSessionPathAndLoadRejectTraversal(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	for _, id := range []string{"", ".", "..", "../escape", "a/b", "a b"} {
		if _, err := Load(id); err == nil || !strings.Contains(err.Error(), "invalid session id") {
			t.Fatalf("Load(%q) = %v, want invalid id", id, err)
		}
		if err := Delete(id); err == nil || !strings.Contains(err.Error(), "invalid session id") {
			t.Fatalf("Delete(%q) = %v, want invalid id", id, err)
		}
	}
	if _, err := (&Session{ID: "../escape"}).Path(); err == nil {
		t.Fatal("Path accepted traversal ID")
	}
}

func TestSessionLoadRequiresStoredIDToMatchFilename(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	s := New(t.TempDir())
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	path, err := s.Path()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), `"id": "`+s.ID+`"`, `"id": "other"`, 1))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(s.ID); err == nil || !strings.Contains(err.Error(), "session id mismatch") {
		t.Fatalf("mismatched session load = %v", err)
	}
}

func TestPruneRemovesOldestSessionsBeyondBound(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PICOGENT_HOME", root)
	workspace := filepath.Join(root, "project")
	dir, err := Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC()
	for i := 0; i < MaxSessions+5; i++ {
		s := Session{
			ID:        fmt.Sprintf("session-%03d", i),
			Title:     fmt.Sprintf("session %d", i),
			Workspace: workspace,
			Updated:   base.Add(time.Duration(i) * time.Second),
		}
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, s.ID+".json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := Prune(workspace); err != nil {
		t.Fatal(err)
	}
	metas, err := ListMeta(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != MaxSessions {
		t.Fatalf("sessions after prune=%d, want %d", len(metas), MaxSessions)
	}
	if _, err := Load("session-000"); !os.IsNotExist(err) {
		t.Fatalf("oldest session load=%v, want not found", err)
	}
	if _, err := Load(fmt.Sprintf("session-%03d", MaxSessions+4)); err != nil {
		t.Fatalf("newest session was pruned: %v", err)
	}
}

func TestSessionSaveLeavesCompleteAtomicRecord(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	s := New(workspace)
	s.Messages = []llm.Message{{Role: "user", Content: "resume this"}, {Role: "assistant", Content: "saved"}}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	path, err := s.Path()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var loaded Session
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("saved record is not complete JSON: %v", err)
	}
	if len(loaded.Messages) != 2 || loaded.ID != s.ID {
		t.Fatalf("saved session=%+v", loaded)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("atomic save left temp file %q", entry.Name())
		}
	}
}

func TestConcurrentTitleAndHistoryUpdatesPreserveLatestHistory(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	id := "concurrent-session"
	initial := []llm.Message{{Role: "user", Content: "start"}}
	if err := SaveMessages(workspace, id, initial); err != nil {
		t.Fatal(err)
	}
	want := []llm.Message{{Role: "user", Content: "start"}, {Role: "assistant", Content: "continued"}}
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := SetTitle(id, fmt.Sprintf("title %d", i)); err != nil {
				t.Errorf("SetTitle: %v", err)
			}
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := SaveMessages(workspace, id, want); err != nil {
			t.Errorf("SaveMessages: %v", err)
		}
	}()
	wg.Wait()

	got, err := Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Messages) != len(want) || got.Messages[1].Content != "continued" {
		t.Fatalf("history lost during concurrent title updates: %+v", got.Messages)
	}
	if got.Title == "" || got.Title == "New chat" {
		t.Fatalf("title lost during concurrent update: %q", got.Title)
	}
}
