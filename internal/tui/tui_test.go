package tui

import (
	"testing"

	"github.com/saiaathish/picogent/internal/taskstate"
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
