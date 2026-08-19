package main

import (
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	if err := run([]string{"version"}); err != nil {
		t.Fatal(err)
	}
}

func TestRunRequiresPrompt(t *testing.T) {
	err := run([]string{"run"})
	if err == nil || !strings.Contains(err.Error(), "missing prompt") {
		t.Fatalf("%v", err)
	}
}
