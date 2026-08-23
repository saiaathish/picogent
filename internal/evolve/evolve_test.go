package evolve

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadDoesNotCreateStateUntilSave(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "missing", "picogent")
	t.Setenv("PICOGENT_HOME", home)
	store, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(home); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load created state directory: %v", err)
	}
	if err := Save(store); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "evolve")); err != nil {
		t.Fatalf("Save did not create state directory: %v", err)
	}
}

func TestUpsertHabitPrefersUpdate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PICOGENT_HOME", dir)
	ws := filepath.Join(dir, "proj")
	_ = os.MkdirAll(ws, 0o755)

	s := Store{Workspace: ws}
	var created bool
	var h Habit
	s, h, created = UpsertHabit(s, "Prefer go test after Go edits", "heuristic")
	if !created || h.Hits != 1 {
		t.Fatalf("expected new habit, got created=%v hits=%d", created, h.Hits)
	}
	s, h, created = UpsertHabit(s, "prefer go test after go edits", "reflect")
	if created || h.Hits != 2 {
		t.Fatalf("expected update, got created=%v hits=%d", created, h.Hits)
	}
	if err := Save(s); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(ws)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Habits) != 1 {
		t.Fatalf("want 1 habit, got %d", len(loaded.Habits))
	}
}

func TestUpsertPlaybookClassFirst(t *testing.T) {
	s := Store{Workspace: "/tmp/x"}
	var created bool
	var p Playbook
	s, p, created = UpsertPlaybook(s, "Verify Go changes", "step 1", "go-tests", "heuristic")
	if !created {
		t.Fatal("expected create")
	}
	s, p, created = UpsertPlaybook(s, "Go test loop", "step 1\nstep 2", "go-tests", "reflect")
	if created {
		t.Fatal("expected class-first update")
	}
	if p.Body != "step 1\nstep 2" || p.Hits != 2 {
		t.Fatalf("unexpected playbook: %+v", p)
	}
	if len(s.Playbooks) != 1 {
		t.Fatalf("want 1 playbook, got %d", len(s.Playbooks))
	}
}

func TestCurateCapsAndArchives(t *testing.T) {
	s := Store{Workspace: "/tmp/x"}
	now := time.Now().UTC()
	for i := 0; i < 12; i++ {
		s.Habits = append(s.Habits, Habit{
			ID:        idFor("h", string(rune('a'+i))),
			Text:      "habit " + string(rune('a'+i)),
			Hits:      i,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}
	old := now.Add(-120 * 24 * time.Hour)
	s.Playbooks = append(s.Playbooks, Playbook{
		ID: "old", Title: "Old", Body: "body", Hits: 0,
		CreatedAt: old, UpdatedAt: old,
	})
	s.Playbooks = append(s.Playbooks, Playbook{
		ID: "fresh", Title: "Fresh", Body: "body", Hits: 3,
		CreatedAt: now, UpdatedAt: now,
	})
	s = Curate(s)
	if len(s.Habits) > maxHabits {
		t.Fatalf("habits not capped: %d", len(s.Habits))
	}
	archived := false
	for _, p := range s.Playbooks {
		if p.ID == "old" && p.Archived {
			archived = true
		}
	}
	if !archived {
		t.Fatal("expected stale playbook archived")
	}
	prompt := Prompt(s)
	if !contains(prompt, "Fresh") {
		t.Fatalf("prompt missing fresh playbook: %s", prompt)
	}
	if contains(prompt, "Old") {
		t.Fatalf("prompt should omit archived: %s", prompt)
	}
}

func TestPromptBudgetAndRelevance(t *testing.T) {
	now := time.Now().UTC()
	s := Store{Workspace: "/tmp/x"}
	for i := 0; i < 5; i++ {
		s.Habits = append(s.Habits, Habit{
			ID: idFor("h", string(rune('a'+i))), Text: "habit " + string(rune('a'+i)) + " prefer small edits and verify",
			Hits: i, CreatedAt: now, UpdatedAt: now,
		})
	}
	s.Playbooks = append(s.Playbooks,
		Playbook{ID: "go", Title: "Verify Go", Class: "go-tests", Body: strings.Repeat("go test step ", 40), Hits: 2, CreatedAt: now, UpdatedAt: now},
		Playbook{ID: "js", Title: "Verify JS", Class: "js-verify", Body: "npm test loop", Hits: 9, CreatedAt: now, UpdatedAt: now},
	)
	p := PromptFor(s, "fix the failing go tests in internal/")
	if len(p) > maxPromptBytes {
		t.Fatalf("prompt too large: %d > %d\n%s", len(p), maxPromptBytes, p)
	}
	if !strings.Contains(p, "Verify Go") {
		t.Fatalf("expected relevant go playbook, got: %s", p)
	}
	if strings.Contains(p, "Verify JS") {
		t.Fatalf("should not inject irrelevant js playbook: %s", p)
	}
	// Cold + irrelevant playbook should stay out when hint is empty: still cap size.
	p2 := PromptFor(s, "")
	if len(p2) > maxPromptBytes {
		t.Fatalf("empty-hint prompt too large: %d", len(p2))
	}
}

func TestVerificationTargetsUseRelevantPlaybookPathsOnly(t *testing.T) {
	now := time.Now().UTC()
	s := Store{Workspace: "/tmp/x", Playbooks: []Playbook{
		{
			ID: "auth", Title: "Auth changes", Class: "auth", Hits: 4,
			Body:      "go test ./internal/auth then go test ./internal/session; go vet ./...",
			CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "js", Title: "Frontend changes", Class: "js", Hits: 1,
			Body: "npm test ./web/components", CreatedAt: now, UpdatedAt: now,
		},
	}}
	got := VerificationTargets(s, "fix the auth token bug")
	want := []string{"internal/auth", "internal/session"}
	if len(got) != len(want) {
		t.Fatalf("targets=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("targets=%v want=%v", got, want)
		}
	}
}

func TestVerificationTargetsSkipUnsafeOrNonPathTokens(t *testing.T) {
	s := Store{Playbooks: []Playbook{{
		Title: "Go", Body: "go test ./... go test ./internal/auth, https://example.test/x ../outside ./internal/auth/",
		Hits: 3,
	}}}
	got := VerificationTargets(s, "go auth")
	if len(got) != 1 || got[0] != "internal/auth" {
		t.Fatalf("targets=%v", got)
	}
}

func TestWorthReflecting(t *testing.T) {
	if WorthReflecting(Signal{UserPrompt: "hi"}) {
		t.Fatal("tiny turn should skip")
	}
	if !WorthReflecting(Signal{GoalDone: true}) {
		t.Fatal("goal done should reflect")
	}
	if WorthReflecting(Signal{FilesChanged: []string{"a.go", "b.go"}, ToolRounds: 2}) {
		t.Fatal("two-file edit without verify should skip")
	}
	if !WorthReflecting(Signal{FilesChanged: []string{"a.go"}, Verified: "ok", ToolRounds: 2}) {
		t.Fatal("verified edit should reflect")
	}
}

func TestHeuristicReflectGo(t *testing.T) {
	out := heuristicReflect(Signal{
		FilesChanged: []string{"internal/foo.go"},
		Verified:     "ok: 3 passed",
		ToolRounds:   3,
	})
	if out.Skip || out.Habit == "" || out.Playbook == "" {
		t.Fatalf("expected go heuristic learnings: %+v", out)
	}
}

func TestHeuristicSkipsTrivialMarkerGoals(t *testing.T) {
	out := heuristicReflect(Signal{
		GoalDone:     true,
		UserPrompt:   "Create file .picogent_qa_probe.txt with exactly qa_probe",
		FilesChanged: []string{".picogent_qa_probe.txt"},
		ToolRounds:   1,
	})
	if !out.Skip {
		t.Fatalf("dotfile probe goals must not create playbooks: %+v", out)
	}
	out = heuristicReflect(Signal{
		GoalDone:     true,
		UserPrompt:   "Create hello_goal.txt",
		FilesChanged: []string{"hello_goal.txt"},
		ToolRounds:   1,
	})
	if !out.Skip {
		t.Fatalf("plain marker txt goals must not create playbooks: %+v", out)
	}
	out = heuristicReflect(Signal{
		GoalDone:     true,
		UserPrompt:   "Add the handler",
		FilesChanged: []string{"internal/gui/server.go"},
		ToolRounds:   2,
	})
	if out.Skip || out.Title == "" || strings.HasPrefix(out.Title, "How:") {
		t.Fatalf("source goal-complete should learn with a clean title: %+v", out)
	}
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
