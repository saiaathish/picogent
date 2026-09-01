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
	if !strings.Contains(string(b), `id="perm"`) ||
		!strings.Contains(string(b), `data-allow="0"`) ||
		!strings.Contains(string(b), `data-allow="1"`) {
		t.Fatal("index missing rendered Safe-mode allow/deny controls")
	}
	if strings.Contains(string(b), "scope-card") {
		t.Fatal("index still contains the blocking scope picker")
	}
	for _, marker := range []string{`id="rail-side"`, `id="side-fab"`, "PicoChat"} {
		if strings.Contains(string(b), marker) {
			t.Fatalf("index still contains retired companion UI: %s", marker)
		}
	}
	if !strings.Contains(string(b), `<script src="/contracts.js"></script>`) {
		t.Fatal("index does not load executable web contracts before app.js")
	}
	js, err := gui.ReadWeb("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(js), "/api/sessions") || !strings.Contains(string(js), "viewEpoch") || !strings.Contains(string(js), `message || "Couldn't save mode"`) {
		t.Fatal("gui missing session client or new-chat race guard")
	}
	if !strings.Contains(string(js), "window.PicogentWebContracts?.createPrimaryEventDispatcher") ||
		!strings.Contains(string(js), "primaryEventDispatcher?.dispatch(e)") {
		t.Fatal("gui primary SSE events are not wired through executable contracts")
	}
	if !strings.Contains(string(js), "function renderRecentSessions()") || !strings.Contains(string(js), "setUndoAvailable(true)") || !strings.Contains(string(js), `prompt: "/undo"`) {
		t.Fatal("gui recovery controls are not wired to session resume and undo")
	}
	if !strings.Contains(string(js), `fetch("/api/permission"`) ||
		!strings.Contains(string(js), "t.dataset.allow === \"1\"") ||
		!strings.Contains(string(js), "permission_id: permissionID") ||
		!strings.Contains(string(js), "permEl.dataset.permissionId = e.permission_id") {
		t.Fatal("gui permission controls are not wired to the existing permission API")
	}
	if strings.Contains(string(js), "/api/scope") || strings.Contains(string(js), "scope_required") || strings.Contains(string(js), "hideScopeCard") || strings.Contains(string(js), "scope_notice") {
		t.Fatal("gui still contains blocking or duplicate client-side scope handling")
	}
	if !strings.Contains(string(js), "statusAnnouncerEl.textContent = text") {
		t.Fatal("gui does not announce accepted-turn status without moving focus")
	}
	for _, marker := range []string{"/api/sidechat", "sideOpen", "sideBusy", "side_delta", "PicoChat"} {
		if strings.Contains(string(js), marker) {
			t.Fatalf("gui still contains retired companion behavior: %s", marker)
		}
	}
	if !strings.Contains(string(js), "const wasThinking = thinkingEl.classList.contains(\"is-on\");") ||
		!strings.Contains(string(js), "if (!wasThinking) resetReasoning();") {
		t.Fatal("gui reconnect refresh can reset active-turn activity evidence")
	}
	if !strings.Contains(string(js), "async function refresh(reconcileHistory = false)") ||
		!strings.Contains(string(js), "const sessionChanged = nextSessionID !== sessionId;") ||
		!strings.Contains(string(js), "let historyReplayPending = false;") ||
		!strings.Contains(string(js), "let refreshGeneration = 0;") ||
		!strings.Contains(string(js), "let chatRequestsPending = 0;") ||
		!strings.Contains(string(js), "if (reconcileHistory || sessionChanged || historyReplayPending)") ||
		!strings.Contains(string(js), "const serverBusy = !!s.busy;") ||
		!strings.Contains(string(js), "const preserveLocalTurn = clientBusy && !sessionChanged") ||
		!strings.Contains(string(js), "historyReplayPending = serverBusy || chatRequestsPending > 0;") ||
		!strings.Contains(string(js), "busy = sessionChanged ? serverBusy : serverBusy || chatRequestsPending > 0;") ||
		!strings.Contains(string(js), "chatRequestsPending++;") ||
		!strings.Contains(string(js), "chatRequestsPending--;") ||
		!strings.Contains(string(js), "const generation = ++refreshGeneration;") ||
		!strings.Contains(string(js), "generation !== refreshGeneration") ||
		!strings.Contains(string(js), "refresh(true).catch(() => {});") {
		t.Fatal("gui reconnect refresh does not reconcile durable session history")
	}
	threadsStart := strings.Index(string(js), "async function loadThreads(")
	threadsEnd := strings.Index(string(js), "function renderThreads()")
	if threadsStart < 0 || threadsEnd < threadsStart || strings.Contains(string(js)[threadsStart:threadsEnd], "sessionId = data.current_id") ||
		!strings.Contains(string(js)[threadsStart:threadsEnd], "if (epoch !== viewEpoch || generation !== refreshGeneration) return;") {
		t.Fatal("chat-list refresh can overwrite the selected session")
	}
	projectsStart := strings.Index(string(js), "async function loadProjects(")
	projectsEnd := strings.Index(string(js), "async function applyProjectSwitch")
	if projectsStart < 0 || projectsEnd < projectsStart ||
		!strings.Contains(string(js)[projectsStart:projectsEnd], "if (epoch !== viewEpoch || generation !== refreshGeneration) return;") {
		t.Fatal("project-list refresh can repaint a newer selection")
	}
	for _, marker := range []string{"async function pickProjectFolder()", "async function switchProject(id)"} {
		start := strings.Index(string(js), marker)
		if start < 0 {
			t.Fatalf("missing project switch handler: %s", marker)
		}
		end := strings.Index(string(js)[start:], "\n}")
		if end < 0 || !strings.Contains(string(js)[start:start+end], "viewEpoch++") ||
			!strings.Contains(string(js)[start:start+end], "const epoch = viewEpoch") ||
			!strings.Contains(string(js)[start:start+end], "if (epoch !== viewEpoch) return;") {
			t.Fatalf("%s does not invalidate stale project requests", marker)
		}
	}
	refreshStart := strings.Index(string(js), "async function refresh(reconcileHistory = false)")
	refreshEnd := strings.Index(string(js), "let authPollTimer = null")
	if refreshStart < 0 || refreshEnd < refreshStart ||
		!strings.Contains(string(js)[refreshStart:refreshEnd], "if (epoch !== viewEpoch || generation !== refreshGeneration) return;") {
		t.Fatal("refresh does not guard asynchronous session-list updates")
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
	dispatchStart := strings.Index(string(js), "const primaryEventDispatcher")
	dispatchEnd := strings.Index(string(js), "function verificationPresentation")
	if dispatchStart < 0 || dispatchEnd < dispatchStart || !strings.Contains(string(js)[dispatchStart:dispatchEnd], "finishTurnUI();") {
		t.Fatal("primary done SSE contract does not finalize the turn UI")
	}
	contracts, err := gui.ReadWeb("web/contracts.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"createPrimaryEventDispatcher", `case "assistant_delta"`, `case "assistant_final"`, `case "done"`, `case "prompts_refresh"`, "mainPromptRequest"} {
		if !strings.Contains(string(contracts), marker) {
			t.Fatalf("web contracts missing %q", marker)
		}
	}
	styles, err := gui.ReadWeb("web/styles.css")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(styles), ".recent-recovery") || !strings.Contains(string(styles), ".turn-recovery") {
		t.Fatal("recovery controls are missing product styling")
	}
	for _, marker := range []string{".side-fab", ".drawer-side", ".side-chat", ".side-ask"} {
		if strings.Contains(string(styles), marker) {
			t.Fatalf("styles still contain retired companion UI: %s", marker)
		}
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
