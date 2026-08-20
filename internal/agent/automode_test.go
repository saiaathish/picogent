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
