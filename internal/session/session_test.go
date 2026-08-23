package session

import (
	"os"
	"strings"
	"testing"
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
