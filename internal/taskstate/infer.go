package taskstate

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Inference is the deterministic, cheap result of inspecting a user prompt.
type Inference struct {
	TaskLike         bool
	Goal             string
	Steps            []string
	Intent           *IntentContract
	DefinitionOfDone []Criterion
}

var actionPhrases = []string{
	"fix", "implement", "add", "build", "create", "update", "upgrade",
	"refactor", "debug", "investigate", "diagnose", "remove", "delete",
	"migrate", "test", "verify", "review", "audit", "optimize", "improve",
	"clean up", "set up", "setup", "wire", "connect", "finish", "ship",
	"make ", "run ", "look into", "get this done",
}

var problemPhrases = []string{
	"failing", "fails", "failure", "broken", "doesn't work", "does not work",
	"error", "crash", "bug", "regression", "flaky", "stuck",
}

var informationalPrefixes = []string{
	"what ", "why ", "how ", "when ", "where ", "who ", "explain ",
	"tell me ", "show me ", "do you know ",
}

// Infer recognizes task-like prompts and creates a compact default plan.
// It is intentionally conservative for informational questions.
func Infer(prompt string) Inference {
	goal := normalizeGoal(prompt)
	if goal == "" {
		return Inference{}
	}
	lower := strings.ToLower(goal)
	action := containsDelimitedPhrase(lower, actionPhrases)
	problem := containsDelimitedPhrase(lower, problemPhrases)
	question := strings.HasSuffix(strings.TrimSpace(prompt), "?") || hasPrefix(lower, informationalPrefixes)
	if question && !action && !problem {
		return Inference{}
	}
	if !action && !problem {
		return Inference{}
	}

	intent, criteria := inferIntent(goal, lower, action, problem)
	steps := make([]string, 0, len(criteria))
	for _, criterion := range criteria {
		steps = append(steps, criterion.Description)
	}
	if len(steps) == 0 {
		steps = inferredSteps(lower)
	}
	return Inference{TaskLike: true, Goal: goal, Steps: steps, Intent: intent, DefinitionOfDone: criteria}
}

// NewFromPrompt returns a task only when prompt looks task-like.
func NewFromPrompt(sessionID, prompt string) (*Task, bool, error) {
	inferred := Infer(prompt)
	if !inferred.TaskLike {
		return nil, false, nil
	}
	task, err := New(sessionID, inferred.Goal, inferred.Steps)
	if task != nil {
		task.Intent = cloneIntent(inferred.Intent)
		task.DefinitionOfDone = append([]Criterion(nil), inferred.DefinitionOfDone...)
	}
	return task, true, err
}

func inferIntent(goal, prompt string, action, problem bool) (*IntentContract, []Criterion) {
	intent := &IntentContract{
		Outcome:      goal,
		Action:       "implementation",
		Completeness: "targeted",
		Scope:        "affected workspace",
		Risk:         "low",
		NeedsTests:   true,
		Confidence:   "medium",
	}
	class := "general"
	switch {
	case containsDelimitedPhrase(prompt, []string{"review", "audit", "inspect", "investigate"}):
		class, intent.Action, intent.NeedsTests = "review", "investigation", false
	case containsDelimitedPhrase(prompt, []string{"security", "auth", "secret", "credential", "permission"}):
		class, intent.Risk, intent.NeedsApproval = "security", "high", true
	case containsDelimitedPhrase(prompt, []string{"delete", "remove", "drop", "reset", "publish", "deploy", "send"}):
		class, intent.Risk, intent.NeedsApproval = "change", "high", true
	case containsDelimitedPhrase(prompt, []string{"bug", "broken", "crash", "error", "failure", "failing", "fails", "regression", "flaky", "debug", "diagnose"}):
		class = "bug"
	case containsDelimitedPhrase(prompt, []string{"ui", "gui", "frontend", "browser", "responsive", "layout", "button", "screen", "visual"}):
		class, intent.NeedsVisual = "ui", true
	case containsDelimitedPhrase(prompt, []string{"performance", "latency", "slow", "memory", "startup", "benchmark", "optimize"}):
		class = "performance"
	case containsDelimitedPhrase(prompt, []string{"refactor", "cleanup", "clean up"}):
		class = "refactor"
	case containsDelimitedPhrase(prompt, []string{"docs", "documentation", "readme", "explain"}):
		class, intent.NeedsTests = "documentation", false
	case containsDelimitedPhrase(prompt, []string{"setup", "set up", "install", "configure"}):
		class = "setup"
	}
	if !action && !problem {
		intent.Action = "investigation"
		intent.NeedsTests = false
	}
	if containsDelimitedPhrase(prompt, []string{"security", "auth", "secret", "credential", "permission"}) {
		intent.Risk = "high"
		intent.NeedsApproval = true
	}
	if containsDelimitedPhrase(prompt, []string{"research", "latest", "current api", "look up", "unknown", "unfamiliar"}) {
		intent.NeedsResearch = true
	}
	if containsDelimitedPhrase(prompt, []string{"finish", "complete", "all", "every", "professional", "make it good", "make this project good", "get this done"}) {
		intent.Completeness = "full"
	}
	if intent.NeedsApproval && intent.Risk == "high" {
		intent.Scope = "explicitly authorized boundary"
	}
	if class == "general" && (action || problem) {
		intent.Confidence = "high"
	}
	intent.Class = class
	return intent, intentCriteria(*intent)
}

func intentCriteria(intent IntentContract) []Criterion {
	if intent.Class == "review" {
		return []Criterion{{Description: "Inspect the affected surface and its existing contracts", Required: true}, {Description: "Reproduce or validate each concrete concern", Required: true}, {Description: "Record findings with direct evidence", Required: true}, {Description: "Run relevant checks for the reviewed surface", Required: true}}
	}
	if intent.Action == "investigation" && !intent.NeedsTests {
		return []Criterion{{Description: "Inspect the affected surface and collect direct evidence", Required: true}, {Description: "Report confirmed findings, unknowns, and the next safe action", Required: true}}
	}
	criteria := []Criterion{{Description: "Inspect the affected code, behavior, and existing conventions", Required: true}}
	switch intent.Class {
	case "bug":
		criteria = append(criteria, Criterion{Description: "Reproduce or locate a credible failure", Required: true}, Criterion{Description: "Apply the smallest responsible fix", Required: true}, Criterion{Description: "Run targeted verification", Required: true}, Criterion{Description: "Run broader affected checks", Required: true})
	case "ui":
		criteria = append(criteria, Criterion{Description: "Implement the complete interaction and required states", Required: true}, Criterion{Description: "Check keyboard, focus, responsive, and reduced-motion behavior", Required: true}, Criterion{Description: "Run targeted verification and inspect the rendered result", Required: true})
	case "performance":
		criteria = append(criteria, Criterion{Description: "Measure the current behavior before changing it", Required: true}, Criterion{Description: "Improve the largest measured bottleneck", Required: true}, Criterion{Description: "Rerun the benchmark and regression checks", Required: true})
	case "review":
		criteria = append(criteria, Criterion{Description: "Validate each concrete concern with direct evidence", Required: true}, Criterion{Description: "Run relevant checks for the reviewed surface", Required: true})
	case "documentation":
		criteria = append(criteria, Criterion{Description: "Ground statements in current repository behavior", Required: true}, Criterion{Description: "Validate links, examples, and affected checks", Required: true})
	default:
		if intent.Completeness == "full" {
			criteria = append(criteria, Criterion{Description: "Inspect the current behavior and identify what remains incomplete", Required: true}, Criterion{Description: "Make the smallest complete change", Required: true}, Criterion{Description: "Run targeted verification", Required: true}, Criterion{Description: "Run broader affected checks", Required: true}, Criterion{Description: "Recheck adjacent flows and stop only when the requested outcome is genuinely satisfied", Required: true})
			if len(criteria) > 8 {
				criteria = criteria[:8]
			}
			return criteria
		}
		criteria = append(criteria, Criterion{Description: "Implement the smallest complete change", Required: true}, Criterion{Description: "Add or update focused tests when behavior changes", Required: true}, Criterion{Description: "Run targeted verification", Required: true}, Criterion{Description: "Run broader affected checks", Required: true})
	}
	if intent.Completeness == "full" && intent.Class != "review" {
		criteria = append(criteria, Criterion{Description: "Recheck adjacent flows and stop only when the requested outcome is genuinely satisfied", Required: true})
	}
	if len(criteria) > 8 {
		criteria = criteria[:8]
	}
	return criteria
}

func cloneIntent(intent *IntentContract) *IntentContract {
	if intent == nil {
		return nil
	}
	cp := *intent
	return &cp
}

func inferredSteps(prompt string) []string {
	switch {
	case containsDelimitedPhrase(prompt, []string{"review", "audit"}):
		return []string{
			"Inspect the affected surface and its existing contracts",
			"Reproduce or validate each concrete concern",
			"Record findings with direct evidence",
			"Run relevant checks for the reviewed surface",
		}
	case containsDelimitedPhrase(prompt, []string{"debug", "diagnose", "investigate", "failing", "fails", "failure", "broken", "crash", "bug", "flaky"}):
		return []string{
			"Locate the affected path and reproduce the failure",
			"Trace the failure to the smallest responsible area",
			"Apply the smallest safe fix",
			"Run targeted verification",
			"Run broader affected checks",
		}
	case containsDelimitedPhrase(prompt, []string{"build", "create", "implement", "add", "wire", "connect"}):
		return []string{
			"Inspect the affected code and existing conventions",
			"Implement the smallest complete change",
			"Add or update focused tests",
			"Run targeted verification",
			"Run broader affected checks",
		}
	default:
		return []string{
			"Inspect the affected code and current behavior",
			"Make the smallest complete change",
			"Run targeted verification",
			"Run broader affected checks",
		}
	}
}

func normalizeGoal(prompt string) string {
	goal := compactText(prompt, maxTaskGoal)
	lower := strings.ToLower(goal)
	for _, prefix := range []string{"please ", "could you please ", "can you please ", "would you please "} {
		if strings.HasPrefix(lower, prefix) {
			goal = strings.TrimSpace(goal[len(prefix):])
			break
		}
	}
	goal = strings.TrimRight(goal, " \t\n.?")
	if goal == "" {
		return ""
	}
	r, size := utf8.DecodeRuneInString(goal)
	return string(unicode.ToUpper(r)) + goal[size:]
}

func containsDelimitedPhrase(s string, phrases []string) bool {
	for _, phrase := range phrases {
		phrase = strings.TrimSpace(phrase)
		from := 0
		for {
			i := strings.Index(s[from:], phrase)
			if i < 0 {
				break
			}
			i += from
			beforeOK := i == 0 || !wordRuneBefore(s, i)
			after := i + len(phrase)
			afterOK := after == len(s) || !wordRuneAt(s, after)
			if beforeOK && afterOK {
				return true
			}
			from = i + len(phrase)
		}
	}
	return false
}

func wordRuneBefore(s string, at int) bool {
	r, _ := utf8.DecodeLastRuneInString(s[:at])
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func wordRuneAt(s string, at int) bool {
	r, _ := utf8.DecodeRuneInString(s[at:])
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func hasPrefix(s string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}
