package taskstate

import (
	"strings"
	"unicode/utf8"
)

var replacementPhrases = []string{
	"replace the current goal with",
	"replace the current task with",
	"cancel the previous task; instead",
}

// MentionsReplacement detects a replacement wrapper even in quoted, question,
// conditional, or negated text. It only suppresses automatic goal inference;
// it must never be used to authorize replacement. Use ExplicitReplacement and
// ReplaceOutcome for that admission boundary.
func MentionsReplacement(prompt string) bool {
	prompt = strings.ToLower(strings.Join(strings.Fields(prompt), " "))
	return containsDelimitedPhrase(prompt, replacementPhrases)
}

// ExplicitReplacement recognizes a small set of direct, anchored replacement
// instructions and returns their outcome text. Other follow-ups remain steering.
// Recognition does not imply that the extracted outcome is task-like; callers
// must pass it to ReplaceOutcome before treating it as a new durable contract.
func ExplicitReplacement(prompt string) (string, bool) {
	prompt = strings.Join(strings.Fields(prompt), " ")
	lower := strings.ToLower(prompt)
	for _, phrase := range replacementPhrases {
		prefix := phrase + " "
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
	if strings.Trim(outcome, " \t\r\n.;:!…") == "" || strings.ContainsAny(outcome, "?？") || hasReplacementQuoteDelimiter(outcome) {
		return false
	}
	lower := strings.ToLower(outcome)
	if hasReplacementPrefix(lower, informationalPrefixes) || hasReplacementPrefix(lower, []string{
		"can ", "could ", "would ", "should ", "will ", "may ", "might ",
		"is ", "are ", "do ", "does ", "did ",
		"not ", "never ", "no ", "do not ", "please do not ",
		"don't ", "don’t ", "please don't ", "please don’t ",
		"cannot ", "can't ", "can’t ", "couldn't ", "couldn’t ",
		"wouldn't ", "wouldn’t ", "shouldn't ", "shouldn’t ", "won't ", "won’t ",
		"isn't ", "isn’t ", "aren't ", "aren’t ", "doesn't ", "doesn’t ", "didn't ", "didn’t ",
		"what's ", "what’s ", "why's ", "why’s ", "how's ", "how’s ",
		"where's ", "where’s ", "who's ", "who’s ", "when's ", "when’s ",
		"when ", "imagine ",
	}) {
		return false
	}
	return !containsDelimitedPhrase(lower, []string{
		"if", "unless", "suppose", "supposing", "hypothetical", "hypothetically",
	})
}

// A prefix ends at a word boundary, not only a space. Punctuation after a
// negation or question word must not turn it into replacement authority.
func hasReplacementPrefix(text string, prefixes []string) bool {
	for _, prefix := range prefixes {
		prefix = strings.TrimSpace(prefix)
		if strings.HasPrefix(text, prefix) && !wordRuneAt(text, len(prefix)) {
			return true
		}
	}
	return false
}

func hasReplacementQuoteDelimiter(outcome string) bool {
	for at, r := range outcome {
		switch r {
		case '\'', '’':
			// Apostrophes inside words (user's, doesn't, user’s) are text,
			// while an opening or closing quote remains ambiguous.
			if !wordRuneBefore(outcome, at) || !wordRuneAt(outcome, at+utf8.RuneLen(r)) {
				return true
			}
		case '"', '`', '“', '”', '‘', '«', '»':
			return true
		}
	}
	return false
}
