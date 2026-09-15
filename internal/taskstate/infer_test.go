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

	uiReview := Infer("review the browser UI visually")
	if uiReview.Intent == nil || uiReview.Intent.Class != "review" || !uiReview.Intent.NeedsVisual {
		t.Fatalf("UI review did not retain visual requirement: %+v", uiReview.Intent)
	}
}

func TestInferClassifiesBroadOutcomeRequests(t *testing.T) {
	for _, prompt := range []string{
		"Make this ready to launch",
		"make this launch-ready",
		"get this repo healthy",
		"make this good enough to ship",
	} {
		t.Run(prompt, func(t *testing.T) {
			got := Infer(prompt)
			if !got.TaskLike || got.Intent == nil {
				t.Fatalf("broad outcome was not inferred: %+v", got)
			}
			if got.Intent.Class != "readiness" || got.Intent.Action != "readiness" || got.Intent.Completeness != "full" || !got.Intent.NeedsTests {
				t.Fatalf("broad outcome intent = %+v", got.Intent)
			}
			joined := strings.ToLower(strings.Join(got.Steps, " "))
			for _, want := range []string{"inspect the current behavior", "targeted verification", "broader affected checks", "adjacent flows"} {
				if !strings.Contains(joined, want) {
					t.Fatalf("broad outcome steps %q do not include %q", joined, want)
				}
			}
		})
	}
}

func TestInferBroadOutcomeKeepsRiskAndTargetedPrecedence(t *testing.T) {
	security := Infer("make this ready to launch and fix the auth permission bug")
	if !security.TaskLike || security.Intent == nil || security.Intent.Class != "security" || security.Intent.Risk != "high" || !security.Intent.NeedsApproval || security.Intent.Completeness != "full" {
		t.Fatalf("security broad outcome intent = %+v", security.Intent)
	}

	destructive := Infer("make this ready to launch and deploy it")
	if !destructive.TaskLike || destructive.Intent == nil || destructive.Intent.Class != "change" || destructive.Intent.Risk != "high" || !destructive.Intent.NeedsApproval || destructive.Intent.Completeness != "full" {
		t.Fatalf("destructive broad outcome intent = %+v", destructive.Intent)
	}

	for _, tc := range []struct {
		prompt   string
		taskLike bool
	}{
		{prompt: "update button text", taskLike: true},
		{prompt: "update internal/auth.go", taskLike: true},
		// "change" is not currently an action trigger. Keep this baseline
		// behavior explicit rather than broadening unrelated task admission.
		{prompt: "change button text", taskLike: false},
	} {
		t.Run(tc.prompt, func(t *testing.T) {
			got := Infer(tc.prompt)
			if got.TaskLike != tc.taskLike {
				t.Fatalf("targeted request task_like=%v, want %v: %+v", got.TaskLike, tc.taskLike, got)
			}
			if !tc.taskLike {
				return
			}
			if got.Intent == nil || got.Intent.Class == "readiness" || got.Intent.Completeness != "targeted" {
				t.Fatalf("targeted request widened unexpectedly: %+v", got.Intent)
			}
		})
	}
}

func TestInferDocumentationRequestIsTaskLike(t *testing.T) {
	got := Infer("instead, document the note workflow")
	if !got.TaskLike || got.Intent == nil {
		t.Fatalf("documentation request was not inferred: %+v", got)
	}
	if got.Intent.Class != "documentation" || got.Intent.NeedsTests {
		t.Fatalf("documentation intent = %+v", got.Intent)
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
