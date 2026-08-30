package taskstate

// StopReason explains why the autonomous loop must yield.
type StopReason string

const (
	StopNone                 StopReason = ""
	StopGoalComplete         StopReason = "goal_complete"
	StopPermissionNeeded     StopReason = "permission_needed"
	StopUserChoiceRequired   StopReason = "user_choice_required"
	StopVerificationFailures StopReason = "verification_repeatedly_failed"
	StopResourceUnavailable  StopReason = "resource_unavailable"
	StopBudgetExhausted      StopReason = "budget_exhausted"
	StopCanceled             StopReason = "canceled"
)

// Valid reports whether the stop reason is one of the bounded policy values.
func (r StopReason) Valid() bool {
	switch r {
	case StopNone, StopGoalComplete, StopPermissionNeeded, StopUserChoiceRequired,
		StopVerificationFailures, StopResourceUnavailable, StopBudgetExhausted, StopCanceled:
		return true
	default:
		return false
	}
}

// Policy bounds autonomous continuation. Zero values use conservative defaults.
type Policy struct {
	MaxAttempts             int
	MaxVerificationFailures int
}

// DefaultPolicy keeps progress bounded without making users operate a planner.
func DefaultPolicy() Policy {
	return Policy{MaxAttempts: 20, MaxVerificationFailures: 3}
}

// Signals describe facts known by the caller after an agent round.
type Signals struct {
	GoalResolved        bool
	SafeNextAction      bool
	PermissionNeeded    bool
	UserChoiceRequired  bool
	ResourceUnavailable bool
}

// Decision tells the integration whether to run another autonomous round.
type Decision struct {
	Continue bool
	Reason   StopReason
	Message  string
}

// Decide applies continuation gates in explicit stop-rule order.
func (p Policy) Decide(task *Task, signals Signals) Decision {
	p = p.normalized()
	if task == nil {
		return stop(StopGoalComplete, "goal complete")
	}
	if (signals.GoalResolved || task.Status == StatusDone) && task.CompletionReady() {
		return stop(StopGoalComplete, "goal complete")
	}
	if signals.PermissionNeeded {
		return stop(StopPermissionNeeded, "permission needed")
	}
	if signals.UserChoiceRequired || !signals.SafeNextAction || task.Status == StatusBlocked {
		return stop(StopUserChoiceRequired, "user choice genuinely required")
	}
	if task.ConsecutiveVerificationFailures() >= p.MaxVerificationFailures {
		return stop(StopVerificationFailures, "verification repeatedly failed")
	}
	if signals.ResourceUnavailable {
		return stop(StopResourceUnavailable, "tool or resource unavailable")
	}
	if task.Attempts >= p.MaxAttempts {
		return stop(StopBudgetExhausted, "task budget exhausted")
	}
	return Decision{Continue: true, Message: "goal unresolved; safe permitted action and budget remain"}
}

// ShouldContinue applies DefaultPolicy.
func ShouldContinue(task *Task, signals Signals) Decision {
	return DefaultPolicy().Decide(task, signals)
}

func (p Policy) normalized() Policy {
	d := DefaultPolicy()
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = d.MaxAttempts
	}
	if p.MaxVerificationFailures <= 0 {
		p.MaxVerificationFailures = d.MaxVerificationFailures
	}
	return p
}

func stop(reason StopReason, message string) Decision {
	return Decision{Reason: reason, Message: message}
}
