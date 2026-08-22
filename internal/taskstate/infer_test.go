package taskstate

import (
	"strings"
	"testing"
)

func TestInferTaskLikePrompts(t *testing.T) {
	tests := []struct {
		prompt string
		step   string
	}{
		{"fix the failing signup tests and clean up whatever caused them", "reproduce"},
		{"Please implement durable resume support.", "Implement"},
		{"Can you fix the login crash?", "reproduce"},
		{"the checkout path is broken", "reproduce"},
		{"review the auth middleware", "contracts"},
		{"get this done", "current behavior"},
	}
	for _, tt := range tests {
		t.Run(tt.prompt, func(t *testing.T) {
			got := Infer(tt.prompt)
			if !got.TaskLike || got.Goal == "" || len(got.Steps) < 4 {
				t.Fatalf("not inferred: %+v", got)
			}
			joined := strings.Join(got.Steps, " ")
			if !strings.Contains(strings.ToLower(joined), strings.ToLower(tt.step)) {
				t.Fatalf("steps %q do not contain %q", joined, tt.step)
			}
		})
	}
}

func TestInferRejectsNonTasks(t *testing.T) {
	for _, prompt := range []string{
		"", "hello", "thanks", "what does the router do?", "how does this work?", "show me the architecture?", "what is a prefix?", "show fixtures?",
	} {
		t.Run(prompt, func(t *testing.T) {
			if got := Infer(prompt); got.TaskLike {
				t.Fatalf("unexpected task: %+v", got)
			}
		})
	}
}

func TestInferUnicodeGoal(t *testing.T) {
	got := Infer("édebug the crash")
	if !got.TaskLike || got.Goal != "Édebug the crash" {
		t.Fatalf("unicode goal = %+v", got)
	}
}

func TestNewFromPrompt(t *testing.T) {
	task, ok, err := NewFromPrompt("chat_42", "please fix the broken signup flow")
	if err != nil || !ok || task == nil {
		t.Fatalf("task=%+v ok=%v err=%v", task, ok, err)
	}
	if task.SessionID != "chat_42" || task.Goal != "Fix the broken signup flow" {
		t.Fatalf("unexpected task: %+v", task)
	}
	none, ok, err := NewFromPrompt("chat_42", "what is signup?")
	if err != nil || ok || none != nil {
		t.Fatalf("non-task = %+v %v %v", none, ok, err)
	}
	if _, ok, err := NewFromPrompt("", "fix signup"); err == nil || !ok {
		t.Fatalf("missing session should return inferred=true and error: ok=%v err=%v", ok, err)
	}
}
