package taskstate

import "strings"

// ExplicitReplacement recognizes a small set of direct, anchored replacement
// instructions and returns their outcome text. Other follow-ups remain steering.
// Recognition does not imply that the extracted outcome is task-like; callers
// must pass it to ReplaceOutcome before treating it as a new durable contract.
func ExplicitReplacement(prompt string) (string, bool) {
	prompt = strings.Join(strings.Fields(prompt), " ")
	lower := strings.ToLower(prompt)
	for _, prefix := range []string{
		"replace the current goal with ",
		"replace the current task with ",
		"cancel the previous task; instead ",
	} {
		if !strings.HasPrefix(lower, prefix) {
			continue
		}
		outcome := strings.TrimSpace(prompt[len(prefix):])
		if !directReplacementOutcome(outcome) {
			return "", false
		}
		return outcome, true
	}
	return "", false
}

// Keep ambiguous quoted, conditional, negated, and question payloads out of
// this deliberately narrow recognizer, even when their wrapper is imperative.
func directReplacementOutcome(outcome string) bool {
	if strings.Trim(outcome, " \t\r\n.;:!…") == "" || strings.ContainsAny(outcome, "?？\"'`“”‘’«»") {
		return false
	}
	lower := strings.ToLower(outcome)
	if hasPrefix(lower, informationalPrefixes) || hasPrefix(lower, []string{
		"can ", "could ", "would ", "should ", "will ", "may ", "might ",
		"is ", "are ", "do ", "does ", "did ",
		"not ", "never ", "no ", "do not ", "please do not ",
		"when ", "imagine ",
	}) {
		return false
	}
	return !containsDelimitedPhrase(lower, []string{
		"if", "unless", "suppose", "supposing", "hypothetical", "hypothetically",
	})
}
