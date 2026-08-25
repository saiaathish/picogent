package agent

import (
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/taskstate"
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

func TestRepeatedVerificationFailureRequiresDifferentRepairRoute(t *testing.T) {
	a := &Agent{task: &taskstate.Task{Verification: []taskstate.Verification{
		{Summary: "verify FAIL old_string not found in auth.go"},
		{Summary: " VERIFY   fail old_string not found in auth.go "},
	}}}
	if !a.repeatedVerificationFailure() {
		t.Fatal("identical normalized verification failures were not detected")
	}
	if got := durableRepairPrompt("verify FAIL old_string not found in auth.go", true); !strings.Contains(got, "materially different safe repair") {
		t.Fatalf("repair prompt did not demand route diversity: %q", got)
	}
	a.task.Verification[1].Summary = "verify FAIL command not found: gofmt"
	if a.repeatedVerificationFailure() {
		t.Fatal("different failure fingerprints were treated as repeated")
	}
}

func containsFold(s, want string) bool {
	return len(s) >= len(want) && strings.Contains(strings.ToLower(s), strings.ToLower(want))
}
