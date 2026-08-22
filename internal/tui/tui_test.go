package tui

import "testing"

func TestAssistantFinalReplacesStreamedText(t *testing.T) {
	m := &model{lines: []logLine{{Kind: "assistant", Text: "Undo: git checkout -- note.txt"}}}
	_, _ = m.Update(logMsg{Kind: "assistant_final", Text: "Undo: /undo"})
	if len(m.lines) != 1 || m.lines[0].Text != "Undo: /undo" {
		t.Fatalf("lines = %#v", m.lines)
	}
}
