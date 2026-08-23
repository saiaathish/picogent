package agent

import "strings"

// AutoDecision is the result of inferring task mode and optional goal from user text.
type AutoDecision struct {
	TaskMode TaskMode
	TaskWhy  string
	Goal     string
	GoalSet  bool
}

// ScopeTaskMode converts the small preflight's stable choice IDs into the
// existing task boundaries. It intentionally returns Agent for implementation
// choices so a scope answer never changes the user's persistent mode.
func ScopeTaskMode(choiceID string) TaskMode {
	switch strings.ToLower(strings.TrimSpace(choiceID)) {
	case "plan":
		return TaskPlan
	case "report":
		return TaskAsk
	default:
		return TaskAgent
	}
}

// InferAuto reads a user message and suggests task mode / goal adjustments.
func InferAuto(prompt string, current TaskMode, activeGoal string) AutoDecision {
	var d AutoDecision
	d.TaskMode = current
	if current == "" || !current.Valid() {
		d.TaskMode = TaskAgent
	}

	p := strings.ToLower(strings.TrimSpace(prompt))
	if p == "" {
		return d
	}

	if g, ok := inferGoalPhrase(prompt); ok && g != activeGoal {
		d.Goal = g
		d.GoalSet = true
	}

	mode, why := inferTaskMode(p, current, activeGoal != "" || d.GoalSet)
	d.TaskMode = mode
	d.TaskWhy = why
	return d
}

func inferTaskMode(p string, current TaskMode, hasGoal bool) (TaskMode, string) {
	for _, k := range []string{
		"go ahead", "build it", "implement it", "implement the", "execute the plan",
		"apply the plan", "looks good", "ship it", "just do it", "make the changes", "start building",
	} {
		if strings.Contains(p, k) {
			return TaskAgent, "implementation requested"
		}
	}

	for _, k := range []string{
		"bug", "error:", "error ", "crash", "broken", "doesn't work", "does not work",
		"fails when", "stack trace", "panic", "exception", "regression", "root cause",
		"debug this", "why does it fail", "not working", "flaky test", "flaky tests",
	} {
		if strings.Contains(p, k) {
			return TaskDebug, "bug or failure reported"
		}
	}

	if strings.HasSuffix(strings.TrimSpace(p), "?") {
		action := []string{"fix", "add", "create", "implement", "update", "remove", "delete", "refactor", "write", "build", "change"}
		hasAction := false
		for _, a := range action {
			if strings.Contains(p, a) {
				hasAction = true
				break
			}
		}
		if !hasAction {
			for _, s := range []string{"what ", "how ", "why ", "where ", "when ", "explain ", "describe ", "show me ", "tell me ", "can you explain"} {
				if strings.HasPrefix(p, s) || strings.Contains(p, " "+s) {
					return TaskAsk, "question detected"
				}
			}
		}
	}

	for _, k := range []string{
		"plan ", "plan how", "design a", "design the", "architect", "outline how",
		"break down", "roadmap", "strategy for", "before we code", "before changing",
		"before you edit", "how should we approach", "think through", "figure out how to",
	} {
		if strings.Contains(p, k) {
			return TaskPlan, "planning request"
		}
	}
	if strings.Contains(p, "refactor") && (strings.Contains(p, " entire ") || strings.Contains(p, " whole ")) {
		return TaskPlan, "large refactor — plan first"
	}

	if len(strings.Fields(p)) <= 14 && (current == TaskPlan || current == TaskDebug) {
		for _, k := range []string{"also", "and then", "what about", "one more", "another", "continue", "yes", "okay", "ok ", "thanks", "that too"} {
			if strings.Contains(p, k) {
				return current, ""
			}
		}
	}

	for _, k := range []string{
		"fix ", "add ", "create ", "implement ", "update ", "remove ", "delete ",
		"run tests", "run the tests", "commit ", "write ", "change ", "patch ",
	} {
		if strings.Contains(p, k) {
			return TaskAgent, "implementation task"
		}
	}

	if hasGoal {
		return TaskAgent, "active goal"
	}

	if current == TaskAsk || current == TaskPlan || current == TaskDebug {
		// Short follow-ups stay in the current mode.
		if len(strings.Fields(p)) <= 10 {
			return current, ""
		}
	}

	return TaskAgent, ""
}

func inferGoalPhrase(prompt string) (string, bool) {
	p := strings.TrimSpace(prompt)
	lower := strings.ToLower(p)
	if len(p) < 12 {
		return "", false
	}

	triggers := []string{
		"until ", "don't stop until", "do not stop until", "keep going until",
		"fix all", "make sure all", "get all", "make ci green", "make the ci green",
		"fix every", "complete the migration", "migrate all", "get tests green",
		"work until", "don't stop till", "keep working until",
	}
	for _, t := range triggers {
		if strings.Contains(lower, t) {
			return p, true
		}
	}

	if strings.Count(lower, " and ") >= 2 && (strings.Contains(lower, "all ") || strings.Contains(lower, "every ")) {
		return p, true
	}

	return "", false
}
