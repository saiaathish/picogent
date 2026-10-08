package agent

import (
	"encoding/json"
	"errors"
	"os"
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

func TestTaskMutationRetryPreservesReplacementLegacyCompletion(t *testing.T) {
	const sessionID = "mutation-retry-replacement-owner"
	store := taskstate.NewStore(t.TempDir())
	original, err := taskstate.New(sessionID, "original task", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(original); err != nil {
		t.Fatal(err)
	}
	a := &Agent{TaskStore: store, TaskSession: sessionID, task: cloneTask(original)}
	owner := taskOwner(original)
	candidate := cloneTask(original)
	mutate := func(task *taskstate.Task) error {
		task.NoteAttempt()
		return nil
	}
	if err := mutate(candidate); err != nil {
		t.Fatal(err)
	}

	replacement, err := taskstate.New(sessionID, "replacement task", nil)
	if err != nil {
		t.Fatal(err)
	}
	replacement.Status = taskstate.StatusDone // an untrusted legacy terminal marker
	replacement.Revision = original.Revision + 1
	replacementBytes, err := json.Marshal(replacement)
	if err != nil {
		t.Fatal(err)
	}
	recordPath, err := store.Path(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recordPath, replacementBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	a.taskMu.Lock()
	_, err = a.persistTaskCandidateWithRetryLocked(candidate, mutate, func(current *taskstate.Task) bool {
		return taskOwner(current) == owner
	})
	a.taskMu.Unlock()
	if !errors.Is(err, errTaskOwnershipChanged) {
		t.Fatalf("replacement task mutation = %v, want ownership refusal", err)
	}
	gotBytes, err := os.ReadFile(recordPath)
	if err != nil || string(gotBytes) != string(replacementBytes) {
		t.Fatalf("rejected replacement was normalized or rewritten: err=%v", err)
	}
	// The generic mutator supplies a record-owner fence by default, while its
	// mutation retains any intent/turn checks needed for supersession.
	_, err = a.mutateTaskResult(mutate)
	if !errors.Is(err, errTaskOwnershipChanged) {
		t.Fatalf("replacement task mutation without a turn binding = %v, want ownership refusal", err)
	}
	gotBytes, err = os.ReadFile(recordPath)
	if err != nil || string(gotBytes) != string(replacementBytes) {
		t.Fatalf("generic replacement was normalized or rewritten: err=%v", err)
	}

	// The low-level helper also refuses to retry when a caller supplies no
	// ownership predicate at all.
	a.taskMu.Lock()
	_, err = a.persistTaskCandidateWithRetryLocked(candidate, mutate, nil)
	a.taskMu.Unlock()
	if !errors.Is(err, errTaskOwnershipChanged) {
		t.Fatalf("replacement task mutation without an explicit owner = %v, want ownership refusal", err)
	}
	gotBytes, err = os.ReadFile(recordPath)
	if err != nil || string(gotBytes) != string(replacementBytes) {
		t.Fatalf("replacement without explicit owner was normalized or rewritten: err=%v", err)
	}
	if got := a.task; got == nil || got.ID != original.ID || got.Revision != original.Revision {
		t.Fatalf("rejected mutation adopted replacement in memory: %#v", got)
	}
}

func containsFold(s, want string) bool {
	return len(s) >= len(want) && strings.Contains(strings.ToLower(s), strings.ToLower(want))
}
