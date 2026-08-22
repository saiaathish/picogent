package gui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/taskstate"
)

func TestTaskProgressClearEventKeepsNullTaskEnvelope(t *testing.T) {
	raw, err := json.Marshal(event{Type: "task_progress", SessionID: "session-current"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"task":null`) {
		t.Fatalf("clear event = %s", raw)
	}
}

func TestChatRejectsUndoWhileAgentTurnIsActive(t *testing.T) {
	tests := []struct {
		name        string
		busy        bool
		activeTurns int
	}{
		{name: "busy", busy: true, activeTurns: 1},
		{name: "cancelled but still exiting", activeTurns: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := make(chan event, 1)
			s := &server{
				busy:        tt.busy,
				activeTurns: tt.activeTurns,
				subs:        []chan event{events},
			}
			req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"prompt":"/undo"}`))
			res := httptest.NewRecorder()

			s.chat(res, req)

			if res.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d", res.Code, http.StatusNoContent)
			}
			select {
			case got := <-events:
				if got.Type != "error" {
					t.Fatalf("event type = %q, want error", got.Type)
				}
				if !strings.Contains(got.Text, "cannot undo while a turn is running") {
					t.Fatalf("event text = %q, want active-turn explanation", got.Text)
				}
			default:
				t.Fatal("expected active-turn error event")
			}
		})
	}
}

func TestGUIHandlerEmitsCanonicalFinalTextReplacement(t *testing.T) {
	events := make(chan event, 1)
	s := &server{subs: []chan event{events}}
	h := &guiHandler{s: s}
	h.OnTextFinal("Undo: /undo")

	got := <-events
	if got.Type != "assistant_final" || got.Text != "Undo: /undo" {
		t.Fatalf("event = %+v", got)
	}
}

func TestSnapshotIncludesDurableTaskForCurrentSession(t *testing.T) {
	store := taskstate.NewStore(t.TempDir())
	task, err := taskstate.New("session-current", "finish the loop", []string{"implement", "verify"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}
	ag := &agent.Agent{TaskStore: store}
	ag.SetTaskSession("session-current")
	s := &server{ag: ag, sessionID: "session-current"}

	got, ok := s.snapshot()["task"].(*taskstate.Task)
	if !ok || got == nil || got.ID != task.ID {
		t.Fatalf("snapshot task = %#v", s.snapshot()["task"])
	}
}

func TestGUIHandlerDropsStaleTaskProgress(t *testing.T) {
	events := make(chan event, 1)
	s := &server{subs: []chan event{events}, sessionID: "session-live", turnGen: 2}
	task, err := taskstate.New("session-live", "finish the loop", []string{"implement"})
	if err != nil {
		t.Fatal(err)
	}
	stale := &guiHandler{s: s, sessionID: "session-live", turnGen: 1}
	stale.OnTaskState(task)
	select {
	case got := <-events:
		t.Fatalf("stale handler emitted %#v", got)
	default:
	}

	live := &guiHandler{s: s, sessionID: "session-live", turnGen: 2}
	live.OnTaskState(task)
	got := <-events
	if got.Type != "task_progress" || got.SessionID != "session-live" || got.Task == nil || got.Task.ID != task.ID {
		t.Fatalf("event = %#v", got)
	}
}
