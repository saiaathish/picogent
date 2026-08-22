package taskstate

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Inference is the deterministic, cheap result of inspecting a user prompt.
type Inference struct {
	TaskLike bool
	Goal     string
	Steps    []string
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

	return Inference{
		TaskLike: true,
		Goal:     goal,
		Steps:    inferredSteps(lower),
	}
}

// NewFromPrompt returns a task only when prompt looks task-like.
func NewFromPrompt(sessionID, prompt string) (*Task, bool, error) {
	inferred := Infer(prompt)
	if !inferred.TaskLike {
		return nil, false, nil
	}
	task, err := New(sessionID, inferred.Goal, inferred.Steps)
	return task, true, err
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
	goal := compactText(prompt, 600)
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
