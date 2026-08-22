package taskstate

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestStoreRoundTripDeleteAndPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "tasks")
	store := NewStore(dir)
	task, err := New("session-42", "fix signup", []string{"reproduce", "patch", "verify"})
	if err != nil {
		t.Fatal(err)
	}
	task.Status = StatusWorking
	task.NoteAttempt()
	task.AddChangedFiles("internal/signup.go")
	task.AddVerification("go test ./internal/signup", true, "ok")
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}
	path, err := store.Path(task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); runtime.GOOS != "windows" && got != 0o600 {
		t.Fatalf("file mode = %o", got)
	}
	loaded, err := store.Load(task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(task, loaded) {
		t.Fatalf("round trip mismatch:\nwant %+v\n got %+v", task, loaded)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || strings.HasSuffix(entries[0].Name(), ".tmp") {
		t.Fatalf("atomic save residue: %+v", entries)
	}
	if err := store.Delete(task.SessionID); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(task.SessionID); err != nil {
		t.Fatalf("delete should be idempotent: %v", err)
	}
	if _, err := store.Load(task.SessionID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("load missing = %v", err)
	}
}

func TestWorkspaceStorePathAndSessionSafety(t *testing.T) {
	root := t.TempDir()
	store := WorkspaceStore(root)
	path, err := store.Path("abc_123-DEF")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, ".picogent", "tasks", "abc_123-DEF.json")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	for _, id := range []string{"", ".", "..", "../escape", "a/b", "a b", strings.Repeat("a", 201)} {
		if _, err := store.Path(id); err == nil {
			t.Fatalf("unsafe id %q accepted", id)
		}
	}
}

func TestStoreInferAndSaveOnlyForTasks(t *testing.T) {
	store := NewStore(t.TempDir())
	task, ok, err := store.InferAndSave("s1", "fix the failing signup tests")
	if err != nil || !ok || task == nil {
		t.Fatalf("task=%+v ok=%v err=%v", task, ok, err)
	}
	if _, err := store.Load("s1"); err != nil {
		t.Fatal(err)
	}
	none, ok, err := store.InferAndSave("s2", "what does signup do?")
	if err != nil || ok || none != nil {
		t.Fatalf("non-task=%+v ok=%v err=%v", none, ok, err)
	}
	if _, err := store.Load("s2"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("non-task persisted: %v", err)
	}
}

func TestStoreRejectsCorruptUnknownAndMismatchedState(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	write := func(session, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, session+".json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("corrupt", "{")
	if _, err := store.Load("corrupt"); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("corrupt load = %v", err)
	}
	write("unknown", `{"version":1,"mystery":true}`)
	if _, err := store.Load("unknown"); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown load = %v", err)
	}
	write("trailing", `{} {}`)
	if _, err := store.Load("trailing"); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("trailing load = %v", err)
	}
	task, err := New("other", "goal", nil)
	if err != nil {
		t.Fatal(err)
	}
	data := `{"version":1,"id":"` + task.ID + `","session_id":"other","goal":"goal","status":"working","current_step":0,"attempts":0,"created_at":"` + task.CreatedAt.Format("2006-01-02T15:04:05.999999999Z") + `","updated_at":"` + task.UpdatedAt.Format("2006-01-02T15:04:05.999999999Z") + `"}`
	write("expected", data)
	if _, err := store.Load("expected"); err == nil || !strings.Contains(err.Error(), "session mismatch") {
		t.Fatalf("mismatch load = %v", err)
	}
}

func TestStoreRejectsInvalidTask(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.Save(nil); err == nil {
		t.Fatal("nil task should fail")
	}
	task, err := New("bad/session", "goal", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(task); err == nil {
		t.Fatal("unsafe session id should fail")
	}
	if err := NewStore("").Save(task); err == nil {
		t.Fatal("empty store should fail")
	}
}
