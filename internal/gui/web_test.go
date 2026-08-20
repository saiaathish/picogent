package gui_test

import (
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/gui"
)

func TestEmbeddedIndex(t *testing.T) {
	b, err := gui.ReadWeb("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "Picogent") {
		t.Fatal("index missing title")
	}
	js, err := gui.ReadWeb("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(js), "Codex connected") {
		t.Fatal("gui missing Codex badge")
	}
	setup, err := gui.ReadWeb("web/setup.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(setup), "Log in") {
		t.Fatal("setup missing login")
	}
}
