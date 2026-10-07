package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/session"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
)

func recoveryServer(t *testing.T) *server {
	t.Helper()
	t.Setenv("PICOGENT_HOME", t.TempDir())
	return &server{
		cfg:       config.Config{Workspace: t.TempDir()},
		sessionID: "current-chat",
		hist:      []llm.Message{{Role: "user", Content: "preserve this chat"}},
	}
}

func recoveryAction(s *server, action, id, undoID string) *httptest.ResponseRecorder {
	data, _ := json.Marshal(map[string]string{"action": action, "id": id, "undo_id": undoID})
	res := httptest.NewRecorder()
	s.sessions(res, loopbackAPIRequest(http.MethodPost, "/api/sessions", string(data)))
	return res
}

func recoveryResponse(t *testing.T, res *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	var data map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func saveRecoveryChat(t *testing.T, id, workspace string) {
	t.Helper()
	if err := session.SaveMessages(workspace, id, []llm.Message{{Role: "user", Content: "chat " + id}}); err != nil {
		t.Fatal(err)
	}
}

func TestSessionsRecoveryCurrentDeleteReloadRestore(t *testing.T) {
	s := recoveryServer(t)
	data := recoveryResponse(t, recoveryAction(s, "delete", s.sessionID, ""))
	undoID, ok := data["undo_id"].(string)
	if !ok || undoID == "" || data["was_current"] != true || data["current_id"] == "current-chat" {
		t.Fatalf("delete response=%#v", data)
	}
	if _, err := session.Load("current-chat"); !os.IsNotExist(err) {
		t.Fatalf("deleted chat still exists: %v", err)
	}
	// No process cache or startup hook: a new server reads the durable journal.
	restarted := &server{cfg: s.cfg, sessionID: "after-restart"}
	list := httptest.NewRecorder()
	restarted.sessions(list, loopbackAPIRequest(http.MethodGet, "/api/sessions", ""))
	state := recoveryResponse(t, list)
	undo, ok := state["delete_undo"].(map[string]any)
	if !ok || undo["undo_id"] != undoID || undo["id"] != "current-chat" {
		t.Fatalf("reload recovery=%#v", state)
	}
	if _, leaked := undo["messages"]; leaked {
		t.Fatal("deleted transcript leaked into recovery projection")
	}
	recoveryResponse(t, recoveryAction(restarted, "undo_delete", "", undoID))
	chat, err := session.Load("current-chat")
	if err != nil || len(chat.Messages) != 1 || chat.Messages[0].Content != "preserve this chat" {
		t.Fatalf("restored chat=%#v err=%v", chat, err)
	}
	if restarted.sessionID != "after-restart" {
		t.Fatal("restore replaced an unrelated active chat")
	}
	if _, err := session.LoadDeleteUndo(); !os.IsNotExist(err) {
		t.Fatalf("recovery journal remained after restore: %v", err)
	}
}

func TestSessionsRecoveryRejectsOtherWorkspace(t *testing.T) {
	s := recoveryServer(t)
	other := t.TempDir()
	saveRecoveryChat(t, "foreign-chat", other)
	res := recoveryAction(s, "delete", "foreign-chat", "")
	if res.Code != http.StatusForbidden {
		t.Fatalf("foreign delete status=%d", res.Code)
	}
	if _, err := session.Load("foreign-chat"); err != nil {
		t.Fatal("foreign chat was deleted")
	}
	data := recoveryResponse(t, recoveryAction(s, "delete", "current-chat", ""))
	s.cfg.Workspace = other
	if got := s.chatDeleteUndoProjection(other); got != nil {
		t.Fatalf("foreign recovery exposed: %#v", got)
	}
	res = recoveryAction(s, "undo_delete", "", data["undo_id"].(string))
	if res.Code != http.StatusForbidden {
		t.Fatalf("foreign restore status=%d", res.Code)
	}
	if _, err := session.LoadDeleteUndo(); err != nil {
		t.Fatal("foreign restore retired recovery")
	}
}

func TestSessionsRecoveryOldTokenCannotRestoreOrClearNewDelete(t *testing.T) {
	s := recoveryServer(t)
	first := recoveryResponse(t, recoveryAction(s, "delete", "current-chat", ""))
	saveRecoveryChat(t, "second-chat", s.cfg.Workspace)
	second := recoveryResponse(t, recoveryAction(s, "delete", "second-chat", ""))
	res := recoveryAction(s, "undo_delete", "", first["undo_id"].(string))
	if res.Code != http.StatusConflict {
		t.Fatalf("old restore status=%d", res.Code)
	}
	current, err := session.LoadDeleteUndo()
	if err != nil || current.UndoID != second["undo_id"] {
		t.Fatalf("old token changed newer recovery: %#v err=%v", current, err)
	}
	recoveryResponse(t, recoveryAction(s, "undo_delete", "", second["undo_id"].(string)))
}

func TestSessionsRecoveryFailedStagePreservesCurrentAndEarlierUndo(t *testing.T) {
	s := recoveryServer(t)
	saveRecoveryChat(t, "earlier-chat", s.cfg.Workspace)
	earlier := recoveryResponse(t, recoveryAction(s, "delete", "earlier-chat", ""))
	dir, err := session.Dir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".delete-undo.stage"), 0o700); err != nil {
		t.Fatal(err)
	}
	res := recoveryAction(s, "delete", "current-chat", "")
	if res.Code != http.StatusInternalServerError || s.sessionID != "current-chat" || s.sessionTransition {
		t.Fatalf("failed delete status=%d current=%s transition=%v", res.Code, s.sessionID, s.sessionTransition)
	}
	if _, err := session.Load("current-chat"); err != nil {
		t.Fatalf("failed delete lost current chat: %v", err)
	}
	// Remove only this test's blocking empty directory to inspect the journal.
	if err := os.Remove(filepath.Join(dir, ".delete-undo.stage")); err != nil {
		t.Fatal(err)
	}
	current, err := session.LoadDeleteUndo()
	if err != nil || current.UndoID != earlier["undo_id"] {
		t.Fatalf("failed delete replaced earlier recovery: %#v err=%v", current, err)
	}
}

func TestSessionsRecoveryFailedReplacementBindingLeavesChatAndUndo(t *testing.T) {
	s := recoveryServer(t)
	saveRecoveryChat(t, "earlier-chat", s.cfg.Workspace)
	earlier := recoveryResponse(t, recoveryAction(s, "delete", "earlier-chat", ""))
	cfg := config.Default()
	cfg.Workspace = s.cfg.Workspace
	cfg.Provider = config.ProviderOllama
	ag := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: cfg.Workspace}), perm.New(config.ModeSafe, cfg.Workspace, nil))
	defer ag.Close()
	blocked := filepath.Join(t.TempDir(), "blocked-store")
	if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	ag.SetTaskStore(taskstate.NewStore(blocked))
	s.ag = ag
	res := recoveryAction(s, "delete", "current-chat", "")
	if res.Code != http.StatusInternalServerError || s.sessionID != "current-chat" || s.sessionTransition {
		t.Fatalf("failed binding status=%d current=%s", res.Code, s.sessionID)
	}
	if _, err := session.Load("current-chat"); err != nil {
		t.Fatalf("failed binding lost current chat: %v", err)
	}
	current, err := session.LoadDeleteUndo()
	if err != nil || current.UndoID != earlier["undo_id"] {
		t.Fatalf("failed binding replaced earlier recovery: %#v err=%v", current, err)
	}
}

func TestSessionsRecoveryConcurrentDeleteCommitsOnce(t *testing.T) {
	s := recoveryServer(t)
	var wg sync.WaitGroup
	results := make(chan *httptest.ResponseRecorder, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- recoveryAction(s, "delete", "current-chat", "")
		}()
	}
	wg.Wait()
	close(results)
	counts := map[int]int{}
	for res := range results {
		counts[res.Code]++
	}
	if counts[http.StatusOK] != 1 || counts[http.StatusNotFound] != 1 {
		t.Fatalf("duplicate delete statuses=%v", counts)
	}
	undo, err := session.LoadDeleteUndo()
	if err != nil || undo.Session.ID != "current-chat" {
		t.Fatalf("duplicate delete lost recovery: %#v err=%v", undo, err)
	}
}

func TestSessionsRecoveryExpiredJournalCannotRestore(t *testing.T) {
	s := recoveryServer(t)
	saveRecoveryChat(t, "expired-chat", s.cfg.Workspace)
	chat, err := session.Load("expired-chat")
	if err != nil {
		t.Fatal(err)
	}
	undo := &session.DeleteUndo{UndoID: "expired-token", Session: chat, Workspace: s.cfg.Workspace, ExpiresAt: time.Now().UTC().Add(-time.Second)}
	if err := session.DeleteWithUndo(chat.ID, undo); err != nil {
		t.Fatal(err)
	}
	if err := session.CommitDeleteUndo(undo.UndoID); err != nil {
		t.Fatal(err)
	}
	res := recoveryAction(s, "undo_delete", "", undo.UndoID)
	if res.Code != http.StatusGone || s.chatDeleteUndoProjection(s.cfg.Workspace) != nil {
		t.Fatalf("expired recovery remained: status=%d", res.Code)
	}
	if _, err := session.Load(chat.ID); !os.IsNotExist(err) {
		t.Fatalf("expired recovery restored a chat: %v", err)
	}
}
