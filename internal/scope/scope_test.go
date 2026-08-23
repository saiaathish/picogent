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

func TestAnalyzeOffersSimpleRecommendedChoices(t *testing.T) {
	tests := []struct {
		prompt string
		wantQ  string
		wantID string
	}{
		{"build something", "How big should the first pass be?", "small"},
		{"fix everything", "What should I focus on first?", "focused"},
		{"make it better", "What outcome do you want first?", "focused"},
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
	if _, ok := Apply("build something", p, "not-a-choice"); ok {
		t.Fatal("unknown choice was accepted")
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
