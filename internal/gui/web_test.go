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
	if !strings.Contains(string(b), "Picogent") || !strings.Contains(string(b), "new-chat-top") {
		t.Fatal("index missing title or header new chat")
	}
	js, err := gui.ReadWeb("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(js), "/api/sessions") || !strings.Contains(string(js), "viewEpoch") || !strings.Contains(string(js), `message || "Couldn't save mode"`) {
		t.Fatal("gui missing session client or new-chat race guard")
	}
	settings, err := gui.ReadWeb("web/settings.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(settings), "Settings") || !strings.Contains(string(settings), "mode-override-hint") {
		t.Fatal("settings page missing")
	}
	settingsJS, err := gui.ReadWeb("web/settings.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(settingsJS), `message || "Couldn't save."`) || strings.Contains(string(settingsJS), "Applied (couldn’t write config file)") {
		t.Fatal("settings save failures are not shown truthfully")
	}
	setup, err := gui.ReadWeb("web/setup.html")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(setup), "Log in") || !strings.Contains(string(setup), "mode-override-hint") {
		t.Fatal("setup missing login")
	}
}
