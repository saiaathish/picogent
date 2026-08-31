package gui

import "testing"

func TestEstimateETAIdle(t *testing.T) {
	got := estimateETA(false, 0, 0, 0, 0, "")
	if got == "" || got[:4] != "Idle" {
		t.Fatalf("expected idle ETA, got %q", got)
	}
}

func TestEstimateETABusy(t *testing.T) {
	got := estimateETA(true, 40, 2, 1, 0, "fix the login bug")
	if got == "" {
		t.Fatal("empty ETA")
	}
}

func TestFormatActivityLine(t *testing.T) {
	if got := formatActivityLine(1, 0, 0); got != "1 read" {
		t.Fatalf("got %q", got)
	}
	if got := formatActivityLine(2, 1, 3); got != "2 reads · 1 search · 3 edits" {
		t.Fatalf("got %q", got)
	}
}

func TestSummarizeMainChat(t *testing.T) {
	// empty
	if summarizeMainChat(nil, 4) == "" {
		t.Fatal("expected empty summary text")
	}
}
