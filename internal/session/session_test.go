package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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

func TestSessionDeleteRejectsSymlinkTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	t.Setenv("PICOGENT_HOME", t.TempDir())
	s := New(t.TempDir())
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	path, err := s.Path()
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte("must survive"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}

	if err := Delete(s.ID); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("Delete symlink = %v, want symbolic-link rejection", err)
	}
	if got, err := os.ReadFile(outside); err != nil || string(got) != "must survive" {
		t.Fatalf("outside target after Delete = %q, %v", got, err)
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("session symlink was removed: %v", err)
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

func TestListMetaDerivesTitleForLegacySessionWithoutStoredTitle(t *testing.T) {
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
	legacy := Session{
		ID:        "legacy-title",
		Workspace: workspace,
		Updated:   time.Now().UTC(),
		Messages: []llm.Message{
			{Role: "user", Content: "resume the legacy task"},
			{Role: "assistant", Content: "I found the checkpoint."},
		},
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, legacy.ID+".json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	metas, err := ListMeta(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 1 || metas[0].ID != legacy.ID || metas[0].Title != "resume the legacy task" {
		t.Fatalf("legacy session metadata = %#v", metas)
	}
}

func TestListMetaRejectsInvalidMessageHistoryShapes(t *testing.T) {
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

	type record struct {
		ID        string          `json:"id"`
		Title     string          `json:"title"`
		Workspace string          `json:"workspace"`
		Updated   time.Time       `json:"updated"`
		Messages  json.RawMessage `json:"messages"`
	}
	shapes := []string{"{}", `"not a message array"`, `["not a message"]`}
	for i, shape := range shapes {
		s := record{
			ID:        fmt.Sprintf("invalid-messages-%d", i),
			Title:     "invalid",
			Workspace: workspace,
			Updated:   time.Now().UTC(),
			Messages:  json.RawMessage(shape),
		}
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, s.ID+".json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	valid := record{
		ID:        "valid-messages",
		Title:     "valid",
		Workspace: workspace,
		Updated:   time.Now().UTC(),
		Messages:  json.RawMessage(`[]`),
	}
	data, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, valid.ID+".json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	metas, err := ListMeta(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 1 || metas[0].ID != valid.ID {
		t.Fatalf("metadata for invalid message histories = %#v", metas)
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

func TestSessionSaveBoundsSerializedHistoryAndKeepsNewestToolExchange(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	id := "bounded-session"
	messages := make([]llm.Message, 0, MaxSessionMessages+40)
	for i := 0; i < 180; i++ {
		messages = append(messages,
			llm.Message{Role: "user", Content: fmt.Sprintf("request %03d %s", i, strings.Repeat("u", 12000))},
			llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: fmt.Sprintf("call-%03d", i), Name: "read_file", Arguments: `{"path":"file.txt"}`}}},
			llm.Message{Role: "tool", ToolCallID: fmt.Sprintf("call-%03d", i), Name: "read_file", Content: strings.Repeat("tool output ", 1200)},
			llm.Message{Role: "assistant", Content: fmt.Sprintf("response %03d", i)},
		)
	}
	if err := SaveMessages(workspace, id, messages); err != nil {
		t.Fatal(err)
	}
	s, err := Load(id)
	if err != nil {
		t.Fatal(err)
	}
	path, err := s.Path()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > MaxSessionBytes {
		t.Fatalf("session size=%d, want <= %d", info.Size(), MaxSessionBytes)
	}
	if len(s.Messages) == 0 || len(s.Messages) > MaxSessionMessages {
		t.Fatalf("bounded message count=%d", len(s.Messages))
	}
	if got := s.Messages[len(s.Messages)-1].Content; got != "response 179" {
		t.Fatalf("newest response=%q", got)
	}
	for i, message := range s.Messages {
		if message.Role != "tool" {
			continue
		}
		if i == 0 || s.Messages[i-1].Role != "assistant" || len(s.Messages[i-1].ToolCalls) == 0 || s.Messages[i-1].ToolCalls[0].ID != message.ToolCallID {
			t.Fatalf("orphaned tool exchange at message %d: %#v", i, s.Messages[i-1:])
		}
	}
}

func TestSessionLoadRejectsOversizedRecordBeforeParsing(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	s := New(t.TempDir())
	path, err := s.Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("x", MaxSessionBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(s.ID); !errors.Is(err, ErrSessionTooLarge) {
		t.Fatalf("Load oversized record=%v, want %v", err, ErrSessionTooLarge)
	}
}

func TestSessionLoadBoundsLegacyHistoryBeforeResume(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	s := New(workspace)
	for i := 0; i < MaxSessionMessages+20; i++ {
		s.Messages = append(s.Messages,
			llm.Message{Role: "user", Content: fmt.Sprintf("request %03d", i)},
			llm.Message{Role: "assistant", Content: fmt.Sprintf("response %03d", i)},
		)
	}
	path, err := s.Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Messages) > MaxSessionMessages {
		t.Fatalf("resumed message count=%d, want <= %d", len(loaded.Messages), MaxSessionMessages)
	}
	if got := loaded.Messages[len(loaded.Messages)-1].Content; got != "response 147" {
		t.Fatalf("resumed newest response=%q", got)
	}
}

func TestSessionLoadRedactsHandWrittenTranscript(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	const secret = "handwritten-load-secret"
	s := &Session{
		ID:        "handwritten-redaction",
		Title:     "manual record",
		Workspace: workspace,
		Updated:   time.Now().UTC(),
		Messages: []llm.Message{
			{Role: "user", Content: `api_key="` + secret + `"`},
			{Role: "assistant", Parts: []llm.Part{{Type: "text", Text: "password=" + secret}}},
			{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call-1", Name: "read_file", Arguments: `{"token":"` + secret + `"}`}}},
			{Role: "tool", ToolCallID: "call-1", Name: "read_file", Content: "Authorization: Bearer " + secret},
		},
	}
	path, err := s.Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Messages) != len(s.Messages) {
		t.Fatalf("loaded message count=%d, want %d", len(loaded.Messages), len(s.Messages))
	}
	for _, message := range loaded.Messages {
		if strings.Contains(message.Content, secret) {
			t.Fatalf("loaded content retained secret: %#v", message)
		}
		for _, part := range message.Parts {
			if strings.Contains(part.Text, secret) {
				t.Fatalf("loaded part retained secret: %#v", part)
			}
		}
		for _, call := range message.ToolCalls {
			if strings.Contains(call.Arguments, secret) {
				t.Fatalf("loaded tool arguments retained secret: %#v", call)
			}
		}
	}
}

func TestSessionNeedsNormalizationFailsClosed(t *testing.T) {
	canonical := Session{
		ID:        "canonical",
		Title:     "canonical session",
		Workspace: "workspace",
		Messages: []llm.Message{
			{Role: "user", Content: "request"},
			{Role: "assistant", Content: "response"},
		},
	}
	data, err := json.Marshal(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if sessionNeedsNormalization(&canonical, len(data)) {
		t.Fatal("canonical session took the normalization path")
	}

	tooMany := canonical
	tooMany.Messages = make([]llm.Message, MaxSessionMessages+1)
	withSystem := canonical
	withSystem.Messages = append([]llm.Message{{Role: "system", Content: "internal"}}, canonical.Messages...)
	withOrphan := canonical
	withOrphan.Messages = append(canonical.Messages, llm.Message{Role: "tool", ToolCallID: "missing"})
	withSecret := canonical
	withSecret.Messages = []llm.Message{{Role: "user", Content: `token="secret"`}}
	emptyArray := canonical
	emptyArray.Messages = []llm.Message{}
	cases := []struct {
		name string
		s    Session
		size int
	}{
		{name: "too many messages", s: tooMany, size: len(data)},
		{name: "system message", s: withSystem, size: len(data)},
		{name: "orphan tool message", s: withOrphan, size: len(data)},
		{name: "sensitive text", s: withSecret, size: len(data)},
		{name: "present empty array", s: emptyArray, size: len(data)},
		{name: "near byte limit", s: canonical, size: maxSessionFastPathBytes + 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !sessionNeedsNormalization(&tc.s, tc.size) {
				t.Fatalf("sessionNeedsNormalization(%s) = false", tc.name)
			}
		})
	}
}

func TestNewestUnitsPrioritizesNewestUnitWhenOldHistoryWouldCrowdItOut(t *testing.T) {
	base := Session{ID: "bounded", Title: "chat", Workspace: "workspace"}
	newest := strings.Repeat("newest response ", 12000)
	got := newestUnits([]llm.Message{
		{Role: "user", Content: strings.Repeat("old request ", 7500)},
		{Role: "assistant", Content: strings.Repeat("middle response ", 7500)},
		{Role: "assistant", Content: newest},
	}, nil, base)
	if len(got) != 1 {
		t.Fatalf("newest unit selection=%d messages: %#v", len(got), got)
	}
	if got[0].Content != newest {
		t.Fatalf("newest unit content=%q", got[0].Content)
	}
}

func TestSessionSaveDropsSystemAndOrphanedToolMessages(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	messages := []llm.Message{
		{Role: "system", Content: "internal prompt"},
		{Role: "user", Content: "request"},
		{Role: "tool", ToolCallID: "missing", Name: "read_file", Content: "orphan"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call", Name: "read_file", Arguments: `{}`}}},
		{Role: "tool", ToolCallID: "call", Name: "read_file", Content: "file contents"},
		{Role: "assistant", Content: "done"},
	}
	if err := SaveMessages(workspace, "normalized-session", messages); err != nil {
		t.Fatal(err)
	}
	s, err := Load("normalized-session")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Messages) != 4 {
		t.Fatalf("normalized message count=%d, messages=%#v", len(s.Messages), s.Messages)
	}
	if s.Messages[0].Role != "user" || s.Messages[1].Role != "assistant" || s.Messages[2].Role != "tool" || s.Messages[3].Role != "assistant" {
		t.Fatalf("normalized roles=%q, %q, %q, %q", s.Messages[0].Role, s.Messages[1].Role, s.Messages[2].Role, s.Messages[3].Role)
	}
	if s.Messages[2].ToolCallID != s.Messages[1].ToolCalls[0].ID {
		t.Fatalf("tool call linkage lost: assistant=%#v tool=%#v", s.Messages[1], s.Messages[2])
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
