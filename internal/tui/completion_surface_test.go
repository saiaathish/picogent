package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/saiaathish/picogent/internal/outcome"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/testsupport"
)

func TestTUIProjectsLongHorizonOutcomeStates(t *testing.T) {
	cases, err := testsupport.NewCompletionProjectionCases()
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			want := outcome.EvaluateCompletion(tc.Task)
			var emitted tea.Msg
			h := &handler{send: func(msg tea.Msg) { emitted = msg }}
			h.OnTaskState(tc.Task)

			progress, ok := emitted.(taskProgressMsg)
			if !ok {
				t.Fatalf("TUI task-progress message = %T, want taskProgressMsg", emitted)
			}
			if !reflect.DeepEqual(progress.completion, want) {
				t.Fatalf("TUI event proof = %#v, want durable proof %#v", progress.completion, want)
			}

			m := &model{
				sessionID: tc.Task.SessionID,
				turnID:    1,
				task:      &taskstate.Task{SessionID: tc.Task.SessionID},
				vp:        viewport.New(80, 20),
				lines:     []logLine{{Kind: "system", Text: "current"}},
			}
			_, _ = m.Update(progress)
			if m.task != tc.Task || !reflect.DeepEqual(m.completion, want) {
				t.Fatalf("TUI stored task/proof = %#v/%#v, want %#v/%#v", m.task, m.completion, tc.Task, want)
			}

			rendered := formatTaskProgressWithProof(m.task, m.completion)
			if rendered == "" {
				t.Fatal("TUI task progress was empty")
			}
			label := "proof pending"
			if tc.WantReady {
				label = "proof ready"
			}
			if !strings.Contains(rendered, label) {
				t.Fatalf("TUI task progress = %q, want %q", rendered, label)
			}
		})
	}
}
