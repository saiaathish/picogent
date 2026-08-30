package outcome

import (
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/taskstate"
)

func TestClassifyFailureUsesSpecificCategoriesBeforeGenericOnes(t *testing.T) {
	tests := []struct {
		name    string
		summary string
		command string
		want    FailureClass
	}{
		{name: "windows wins", summary: "test failed: cannot find the path", command: "go test ./...", want: FailureClassWindowsPath},
		{name: "generated drift wins", summary: "generated file differs", command: "go test ./...", want: FailureClassGeneratedDrift},
		{name: "frontend runtime wins", summary: "uncaught TypeError in browser console", command: "npm test", want: FailureClassFrontendRuntime},
		{name: "concurrency wins", summary: "WARNING: DATA RACE", command: "go test -race ./...", want: FailureClassConcurrency},
		{name: "auth wins", summary: "401 unauthorized", command: "go test ./...", want: FailureClassAuth},
		{name: "dependency wins", summary: "module not found", command: "go test ./...", want: FailureClassDependency},
		{name: "compiler wins", summary: "undefined: missingSymbol", command: "go test ./...", want: FailureClassCompiler},
		{name: "tests", summary: "--- FAIL: TestThing", command: "go test ./...", want: FailureClassTests},
		{name: "unknown", summary: "the check needs more evidence", command: "inspect", want: FailureClassUnknown},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ClassifyFailure(test.summary, test.command); got != test.want {
				t.Fatalf("ClassifyFailure() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestFailureFingerprintIsBoundedDeterministicAndCredentialIndependent(t *testing.T) {
	first := FailureFingerprint("  Test Failed:\nAPI key: ghp_12345678\n")
	second := FailureFingerprint("test   failed: api key: ghp_abcdefgh\n")
	if first == "" || first != second {
		t.Fatalf("fingerprints = %q and %q, want equal non-empty digests", first, second)
	}
	if len(first) != len(failureFingerprintPrefix)+12 || !strings.HasPrefix(first, failureFingerprintPrefix) {
		t.Fatalf("fingerprint format = %q", first)
	}
	if strings.Contains(first, "ghp_") || strings.Contains(first, "12345678") {
		t.Fatalf("fingerprint exposed credential-shaped input: %q", first)
	}

	long := strings.Repeat("x", maxFailureFingerprintInput) + "different suffix"
	if FailureFingerprint(long) != FailureFingerprint(long[:maxFailureFingerprintInput]) {
		t.Fatal("fingerprint changed after the bounded input limit")
	}
	if FailureFingerprint(" \n\t") != "" {
		t.Fatal("blank evidence produced a fingerprint")
	}
}

func TestFailureIntelligenceForTaskDerivesContiguousRepeatSignal(t *testing.T) {
	task := &taskstate.Task{
		Verification: []taskstate.Verification{
			failureVerification("FAIL: undefined: missingSymbol", "go test ./..."),
			failureVerification("fail:   undefined: missingSymbol", "go test ./internal/thing"),
		},
	}

	got := FailureIntelligenceForTask(task)
	if got.Class != FailureClassCompiler || got.RepeatCount != 2 {
		t.Fatalf("failure intelligence = %+v", got)
	}
	if !got.RequiresNewHypothesis() || !got.RequiresDifferentRoute() {
		t.Fatalf("repeat signal = %+v", got)
	}
	if got.Route != "inspect compiler output and affected source" {
		t.Fatalf("route = %q", got.Route)
	}
	if strings.Contains(got.Route, "missingSymbol") {
		t.Fatalf("evidence leaked into route: %q", got.Route)
	}
}

func TestFailureIntelligenceForTaskStopsAtPassAndDifferentFailure(t *testing.T) {
	tests := []struct {
		name         string
		verification []taskstate.Verification
		wantRepeat   int
		wantEmpty    bool
	}{
		{
			name: "pass boundary",
			verification: []taskstate.Verification{
				failureVerification("FAIL: assertion mismatch", "go test ./..."),
				{Passed: true, Summary: "VERIFY PASS go test ./..."},
				failureVerification("FAIL: assertion mismatch", "go test ./..."),
			},
			wantRepeat: 1,
		},
		{
			name: "different fingerprint boundary",
			verification: []taskstate.Verification{
				failureVerification("FAIL: assertion mismatch", "go test ./..."),
				failureVerification("FAIL: timeout waiting for server", "go test ./..."),
			},
			wantRepeat: 1,
		},
		{
			name: "inconclusive latest",
			verification: []taskstate.Verification{
				failureVerification("FAIL: assertion mismatch", "go test ./..."),
				{Summary: "VERIFY INCONCLUSIVE no runner", Command: "go test ./..."},
			},
			wantEmpty: true,
		},
		{
			name:         "no history",
			verification: nil,
			wantEmpty:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := FailureIntelligenceForTask(&taskstate.Task{Verification: test.verification})
			if test.wantEmpty {
				if got != (FailureIntelligence{}) {
					t.Fatalf("failure intelligence = %+v, want empty", got)
				}
				return
			}
			if got.RepeatCount != test.wantRepeat || got.Fingerprint == "" {
				t.Fatalf("failure intelligence = %+v", got)
			}
		})
	}
}

func TestFailureIntelligenceForTaskBoundsRepeatCount(t *testing.T) {
	verification := make([]taskstate.Verification, maxFailureRepeatCount+20)
	for i := range verification {
		verification[i] = failureVerification("FAIL: assertion mismatch", "go test ./...")
	}

	got := FailureIntelligenceForTask(&taskstate.Task{Verification: verification})
	if got.RepeatCount != maxFailureRepeatCount || !got.NeedsNewHypothesis || !got.NeedsDifferentRoute {
		t.Fatalf("bounded failure intelligence = %+v", got)
	}
}

func TestFailureIntelligenceForTaskIgnoresUnavailableEvidence(t *testing.T) {
	for _, summary := range []string{
		"VERIFY INCONCLUSIVE timeout",
		"VERIFY SKIPPED no supported runner",
	} {
		got := FailureIntelligenceForTask(&taskstate.Task{
			Verification: []taskstate.Verification{{Summary: summary, Command: "go test ./..."}},
		})
		if got != (FailureIntelligence{}) {
			t.Fatalf("summary %q produced failure intelligence %+v", summary, got)
		}
	}
}

func failureVerification(summary, command string) taskstate.Verification {
	return taskstate.Verification{Summary: summary, Command: command}
}
