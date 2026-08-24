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

func TestInferBuildsIntentContractAndDefinitionOfDone(t *testing.T) {
	got := Infer("finish this project")
	if !got.TaskLike || got.Intent == nil {
		t.Fatalf("intent not inferred: %+v", got)
	}
	if got.Intent.Completeness != "full" || got.Intent.Class != "general" || got.Intent.NeedsTests != true {
		t.Fatalf("unexpected intent: %+v", got.Intent)
	}
	if len(got.DefinitionOfDone) < 5 || len(got.Steps) != len(got.DefinitionOfDone) {
		t.Fatalf("definition of done = %#v steps=%#v", got.DefinitionOfDone, got.Steps)
	}
	joined := strings.ToLower(strings.Join(got.Steps, " "))
	if !strings.Contains(joined, "adjacent flows") {
		t.Fatalf("full-completeness criterion missing: %q", joined)
	}

	security := Infer("fix the auth permission bug")
	if security.Intent == nil || security.Intent.Class != "security" || security.Intent.Risk != "high" || !security.Intent.NeedsApproval {
		t.Fatalf("security intent = %+v", security.Intent)
	}

	ui := Infer("make the dashboard responsive")
	if ui.Intent == nil || ui.Intent.Class != "ui" || !ui.Intent.NeedsVisual {
		t.Fatalf("ui intent = %+v", ui.Intent)
	}
}

func TestNewFromPromptPersistsIntentContract(t *testing.T) {
	task, ok, err := NewFromPrompt("chat_42", "fix the broken signup flow")
	if err != nil || !ok || task == nil || task.Intent == nil {
		t.Fatalf("task=%+v ok=%v err=%v", task, ok, err)
	}
	if err := task.Validate(); err != nil {
		t.Fatalf("intent task invalid: %v", err)
	}
	if task.Intent.Outcome != task.Goal || len(task.DefinitionOfDone) != len(task.Steps) {
		t.Fatalf("task intent/criteria mismatch: %+v", task)
	}
}
