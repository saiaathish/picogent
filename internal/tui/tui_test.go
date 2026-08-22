package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
)

func TestAssistantFinalReplacesStreamedText(t *testing.T) {
	m := &model{lines: []logLine{{Kind: "assistant", Text: "Undo: git checkout -- note.txt"}}}
	_, _ = m.Update(logMsg{Kind: "assistant_final", Text: "Undo: /undo"})
	if len(m.lines) != 1 || m.lines[0].Text != "Undo: /undo" {
		t.Fatalf("lines = %#v", m.lines)
	}
}

func TestFormatTaskProgress(t *testing.T) {
	tests := []struct {
		name string
		task *taskstate.Task
		want string
	}{
		{name: "none", want: ""},
		{
			name: "working",
			task: &taskstate.Task{
				Status:       taskstate.StatusWorking,
				Steps:        []taskstate.Step{{Description: "Map repository", Done: true}, {Description: "Implement UI"}},
				CurrentStep:  1,
				ChangedFiles: []string{"one.go"},
			},
			want: "task · working · 1/2 · Implement UI · 1 file",
		},
		{
			name: "blocked",
			task: &taskstate.Task{
				Status:       taskstate.StatusBlocked,
				Steps:        []taskstate.Step{{Description: "Implement UI"}},
				BlockedBy:    "verification repeatedly failed",
				ChangedFiles: []string{"one.go", "two.go"},
			},
			want: "task · blocked · 0/1 · blocked: verification repeatedly failed · 2 files",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatTaskProgress(tt.task); got != tt.want {
				t.Fatalf("formatTaskProgress() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStaleCanceledTurnEventsCannotMutateCurrentTurn(t *testing.T) {
	currentTask := &taskstate.Task{
		SessionID: "session-new",
		Status:    taskstate.StatusWorking,
		Goal:      "current task",
	}
	m := &model{
		sessionID: "session-new",
		turnID:    2,
		busy:      true,
		task:      currentTask,
		lines:     []logLine{{Kind: "system", Text: "current"}},
		h:         &handler{permCh: make(chan perm.Decision, 1)},
		vp:        viewport.New(80, 20),
	}

	_, _ = m.Update(logMsg{Kind: "assistant", Text: "stale text", turnID: 1, sessionID: "session-new"})
	_, _ = m.Update(taskProgressMsg{
		task:      &taskstate.Task{SessionID: "session-new", Goal: "stale task"},
		turnID:    1,
		sessionID: "session-new",
	})
	_, _ = m.Update(permAskMsg{
		Request:   perm.Request{Tool: "stale-tool", Summary: "stale permission"},
		turnID:    1,
		sessionID: "session-new",
	})
	_, _ = m.Update(doneMsg{
		history:   []llm.Message{{Role: "user", Content: "stale"}},
		turnID:    1,
		sessionID: "session-new",
	})
	_, _ = m.Update(evolveMsg{text: "stale reflection", turnID: 1, sessionID: "session-new"})

	if len(m.lines) != 1 || m.lines[0].Text != "current" {
		t.Fatalf("stale event changed lines: %#v", m.lines)
	}
	if m.task != currentTask {
		t.Fatalf("stale event replaced task: %#v", m.task)
	}
	if m.perm != nil {
		t.Fatalf("stale event opened permission prompt: %#v", m.perm)
	}
	if !m.busy {
		t.Fatal("stale done event cleared the active turn")
	}
	if len(m.history) != 0 {
		t.Fatalf("stale done event changed history: %#v", m.history)
	}
}

func TestStopInvalidatesActiveTurnID(t *testing.T) {
	stopCalled := false
	m := &model{
		turnID: 2,
		busy:   true,
		cancel: func() { stopCalled = true },
		h:      &handler{permCh: make(chan perm.Decision, 1)},
	}

	m.stop()

	if !stopCalled {
		t.Fatal("stop did not cancel the active turn")
	}
	if m.turnID != 3 {
		t.Fatalf("turn ID after stop = %d, want 3", m.turnID)
	}
	if m.busy {
		t.Fatal("stop left the model busy")
	}
}

func TestHandlerTagsEventsWithTurnAndSession(t *testing.T) {
	var got []tea.Msg
	h := &handler{
		send:      func(msg tea.Msg) { got = append(got, msg) },
		turnID:    9,
		sessionID: "session-9",
	}
	h.OnText("hello")
	h.OnTaskState(&taskstate.Task{SessionID: "session-9", Goal: "goal"})

	if len(got) != 2 {
		t.Fatalf("got %d events, want 2", len(got))
	}
	log, ok := got[0].(logMsg)
	if !ok || log.turnID != 9 || log.sessionID != "session-9" {
		t.Fatalf("log event metadata = %#v", got[0])
	}
	task, ok := got[1].(taskProgressMsg)
	if !ok || task.turnID != 9 || task.sessionID != "session-9" {
		t.Fatalf("task event metadata = %#v", got[1])
	}
}

func TestClearStartsNewSessionAndClearsDurableTask(t *testing.T) {
	testSessionReset(t, "clear")
}

func TestResetStartsNewSessionAndClearsDurableTask(t *testing.T) {
	testSessionReset(t, "reset")
}

func testSessionReset(t *testing.T, command string) {
	t.Helper()
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Mode = config.ModeFast
	a := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = taskstate.NewStore(t.TempDir())
	oldSession := "old-session"
	task, err := taskstate.New(oldSession, "finish the old task", []string{"work"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.TaskStore.Save(task); err != nil {
		t.Fatal(err)
	}
	a.SetTaskSession(oldSession)
	if a.TaskSnapshot() == nil {
		t.Fatal("test setup did not load the old durable task")
	}
	m := &model{
		cfg:       cfg,
		ag:        a,
		history:   []llm.Message{{Role: "user", Content: "old"}},
		sessionID: oldSession,
		lines:     []logLine{{Kind: "user", Text: "old"}},
		task:      a.TaskSnapshot(),
		h:         &handler{permCh: make(chan perm.Decision, 1)},
		vp:        viewport.New(80, 20),
	}

	if command == "clear" {
		m.slashLocal("clear")
	} else {
		m.slash("/reset")
	}

	if m.sessionID == oldSession {
		t.Fatalf("%s reused the old session ID %q", command, m.sessionID)
	}
	if m.history != nil {
		t.Fatalf("history after %s = %#v, want nil", command, m.history)
	}
	if a.TaskSession != m.sessionID {
		t.Fatalf("agent task session = %q, want %q", a.TaskSession, m.sessionID)
	}
	if a.TaskSnapshot() != nil {
		t.Fatalf("agent retained durable task after %s: %#v", command, a.TaskSnapshot())
	}
	if m.task != nil {
		t.Fatalf("model retained durable task after %s: %#v", command, m.task)
	}
	if len(m.lines) != 1 || m.lines[0].Text == "old" {
		t.Fatalf("lines after %s = %#v", command, m.lines)
	}
}
