package gui

import (
	"strings"
	"testing"
)

func TestAnswerHelpModes(t *testing.T) {
	docs, err := loadHelpDocs()
	if err != nil {
		t.Fatal(err)
	}
	ans := answerHelp(docs, "What's the difference between Safe and Fast?")
	if !ans.Matched {
		t.Fatalf("expected match, got %#v", ans)
	}
	if len(ans.TopicIDs) == 0 || ans.TopicIDs[0] != "modes" {
		t.Fatalf("expected modes topic, got %v", ans.TopicIDs)
	}
	if !strings.Contains(ans.Answer, "Safe") {
		t.Fatalf("answer missing Safe: %s", ans.Answer)
	}
}

func TestAnswerHelpSlash(t *testing.T) {
	docs, err := loadHelpDocs()
	if err != nil {
		t.Fatal(err)
	}
	ans := answerHelp(docs, "how do slash commands work")
	if !ans.Matched || ans.TopicIDs[0] != "slash" {
		t.Fatalf("expected slash, got %#v", ans)
	}
}

func TestAnswerHelpMiss(t *testing.T) {
	docs, err := loadHelpDocs()
	if err != nil {
		t.Fatal(err)
	}
	ans := answerHelp(docs, "xyzzy plumbers union 99")
	if ans.Matched {
		t.Fatalf("expected miss, got %#v", ans)
	}
}
