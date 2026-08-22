package agent

import (
	"strings"
	"testing"
)

func TestDurableRecoveryHintClassifiesCommonFailures(t *testing.T) {
	tests := []struct {
		name     string
		evidence string
		want     string
	}{
		{"ambiguous edit", "old_string found 2 times", "multiple regions"},
		{"stale file", "old_string not found in auth.go", "stale"},
		{"truncated output", "output truncated", "truncated"},
		{"missing runner", "executable file not found", "runner"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := durableRecoveryHint(tt.evidence); got == "" || !containsFold(got, tt.want) {
				t.Fatalf("hint=%q want substring %q", got, tt.want)
			}
		})
	}
}

func containsFold(s, want string) bool {
	return len(s) >= len(want) && strings.Contains(strings.ToLower(s), strings.ToLower(want))
}
