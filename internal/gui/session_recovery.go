package gui

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/session"
)

const chatDeleteUndoTTL = 30 * time.Second

// Recovery state lives in the session journal, not a second process cache.
// Reload/restart can discover it without exposing the deleted transcript.
func (s *server) chatDeleteUndoProjection(workspace string) map[string]any {
	undo, err := session.LoadDeleteUndo()
	if err != nil || !sessionWorkspaceMatches(undo.Session, workspace) {
		return nil
	}
	return map[string]any{
		"id":          undo.Session.ID,
		"title":       undo.Session.Title,
		"undo_id":     undo.UndoID,
		"expires_at":  undo.ExpiresAt,
		"was_current": undo.WasCurrent,
	}
}

// The caller holds configTxMu then sessionTxMu. Project/session changes cannot
// move the operation into another workspace while its disk writes are pending.
func (s *server) deleteChatWithRecovery(w http.ResponseWriter, id string) {
	id = strings.TrimSuffix(strings.TrimSpace(id), ".json")
	if id == "" {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	workspace := s.cfg.Workspace
	wasCurrent := s.sessionID == id
	src := s.ag
	var history []llm.Message
	if wasCurrent {
		s.sessionTransition = true
		s.abortTurnLocked()
		history = append([]llm.Message(nil), s.hist...)
	}
	s.mu.Unlock()
	if wasCurrent {
		defer func() {
			s.mu.Lock()
			s.sessionTransition = false
			s.mu.Unlock()
		}()
		if err := session.SaveMessages(workspace, id, history); err != nil {
			http.Error(w, "couldn't save chat before deletion", http.StatusInternalServerError)
			return
		}
	}
	deleted, err := session.Load(id)
	if err != nil {
		status := http.StatusInternalServerError
		if os.IsNotExist(err) {
			status = http.StatusNotFound
		}
		http.Error(w, "couldn't read chat before deletion", status)
		return
	}
	if !sessionWorkspaceMatches(deleted, workspace) {
		http.Error(w, "chat belongs to another project", http.StatusForbidden)
		return
	}
	var next *agent.Agent
	nextID := ""
	if wasCurrent {
		nextID = session.New(workspace).ID
		// Prepare before deleting. Failed replacement binding leaves both the
		// current chat and the previous recovery journal intact.
		next, err = cloneAgentForSession(src, nextID)
		if err != nil {
			http.Error(w, "couldn't prepare a new chat; nothing was deleted", http.StatusInternalServerError)
			return
		}
		if next != nil {
			next.SetTaskMode(agent.TaskAgent)
		}
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		http.Error(w, "couldn't prepare chat recovery", http.StatusInternalServerError)
		return
	}
	undo := &session.DeleteUndo{
		UndoID:     hex.EncodeToString(nonce[:]),
		Session:    deleted,
		Workspace:  workspace,
		WasCurrent: wasCurrent,
		ExpiresAt:  time.Now().UTC().Add(chatDeleteUndoTTL),
	}
	if err := session.DeleteWithUndo(id, undo); err != nil {
		http.Error(w, "couldn't delete chat; recovery was not confirmed", http.StatusInternalServerError)
		return
	}
	s.mu.Lock()
	if wasCurrent {
		s.sessionID = nextID
		s.hist = nil
		s.liveTask = agent.TaskAgent
		if next != nil {
			s.ag = next
		}
	}
	currentID := s.sessionID
	s.mu.Unlock()
	if err := session.CommitDeleteUndo(undo.UndoID); err != nil {
		// DeleteWithUndo already saved the stage. It remains restart-recoverable
		// even when journal promotion/retirement cannot finish immediately.
		s.emit(event{Type: "error", Text: "Chat deleted. Recovery is saved, but cleanup needs retrying."})
	}
	if wasCurrent {
		s.emit(event{Type: "undo", Status: "cleared"})
		s.emit(event{Type: "task_mode", Text: string(agent.TaskAgent)})
		s.invalidatePromptRecs()
		s.emit(event{Type: "prompts_refresh", Text: "all"})
	}
	s.emitTaskSnapshot(currentID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":              id,
		"current_id":      currentID,
		"was_current":     wasCurrent,
		"title":           deleted.Title,
		"undo_id":         undo.UndoID,
		"undo_expires_at": undo.ExpiresAt,
	})
}

func (s *server) restoreDeletedChat(w http.ResponseWriter, undoID string) {
	if strings.TrimSpace(undoID) == "" {
		http.Error(w, "undo_id required", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	workspace := s.cfg.Workspace
	currentID := s.sessionID
	s.mu.Unlock()
	pending, err := session.LoadDeleteUndo()
	if err != nil {
		http.Error(w, "chat recovery is no longer available", http.StatusGone)
		return
	}
	if !sessionWorkspaceMatches(pending.Session, workspace) {
		http.Error(w, "chat belongs to another project", http.StatusForbidden)
		return
	}
	restored, err := session.RestoreDeleteUndo(undoID)
	if err != nil && restored == nil {
		status := http.StatusInternalServerError
		if errors.Is(err, session.ErrDeleteUndoNotCurrent) || errors.Is(err, os.ErrExist) {
			status = http.StatusConflict
		} else if os.IsNotExist(err) {
			status = http.StatusGone
		}
		http.Error(w, "couldn't restore chat", status)
		return
	}
	if err != nil {
		// Restoration happened; a cleanup failure must not invite a second
		// restore or erase the fact that the chat is back.
		s.emit(event{Type: "error", Text: "Chat restored, but recovery cleanup needs retrying."})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":         restored.Session.ID,
		"title":      restored.Session.Title,
		"current_id": currentID,
	})
}
