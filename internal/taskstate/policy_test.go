package taskstate

import "testing"

func TestPolicyContinuationAndStopRules(t *testing.T) {
	base := func() *Task {
		task, err := New("s", "fix it", []string{"work"})
		if err != nil {
			t.Fatal(err)
		}
		task.Status = StatusWorking
		return task
	}
	tests := []struct {
		name    string
		mutate  func(*Task)
		signals Signals
		want    StopReason
	}{
		{"goal signal", nil, Signals{GoalResolved: true, SafeNextAction: true}, StopGoalComplete},
		{"done state", func(v *Task) { v.Status = StatusDone }, Signals{SafeNextAction: true}, StopGoalComplete},
		{"permission", nil, Signals{PermissionNeeded: true, SafeNextAction: true}, StopPermissionNeeded},
		{"choice", nil, Signals{UserChoiceRequired: true, SafeNextAction: true}, StopUserChoiceRequired},
		{"no safe action", nil, Signals{}, StopUserChoiceRequired},
		{"blocked state", func(v *Task) { v.Status = StatusBlocked }, Signals{SafeNextAction: true}, StopUserChoiceRequired},
		{"resource", nil, Signals{ResourceUnavailable: true, SafeNextAction: true}, StopResourceUnavailable},
		{"budget", func(v *Task) { v.Attempts = 2 }, Signals{SafeNextAction: true}, StopBudgetExhausted},
	}
	policy := Policy{MaxAttempts: 2, MaxVerificationFailures: 3}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := base()
			if tt.mutate != nil {
				tt.mutate(task)
			}
			got := policy.Decide(task, tt.signals)
			if got.Continue || got.Reason != tt.want || got.Message == "" {
				t.Fatalf("decision = %+v, want stop %q", got, tt.want)
			}
		})
	}

	task := base()
	got := policy.Decide(task, Signals{SafeNextAction: true})
	if !got.Continue || got.Reason != StopNone || got.Message == "" {
		t.Fatalf("continue decision = %+v", got)
	}
}

func TestPolicyStopsAfterRepeatedVerificationFailure(t *testing.T) {
	task, err := New("s", "fix", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.Status = StatusVerifying
	policy := Policy{MaxAttempts: 10, MaxVerificationFailures: 2}
	task.AddVerification("test", false, "failed once")
	if got := policy.Decide(task, Signals{SafeNextAction: true}); !got.Continue {
		t.Fatalf("first failure should continue: %+v", got)
	}
	task.AddVerification("test", false, "failed twice")
	if got := policy.Decide(task, Signals{SafeNextAction: true}); got.Continue || got.Reason != StopVerificationFailures {
		t.Fatalf("repeated failure should stop: %+v", got)
	}
	task.AddVerification("test", true, "passed")
	if got := policy.Decide(task, Signals{SafeNextAction: true}); !got.Continue {
		t.Fatalf("pass should reset failure streak: %+v", got)
	}
}

func TestPolicyPrecedenceAndDefaults(t *testing.T) {
	task, err := New("s", "fix", nil)
	if err != nil {
		t.Fatal(err)
	}
	task.Status = StatusWorking
	task.Attempts = DefaultPolicy().MaxAttempts
	got := ShouldContinue(task, Signals{GoalResolved: true, PermissionNeeded: true})
	if got.Reason != StopGoalComplete {
		t.Fatalf("goal completion must win precedence: %+v", got)
	}
	got = (Policy{}).Decide(task, Signals{SafeNextAction: true})
	if got.Reason != StopBudgetExhausted {
		t.Fatalf("zero policy should use defaults: %+v", got)
	}
	got = (Policy{MaxAttempts: 100}).Decide(nil, Signals{SafeNextAction: true})
	if got.Reason != StopGoalComplete {
		t.Fatalf("nil task should not continue: %+v", got)
	}
}
