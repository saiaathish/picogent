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
	if !strings.Contains(string(b), `id="status-announcer"`) || !strings.Contains(string(b), `aria-live="polite"`) {
		t.Fatal("index missing non-disruptive status announcer")
	}
	if !strings.Contains(string(b), `id="recent-recovery"`) || !strings.Contains(string(b), `id="open-chats"`) || !strings.Contains(string(b), `id="undo-turn"`) {
		t.Fatal("index missing visible recovery controls")
	}
	if strings.Contains(string(b), "scope-card") {
		t.Fatal("index still contains the blocking scope picker")
	}
	js, err := gui.ReadWeb("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(js), "/api/sessions") || !strings.Contains(string(js), "viewEpoch") || !strings.Contains(string(js), `message || "Couldn't save mode"`) {
		t.Fatal("gui missing session client or new-chat race guard")
	}
	if !strings.Contains(string(js), "function renderRecentSessions()") || !strings.Contains(string(js), "setUndoAvailable(true)") || !strings.Contains(string(js), `prompt: "/undo"`) {
		t.Fatal("gui recovery controls are not wired to session resume and undo")
	}
	if strings.Contains(string(js), "/api/scope") || strings.Contains(string(js), "scope_required") || strings.Contains(string(js), "hideScopeCard") || strings.Contains(string(js), "scope_notice") {
		t.Fatal("gui still contains blocking or duplicate client-side scope handling")
	}
	if !strings.Contains(string(js), "statusAnnouncerEl.textContent = text") {
		t.Fatal("gui does not announce accepted-turn status without moving focus")
	}
	if !strings.Contains(string(js), "function verificationPresentation(status)") ||
		!strings.Contains(string(js), `case "pass":`) ||
		!strings.Contains(string(js), `case "done": return { className: "is-pass", label: "PASS" }`) ||
		!strings.Contains(string(js), `case "skipped": return { className: "is-skipped", label: "SKIPPED" }`) ||
		!strings.Contains(string(js), `label: "INCONCLUSIVE"`) ||
		strings.Contains(string(js), `e.status === "fail" ? "is-fail" : "is-pass"`) {
		t.Fatal("gui verification rendering does not preserve all non-pass outcomes")
	}
	activityStart := strings.Index(string(js), "function updateActivityPanel()")
	activityEnd := strings.Index(string(js), "function addReasonStep(text)")
	if activityStart < 0 || activityEnd < activityStart || !strings.Contains(string(js)[activityStart:activityEnd], `activityComplete ? "Completed" : "Working…"`) {
		t.Fatal("activity panel does not replace an empty completed turn's Working label")
	}
	finishStart := strings.Index(string(js), "function finishTurnUI()")
	finishEnd := strings.Index(string(js), "/* ─── SSE ─── */")
	if finishStart < 0 || finishEnd < finishStart || !strings.Contains(string(js)[finishStart:finishEnd], "activityComplete = true;") || !strings.Contains(string(js)[finishStart:finishEnd], "updateActivityPanel();") {
		t.Fatal("completed turn UI does not finalize the activity panel")
	}
	doneStart := strings.Index(string(js), `if (e.type === "done")`)
	doneEnd := strings.Index(string(js), `if (e.type === "side_delta")`)
	if doneStart < 0 || doneEnd < doneStart || !strings.Contains(string(js)[doneStart:doneEnd], "finishTurnUI();") {
		t.Fatal("done SSE path does not finalize the turn UI")
	}
	styles, err := gui.ReadWeb("web/styles.css")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(styles), ".recent-recovery") || !strings.Contains(string(styles), ".turn-recovery") {
		t.Fatal("recovery controls are missing product styling")
	}
	if !strings.Contains(string(styles), ".test-result.is-skipped") || !strings.Contains(string(styles), ".test-result.is-inconclusive") {
		t.Fatal("unresolved verification states are missing distinct styling")
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
