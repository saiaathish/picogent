package gui

import "testing"

func TestParsePromptRecs(t *testing.T) {
	raw := "```json\n[{\"title\":\"Go tests\",\"subtitle\":\"Fix failures\",\"prompt\":\"Run go test and fix failures.\"}]\n```"
	recs := parsePromptRecs(raw)
	if len(recs) != 1 || recs[0].Title != "Go tests" {
		t.Fatalf("got %#v", recs)
	}
}

func TestHeuristicMainRecs(t *testing.T) {
	st := sideStatus{Workspace: "/tmp/x", Project: "x", ChatSummary: "empty"}
	recs := heuristicPromptRecs("main", "go.mod internal/ cmd/", st)
	if len(recs) == 0 {
		t.Fatal("expected heuristic recs")
	}
}

func TestLightModelIDFallback(t *testing.T) {
	s := &server{}
	// empty cfg still returns something or empty without panic
	_ = s.lightModelID()
}
