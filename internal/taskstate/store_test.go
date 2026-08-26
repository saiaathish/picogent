package taskstate

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
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
	task.RecordChanged("internal/signup.go")
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
	if loaded.ChangeSeq != 1 || loaded.VerifiedChangeSeq != 1 || loaded.NeedsVerification() {
		t.Fatalf("round-tripped evidence = %+v", loaded)
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

func TestStoreRepeatedSaveReplacesExistingState(t *testing.T) {
	store := NewStore(t.TempDir())
	task, err := New("repeat-save", "keep progress", []string{"work"})
	if err != nil {
		t.Fatal(err)
	}
	for attempts := 0; attempts < 4; attempts++ {
		if attempts > 0 {
			task.NoteAttempt()
		}
		if err := store.Save(task); err != nil {
			t.Fatalf("save %d: %v", attempts, err)
		}
		loaded, err := store.Load(task.SessionID)
		if err != nil {
			t.Fatalf("load %d: %v", attempts, err)
		}
		if loaded.Attempts != attempts {
			t.Fatalf("attempts after save %d = %d, want %d", attempts, loaded.Attempts, attempts)
		}
	}
}

func TestStoreSaveUsesPersistedCompareAndSwapRevision(t *testing.T) {
	dir := t.TempDir()
	firstStore := NewStore(dir)
	secondStore := NewStore(dir)
	first, err := New("cas-session", "keep concurrent progress safe", []string{"work"})
	if err != nil {
		t.Fatal(err)
	}
	if err := firstStore.Save(first); err != nil {
		t.Fatal(err)
	}
	if first.Revision != 1 {
		t.Fatalf("initial revision = %d, want 1", first.Revision)
	}
	loaded, err := secondStore.Load(first.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Revision != first.Revision {
		t.Fatalf("loaded revision = %d, want %d", loaded.Revision, first.Revision)
	}
	first.NoteAttempt()
	if err := firstStore.SaveIfRevision(first, 1); err != nil {
		t.Fatal(err)
	}
	if first.Revision != 2 {
		t.Fatalf("updated revision = %d, want 2", first.Revision)
	}
	loaded.NoteAttempt()
	err = secondStore.SaveIfRevision(loaded, 1)
	if !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale save error = %v, want revision conflict", err)
	}
	if loaded.Revision != 1 {
		t.Fatalf("conflicted task mutated to revision %d", loaded.Revision)
	}
	final, err := firstStore.Load(first.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Revision != 2 || final.Attempts != first.Attempts {
		t.Fatalf("final state = revision %d attempts %d, want revision 2 attempts %d", final.Revision, final.Attempts, first.Attempts)
	}
}

func TestStoreLegacyZeroRevisionCanBeClaimedOnce(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	task, err := New("legacy-revision", "migrate once", nil)
	if err != nil {
		t.Fatal(err)
	}
	path, err := store.Path(task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	data := `{"version":1,"id":"` + task.ID + `","session_id":"legacy-revision","goal":"migrate once","status":"planning","current_step":0,"attempts":0,"created_at":"` + task.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00") + `","updated_at":"` + task.UpdatedAt.Format("2006-01-02T15:04:05.999999999Z07:00") + `"}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Revision != 0 {
		t.Fatalf("legacy revision = %d", loaded.Revision)
	}
	if err := store.SaveIfRevision(loaded, 0); err != nil {
		t.Fatal(err)
	}
	if loaded.Revision != 1 {
		t.Fatalf("migrated revision = %d, want 1", loaded.Revision)
	}
}

func TestStoreCrossProcessRejectsStaleRevision(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	task, err := New("cross-process", "reject stale writers", []string{"work"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}
	if task.Revision != 1 {
		t.Fatalf("initial revision = %d, want 1", task.Revision)
	}

	type child struct {
		cmd    *exec.Cmd
		ready  string
		result string
		output bytes.Buffer
	}
	children := make([]child, 2)
	defer func() {
		for i := range children {
			if children[i].cmd != nil && children[i].cmd.Process != nil {
				_ = children[i].cmd.Process.Kill()
				_ = children[i].cmd.Wait()
			}
		}
	}()
	for i := range children {
		children[i].ready = filepath.Join(dir, "ready-"+string(rune('a'+i)))
		children[i].result = filepath.Join(dir, "result-"+string(rune('a'+i)))
		goPath := filepath.Join(dir, "go")
		cmd := exec.Command(os.Args[0], "-test.run", "^TestStoreCrossProcessHelper$", "-test.count=1")
		cmd.Env = append(os.Environ(),
			"PICOGENT_TASKSTATE_HELPER=1",
			"PICOGENT_TASKSTATE_DIR="+dir,
			"PICOGENT_TASKSTATE_SESSION="+task.SessionID,
			"PICOGENT_TASKSTATE_READY="+children[i].ready,
			"PICOGENT_TASKSTATE_RESULT="+children[i].result,
			"PICOGENT_TASKSTATE_GO="+goPath,
		)
		cmd.Stdout = &children[i].output
		cmd.Stderr = &children[i].output
		children[i].cmd = cmd
		if err := cmd.Start(); err != nil {
			t.Fatalf("start child %d: %v", i, err)
		}
	}
	for i := range children {
		waitForTaskstateFile(t, children[i].ready)
	}
	goPath := filepath.Join(dir, "go")
	if err := os.WriteFile(goPath, []byte("go\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for i := range children {
		if err := children[i].cmd.Wait(); err != nil {
			t.Fatalf("child %d failed: %v\n%s", i, err, children[i].output.String())
		}
	}

	successes, conflicts := 0, 0
	for i := range children {
		data, err := os.ReadFile(children[i].result)
		if err != nil {
			t.Fatal(err)
		}
		switch strings.TrimSpace(string(data)) {
		case "success":
			successes++
		case "conflict":
			conflicts++
		default:
			t.Fatalf("child %d result = %q", i, data)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("cross-process results = successes %d conflicts %d", successes, conflicts)
	}
	final, err := store.Load(task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Revision != 2 || final.Attempts != 1 {
		t.Fatalf("final state = revision %d attempts %d, want revision 2 attempts 1", final.Revision, final.Attempts)
	}
}

func TestStoreCrossProcessHelper(t *testing.T) {
	if os.Getenv("PICOGENT_TASKSTATE_HELPER") != "1" {
		return
	}
	dir := os.Getenv("PICOGENT_TASKSTATE_DIR")
	session := os.Getenv("PICOGENT_TASKSTATE_SESSION")
	ready := os.Getenv("PICOGENT_TASKSTATE_READY")
	result := os.Getenv("PICOGENT_TASKSTATE_RESULT")
	goPath := os.Getenv("PICOGENT_TASKSTATE_GO")
	store := NewStore(dir)
	task, err := store.Load(session)
	if err != nil {
		t.Fatal(err)
	}
	task.NoteAttempt()
	if err := os.WriteFile(ready, []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err := os.Stat(goPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for parent release")
		}
		time.Sleep(10 * time.Millisecond)
	}
	err = store.SaveIfRevision(task, task.Revision)
	status := "success"
	if errors.Is(err, ErrRevisionConflict) {
		status = "conflict"
	} else if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(result, []byte(status+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func waitForTaskstateFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", path)
		}
		time.Sleep(10 * time.Millisecond)
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
