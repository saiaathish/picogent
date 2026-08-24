package agent

import "testing"

func TestInferTaskModeDebug(t *testing.T) {
	m, why := inferTaskMode("the login form crashes on submit", TaskAgent, false)
	if m != TaskDebug || why == "" {
		t.Fatalf("got %s %q", m, why)
	}
}

func TestInferTaskModePlan(t *testing.T) {
	m, _ := inferTaskMode("plan how to refactor the auth module", TaskAgent, false)
	if m != TaskPlan {
		t.Fatal(m)
	}
}

func TestInferTaskModeAsk(t *testing.T) {
	m, _ := inferTaskMode("how does the router pick models?", TaskAgent, false)
	if m != TaskAsk {
		t.Fatal(m)
	}
}

func TestInferTaskModeReportFirst(t *testing.T) {
	m, why := inferTaskMode("build something, but inspect and report first", TaskAgent, false)
	if m != TaskAsk || why == "" {
		t.Fatalf("got %s %q", m, why)
	}
}

func TestScopeTaskModeKeepsExplicitPlanAndReportChoices(t *testing.T) {
	for _, tt := range []struct {
		choice string
		want   TaskMode
	}{
		{"plan", TaskPlan},
		{"report", TaskAsk},
		{"explore", TaskAsk},
		{"small", TaskAgent},
	} {
		if got := ScopeTaskMode(tt.choice); got != tt.want {
			t.Fatalf("ScopeTaskMode(%q) = %q, want %q", tt.choice, got, tt.want)
		}
	}
}

func TestInferAutomaticScopePreservesSelectedBoundaries(t *testing.T) {
	for _, tt := range []struct {
		prompt  string
		current TaskMode
		want    TaskMode
	}{
		{"create something", TaskPlan, TaskPlan},
		{"fix everything", TaskAsk, TaskAsk},
		{"remove everything", TaskDebug, TaskDebug},
		{"go ahead and build it", TaskPlan, TaskAgent},
		{"build something, but plan it first", TaskAgent, TaskPlan},
		{"build something, but inspect and report first", TaskAgent, TaskAsk},
		{"debug this broken build", TaskAgent, TaskDebug},
		{"make me a website for my landscaping business", TaskAgent, TaskAgent},
		{"this button doesn’t work", TaskAgent, TaskDebug},
		{"I want this app to feel way better", TaskAgent, TaskAgent},
		{"finish this project", TaskAgent, TaskAgent},
	} {
		t.Run(string(tt.current)+"/"+tt.prompt, func(t *testing.T) {
			if got := InferAutomaticScope(tt.prompt, tt.current, "").TaskMode; got != tt.want {
				t.Fatalf("InferAutomaticScope(%q, %q) = %q, want %q", tt.prompt, tt.current, got, tt.want)
			}
		})
	}
}

func TestInferTaskModeImplementExitPlan(t *testing.T) {
	m, _ := inferTaskMode("looks good, go ahead and build it", TaskPlan, false)
	if m != TaskAgent {
		t.Fatal(m)
	}
}

func TestInferGoalPhrase(t *testing.T) {
	g, ok := inferGoalPhrase("fix all flaky tests and make CI green")
	if !ok || g == "" {
		t.Fatal("expected goal")
	}
}
