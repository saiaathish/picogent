package taskstate

import "testing"

func TestExplicitReplacement(t *testing.T) {
	tests := []struct {
		prompt string
		want   string
	}{
		{"replace the current goal with fix signup", "fix signup"},
		{"replace the current task with review the API", "review the API"},
		{"cancel the previous task; instead document the API", "document the API"},
		{"  REPLACE the current GOAL with   build a CLI.  ", "build a CLI."},
		{"replace\tthe current task with\nfix the cache", "fix the cache"},
		{"replace the current task with fix the user's profile", "fix the user's profile"},
		{"replace the current task with fix the user’s profile", "fix the user’s profile"},
		{"replace the current goal with fix the cache that doesn't refresh", "fix the cache that doesn't refresh"},
		{"replace the current goal with fix the cache that doesn’t refresh", "fix the cache that doesn’t refresh"},
		{"cancel the previous task; instead fix l’utilisateur profile", "fix l’utilisateur profile"},
		// Classification extracts a nonempty outcome; inference decides whether
		// that outcome can become a task without inventing a requirement.
		{"replace the current goal with a better outcome", "a better outcome"},
	}
	for _, tt := range tests {
		t.Run(tt.prompt, func(t *testing.T) {
			got, ok := ExplicitReplacement(tt.prompt)
			if !ok || got != tt.want {
				t.Fatalf("ExplicitReplacement(%q) = %q, %v; want %q, true", tt.prompt, got, ok, tt.want)
			}
		})
	}
}

func TestExplicitReplacementRejectsSteeringAndAmbiguity(t *testing.T) {
	for _, prompt := range []string{
		"", " ",
		"instead fix signup", "also fix signup", "focus on signup", "improve signup",
		"fix signup instead", "please replace the current goal with fix signup",
		"we should replace the current goal with fix signup",
		"replace the goal with fix signup", "replace the current goals with fix signup",
		"cancel the previous task instead fix signup",
		"cancel the previous task; instead", "cancel the previous task; instead ...",
		"replace the current goal with", "replace the current task with   ",
		"replace the current goal with ...",
		`"replace the current goal with fix signup"`,
		"`replace the current task with fix signup`",
		"'replace the current task with fix signup'",
		"“replace the current goal with fix signup”",
		`replace the current goal with "fix signup"`,
		"replace the current goal with 'fix signup'",
		"replace the current goal with 'fix the user's profile'",
		"replace the current goal with ‘fix the user’s profile’",
		"replace the current goal with ’fix signup’",
		"replace the current task with `fix signup`",
		"if needed, replace the current goal with fix signup",
		"suppose we replace the current goal with fix signup",
		"hypothetically replace the current task with fix signup",
		"replace the current goal with hypothetically fix signup",
		"replace the current goal with fix signup if needed",
		"replace the current goal with imagine we fix signup",
		"do not replace the current goal with fix signup",
		"don't replace the current task with fix signup",
		"never replace the current goal with fix signup",
		"replace the current goal with not fixing signup",
		"replace the current goal with do not fix signup",
		"replace the current goal with don't, under any circumstances, fix all tests",
		"replace the current goal with don’t, under any circumstances, fix all tests",
		"replace the current goal with never: fix all tests",
		"replace the current goal with not—fix all tests",
		"replace the current goal with please do not fix signup",
		"replace the current goal with don't fix signup",
		"replace the current goal with don’t fix signup",
		"replace the current goal with please don't fix signup",
		"replace the current goal with please don’t fix signup",
		"replace the current goal with can't fix signup",
		"replace the current goal with can’t fix signup",
		"replace the current goal with shouldn't fix signup",
		"replace the current goal with shouldn’t fix signup",
		"should we replace the current goal with fix signup?",
		"replace the current task with fix signup?",
		"cancel the previous task; instead fix signup？",
		"replace the current goal with can you fix signup",
		"replace the current goal with how to fix signup",
		"replace the current goal with what's needed to fix signup",
		"replace the current goal with what’s needed to fix signup",
	} {
		t.Run(prompt, func(t *testing.T) {
			if got, ok := ExplicitReplacement(prompt); ok || got != "" {
				t.Fatalf("ExplicitReplacement(%q) = %q, %v; want empty, false", prompt, got, ok)
			}
		})
	}
}

func TestMentionsReplacementDoesNotAuthorizeAmbiguousCommands(t *testing.T) {
	for _, prompt := range []string{
		`replace the current goal with "fix all tests"`,
		"replace the current task with 'fix all tests'",
		"replace the current goal with ‘fix the user’s profile’",
		"replace the current task with fix all tests?",
		"replace the current goal with fix all tests if needed",
		"replace the current goal with don't fix all tests",
		"replace the current goal with don’t fix all tests",
		`"replace the current goal with fix all tests"`,
		"`replace the current task with fix all tests`",
		"should we replace the current goal with fix all tests?",
		"if needed, replace the current task with fix all tests",
		"do not replace the current goal with fix all tests",
		"don't replace the current task with fix all tests",
		"don’t replace the current task with fix all tests",
		"suppose we cancel the previous task; instead fix all tests",
		"cancel the previous task; instead fix all tests unless needed",
		"  NEVER\nREPLACE\tthe current GOAL with fix all tests  ",
		"replace the current task with",
	} {
		t.Run(prompt, func(t *testing.T) {
			if !MentionsReplacement(prompt) {
				t.Fatalf("MentionsReplacement(%q) missed a wrapper", prompt)
			}
			if outcome, ok := ExplicitReplacement(prompt); ok || outcome != "" {
				t.Fatalf("mention authorized replacement: %q, %v", outcome, ok)
			}
		})
	}
}

func TestMentionsReplacementMatchesOnlySharedPhrases(t *testing.T) {
	for _, prompt := range []string{
		"replace the current goal with fix all tests",
		"replace the current task with fix the user's profile",
		"cancel the previous task; instead document the API",
	} {
		if !MentionsReplacement(prompt) {
			t.Fatalf("MentionsReplacement(%q) missed a direct command", prompt)
		}
	}
	for _, prompt := range []string{
		"", "fix all flaky tests and make CI green",
		"fix all tests for replacement tasks",
		"replace the current goals with fix all tests",
		"replace the current goal without fixing all tests",
		"prereplace the current goal with fix all tests",
		"cancel the previous task; insteadify all tests",
	} {
		if MentionsReplacement(prompt) {
			t.Fatalf("MentionsReplacement(%q) matched without a wrapper", prompt)
		}
	}
}
