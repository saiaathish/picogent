package scope

import "testing"

func TestAnalyzeSkipsSpecificRequests(t *testing.T) {
	for _, prompt := range []string{
		"fix internal/auth/login.go token refresh",
		"update the Button component in `ui/Button.tsx`",
		"run the tests and fix the failing parser test",
		"explain how the checkout flow works",
	} {
		if got, ok := Analyze(prompt); ok || len(got.Choices) != 0 {
			t.Fatalf("Analyze(%q) = %#v, %v; want no preflight", prompt, got, ok)
		}
	}
}

func TestAnalyzeSkipsExplicitCompletionRequests(t *testing.T) {
	for _, prompt := range []string{
		"finish this project",
		"finish the project",
	} {
		if got, ok := Analyze(prompt); ok || len(got.Choices) != 0 {
			t.Fatalf("Analyze(%q) = %#v, %v; want no preflight", prompt, got, ok)
		}
	}
}

func TestAnalyzeOffersSimpleRecommendedChoices(t *testing.T) {
	tests := []struct {
		prompt string
		wantQ  string
		wantID string
	}{
		{"build something", "How big should the first pass be?", "small"},
		{"fix everything", "What should I focus on first?", "focused"},
		{"fix all flaky tests and make CI green", "What should I focus on first?", "focused"},
		{"make it better", "What outcome do you want first?", "focused"},
		{"make me a website for my landscaping business", "How big should the first pass be?", "small"},
		{"this button doesn’t work", "What should I focus on first?", "focused"},
		{"I want this app to feel way better", "What outcome do you want first?", "focused"},
	}
	for _, tt := range tests {
		p, ok := Analyze(tt.prompt)
		if !ok {
			t.Fatalf("Analyze(%q) did not ask a preflight", tt.prompt)
		}
		if p.Question != tt.wantQ || len(p.Choices) < 2 {
			t.Fatalf("prompt for %q = %#v", tt.prompt, p)
		}
		rec := Recommended(p)
		if rec.ID != tt.wantID || !rec.Recommended || rec.Why == "" {
			t.Fatalf("recommended choice for %q = %#v", tt.prompt, rec)
		}
	}
}

func TestApplyValidatesChoiceAndKeepsBoundaryPlain(t *testing.T) {
	p, ok := Analyze("build something")
	if !ok {
		t.Fatal("expected preflight")
	}
	got, ok := Apply("build something", p, "small")
	if !ok || got == "" {
		t.Fatalf("Apply returned %q, %v", got, ok)
	}
	if want := "A small working version"; !contains(got, want) {
		t.Fatalf("applied prompt = %q, want %q", got, want)
	}
	if got, ok := Select(p, "small"); !ok || got.ID != "small" {
		t.Fatalf("Select() = %#v, %v", got, ok)
	}
	if _, ok := Select(p, "not-a-choice"); ok {
		t.Fatal("unknown choice was selected")
	}
	if _, ok := Apply("build something", p, "not-a-choice"); ok {
		t.Fatal("unknown choice was accepted")
	}
}

func TestDefaultMessageExplainsAutomaticRecommendation(t *testing.T) {
	if got, want := DefaultMessage(Choice{Label: "A focused fix"}), "Starting with a focused fix by default."; got != want {
		t.Fatalf("DefaultMessage() = %q, want %q", got, want)
	}
	if got := DefaultMessage(Choice{}); got != "Starting with the recommended scope by default." {
		t.Fatalf("empty choice message = %q", got)
	}
}

func TestTurnBoundaryOverridesBroaderWorkForOneTurn(t *testing.T) {
	got := TurnBoundary(Choice{Label: "A focused fix"})
	for _, want := range []string{"A focused fix", "scope boundary", "takes precedence", "do not expand"} {
		if !contains(got, want) {
			t.Fatalf("TurnBoundary() = %q, want %q", got, want)
		}
	}
}

func contains(text, want string) bool {
	for i := 0; i+len(want) <= len(text); i++ {
		if text[i:i+len(want)] == want {
			return true
		}
	}
	return false
}
