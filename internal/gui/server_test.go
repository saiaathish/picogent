package gui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/goal"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/scope"
	"github.com/saiaathish/picogent/internal/session"
	"github.com/saiaathish/picogent/internal/tools"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/taskstate"
)

func TestTaskProgressClearEventKeepsNullTaskEnvelope(t *testing.T) {
	raw, err := json.Marshal(event{Type: "task_progress", SessionID: "session-current"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"task":null`) {
		t.Fatalf("clear event = %s", raw)
	}
}

func TestGUIOnlyAcceptsLoopbackListenAddresses(t *testing.T) {
	for _, tc := range []struct {
		addr string
		want bool
	}{
		{addr: "127.0.0.1:7420", want: true},
		{addr: "[::1]:7420", want: true},
		{addr: "localhost:7420", want: true},
		{addr: "0.0.0.0:7420", want: false},
		{addr: ":7420", want: false},
		{addr: "192.168.1.20:7420", want: false},
	} {
		if got := loopbackListenAddress(tc.addr); got != tc.want {
			t.Errorf("loopbackListenAddress(%q) = %v, want %v", tc.addr, got, tc.want)
		}
	}
}

func TestSetModePersistsDeliberateChoiceWithEnvironmentOverride(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)
	t.Setenv("PICOGENT_CODEX_HOME", t.TempDir())
	t.Setenv("PICOGENT_MODE", "")
	t.Chdir(t.TempDir())

	user := config.Default()
	user.Mode = config.ModeFast
	user.Provider = config.ProviderOllama
	user.Workspace = t.TempDir()
	if err := config.Save(user); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PICOGENT_MODE", "safe")
	effective, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	gate := perm.New(effective.Mode, effective.Workspace, nil)
	ag := agent.New(effective, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: effective.Workspace}), gate)
	s := &server{cfg: effective, ag: ag, permCh: make(chan perm.Decision, 1)}
	state := s.snapshot()
	if state["mode"] != config.ModeSafe || state["saved_mode"] != config.ModeFast || state["mode_overridden"] != true {
		t.Fatalf("state mode fields = %#v", state)
	}
	res := httptest.NewRecorder()
	s.setMode(res, httptest.NewRequest(http.MethodPost, "/api/mode", strings.NewReader(`{"mode":"fast"}`)))
	if res.Code != http.StatusNoContent {
		t.Fatalf("set mode status = %d", res.Code)
	}
	if s.cfg.Mode != config.ModeSafe {
		t.Fatalf("server effective mode = %q, want environment mode %q", s.cfg.Mode, config.ModeSafe)
	}
	if got := ag.ConfigSnapshot().Mode; got != config.ModeSafe {
		t.Fatalf("agent effective mode = %q, want environment mode %q", got, config.ModeSafe)
	}
	if gate.Mode != config.ModeSafe {
		t.Fatalf("gate mode = %q, want environment mode %q", gate.Mode, config.ModeSafe)
	}

	t.Setenv("PICOGENT_MODE", "")
	reloaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Mode != config.ModeFast {
		t.Fatalf("saved mode = %q, want deliberate user mode %q", reloaded.Mode, config.ModeFast)
	}
}

func TestSetModeDoesNotReportOrApplyUnsavedChange(t *testing.T) {
	home := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(home, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PICOGENT_HOME", home)

	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	cfg.SetRuntimeMode(config.ModeFast)
	gate := perm.New(cfg.Mode, workspace, nil)
	ag := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), gate)
	s := &server{cfg: cfg, ag: ag, permCh: make(chan perm.Decision, 1)}

	res := httptest.NewRecorder()
	s.setMode(res, httptest.NewRequest(http.MethodPost, "/api/mode", strings.NewReader(`{"mode":"fast"}`)))
	if res.Code != http.StatusInternalServerError {
		t.Fatalf("set mode status = %d, body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "couldn't save mode") {
		t.Fatalf("set mode error = %q", res.Body.String())
	}
	if s.cfg.Mode != config.ModeFast || s.cfg.PersistentMode() != config.ModeSafe {
		t.Fatalf("server config changed after failed save: %#v", s.cfg)
	}
	if got := ag.ConfigSnapshot(); got.Mode != config.ModeFast || got.PersistentMode() != config.ModeSafe {
		t.Fatalf("agent config changed after failed save: %#v", got)
	}
	if gate.Mode != config.ModeFast {
		t.Fatalf("gate mode changed after failed save: %q", gate.Mode)
	}
}

func TestSettingsDoesNotReportOrApplyUnsavedModeChange(t *testing.T) {
	home := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(home, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PICOGENT_HOME", home)

	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	cfg.SetRuntimeMode(config.ModeFast)
	gate := perm.New(cfg.Mode, workspace, nil)
	ag := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), gate)
	s := &server{cfg: cfg, ag: ag, permCh: make(chan perm.Decision, 1)}

	res := httptest.NewRecorder()
	s.settings(res, httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(`{"mode":"fast"}`)))
	if res.Code != http.StatusInternalServerError {
		t.Fatalf("settings status = %d, body=%s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "couldn't save settings") {
		t.Fatalf("settings error = %q", res.Body.String())
	}
	if s.cfg.Mode != config.ModeFast || s.cfg.PersistentMode() != config.ModeSafe {
		t.Fatalf("server config changed after failed settings save: %#v", s.cfg)
	}
	if got := ag.ConfigSnapshot(); got.Mode != config.ModeFast || got.PersistentMode() != config.ModeSafe {
		t.Fatalf("agent config changed after failed settings save: %#v", got)
	}
	if gate.Mode != config.ModeFast {
		t.Fatalf("gate mode changed after failed settings save: %q", gate.Mode)
	}
}

func TestSettingsDoesNotReplaceWorkspaceWhenSaveFails(t *testing.T) {
	home := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(home, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PICOGENT_HOME", home)
	oldWorkspace := t.TempDir()
	newWorkspace := t.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = oldWorkspace
	ag := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: oldWorkspace}), perm.New(cfg.Mode, oldWorkspace, nil))
	s := &server{cfg: cfg, ag: ag, sessionID: "old-session", permCh: make(chan perm.Decision, 1)}

	res := httptest.NewRecorder()
	s.settings(res, httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(fmt.Sprintf(`{"workspace":%q}`, newWorkspace))))
	if res.Code != http.StatusInternalServerError {
		t.Fatalf("settings status = %d, body=%s", res.Code, res.Body.String())
	}
	s.mu.Lock()
	gotCfg, gotAgent, gotSession := s.cfg, s.ag, s.sessionID
	s.mu.Unlock()
	if gotCfg.Workspace != oldWorkspace || gotAgent != ag || gotSession != "old-session" {
		t.Fatalf("failed settings workspace change published state: cfg=%#v agentReplaced=%t session=%q", gotCfg, gotAgent != ag, gotSession)
	}
}

func TestSettingsAndModePersistenceAreSerialized(t *testing.T) {
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	gate := perm.New(cfg.Mode, workspace, nil)
	ag := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), gate)

	firstSaveStarted := make(chan struct{})
	releaseFirstSave := make(chan struct{})
	secondSaveStarted := make(chan struct{})
	var saves []config.Config
	var savesMu sync.Mutex
	s := &server{
		cfg:    cfg,
		ag:     ag,
		permCh: make(chan perm.Decision, 1),
		saveConfig: func(next config.Config) error {
			savesMu.Lock()
			index := len(saves)
			saves = append(saves, next)
			savesMu.Unlock()
			switch index {
			case 0:
				close(firstSaveStarted)
				<-releaseFirstSave
			case 1:
				close(secondSaveStarted)
			}
			return nil
		},
	}

	settingsRes := httptest.NewRecorder()
	settingsDone := make(chan struct{})
	go func() {
		s.settings(settingsRes, httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(`{"mode":"fast"}`)))
		close(settingsDone)
	}()
	<-firstSaveStarted

	modeRes := httptest.NewRecorder()
	modeDone := make(chan struct{})
	go func() {
		s.setMode(modeRes, httptest.NewRequest(http.MethodPost, "/api/mode", strings.NewReader(`{"mode":"safe"}`)))
		close(modeDone)
	}()

	select {
	case <-secondSaveStarted:
		close(releaseFirstSave)
		<-settingsDone
		<-modeDone
		t.Fatal("mode save began before the settings transaction completed")
	case <-time.After(10 * time.Second):
	}
	close(releaseFirstSave)
	select {
	case <-settingsDone:
	case <-time.After(10 * time.Second):
		t.Fatal("settings transaction did not finish")
	}
	select {
	case <-modeDone:
	case <-time.After(10 * time.Second):
		t.Fatal("mode transaction did not finish")
	}
	if settingsRes.Code != http.StatusOK || modeRes.Code != http.StatusNoContent {
		t.Fatalf("settings/mode status = %d/%d", settingsRes.Code, modeRes.Code)
	}
	savesMu.Lock()
	defer savesMu.Unlock()
	if len(saves) != 2 || saves[0].PersistentMode() != config.ModeFast || saves[1].PersistentMode() != config.ModeSafe {
		t.Fatalf("save order = %#v, want fast then safe", saves)
	}
	if s.cfg.PersistentMode() != config.ModeSafe || ag.ConfigSnapshot().PersistentMode() != config.ModeSafe || gate.Mode != config.ModeSafe {
		t.Fatalf("final live state did not match final saved mode: server=%#v agent=%#v gate=%q", s.cfg, ag.ConfigSnapshot(), gate.Mode)
	}
}

func TestSettingsSavesBaselineModeDuringEnvironmentOverride(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("PICOGENT_HOME", home)
	t.Setenv("PICOGENT_CODEX_HOME", t.TempDir())
	t.Setenv("PICOGENT_MODE", "")
	t.Chdir(t.TempDir())

	user := config.Default()
	user.Mode = config.ModeSafe
	user.Provider = config.ProviderOllama
	user.Workspace = workspace
	if err := config.Save(user); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PICOGENT_MODE", "fast")
	effective, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	gate := perm.New(effective.Mode, workspace, nil)
	ag := agent.New(effective, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), gate)
	s := &server{cfg: effective, ag: ag, permCh: make(chan perm.Decision, 1)}

	get := httptest.NewRecorder()
	s.settings(get, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if get.Code != http.StatusOK {
		t.Fatalf("settings get status = %d", get.Code)
	}
	var settings map[string]any
	if err := json.NewDecoder(get.Body).Decode(&settings); err != nil {
		t.Fatal(err)
	}
	if got := settings["mode"]; got != string(config.ModeSafe) {
		t.Fatalf("settings saved mode = %#v, want %q", got, config.ModeSafe)
	}
	if got := settings["active_mode"]; got != string(config.ModeFast) {
		t.Fatalf("settings active mode = %#v, want %q", got, config.ModeFast)
	}
	if got, _ := settings["mode_overridden"].(bool); !got {
		t.Fatalf("settings mode_overridden = %#v, want true", settings["mode_overridden"])
	}

	post := httptest.NewRecorder()
	s.settings(post, httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(`{"mode":"fast"}`)))
	if post.Code != http.StatusOK {
		t.Fatalf("settings post status = %d, body=%s", post.Code, post.Body.String())
	}
	if s.cfg.Mode != config.ModeFast {
		t.Fatalf("server effective mode = %q, want environment mode %q", s.cfg.Mode, config.ModeFast)
	}
	if got := ag.ConfigSnapshot().Mode; got != config.ModeFast {
		t.Fatalf("agent effective mode = %q, want environment mode %q", got, config.ModeFast)
	}
	if gate.Mode != config.ModeFast {
		t.Fatalf("gate mode = %q, want environment mode %q", gate.Mode, config.ModeFast)
	}

	t.Setenv("PICOGENT_MODE", "")
	reloaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Mode != config.ModeFast {
		t.Fatalf("saved mode = %q, want settings selection %q", reloaded.Mode, config.ModeFast)
	}
}

func TestSetupFinishRetainsSavedModeDuringEnvironmentOverride(t *testing.T) {
	for _, tc := range []struct {
		name      string
		persisted config.Mode
		override  config.Mode
	}{
		{name: "safe preference beneath fast environment", persisted: config.ModeSafe, override: config.ModeFast},
		{name: "fast preference beneath safe environment", persisted: config.ModeFast, override: config.ModeSafe},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			workspace := t.TempDir()
			t.Setenv("PICOGENT_HOME", home)
			t.Setenv("PICOGENT_CODEX_HOME", t.TempDir())
			t.Setenv("PICOGENT_MODE", "")
			t.Setenv("PICOGENT_MODEL", "")
			t.Chdir(t.TempDir())

			user := config.Default()
			user.Mode = tc.persisted
			user.Provider = config.ProviderOllama
			user.Model = "qwen2.5-coder:7b"
			user.Workspace = workspace
			if err := config.Save(user); err != nil {
				t.Fatal(err)
			}

			t.Setenv("PICOGENT_MODE", string(tc.override))
			effective, err := config.Load()
			if err != nil {
				t.Fatal(err)
			}
			s := &server{cfg: effective, permCh: make(chan perm.Decision, 1), sessionID: "setup-session"}

			status := httptest.NewRecorder()
			s.setupStatus(status, httptest.NewRequest(http.MethodGet, "/api/setup", nil))
			if status.Code != http.StatusOK {
				t.Fatalf("setup status = %d", status.Code)
			}
			var snapshot struct {
				Mode           string `json:"mode"`
				ActiveMode     string `json:"active_mode"`
				ModeOverridden bool   `json:"mode_overridden"`
				Model          string `json:"model"`
			}
			if err := json.NewDecoder(status.Body).Decode(&snapshot); err != nil {
				t.Fatal(err)
			}
			if snapshot.Mode != string(tc.persisted) || snapshot.ActiveMode != string(tc.override) || !snapshot.ModeOverridden {
				t.Fatalf("setup snapshot = %#v, want saved=%q active=%q overridden=true", snapshot, tc.persisted, tc.override)
			}

			body, err := json.Marshal(map[string]string{"workspace": workspace, "mode": snapshot.Mode, "model": snapshot.Model})
			if err != nil {
				t.Fatal(err)
			}
			finish := httptest.NewRecorder()
			s.setupFinish(finish, httptest.NewRequest(http.MethodPost, "/api/setup/finish", strings.NewReader(string(body))))
			if finish.Code != http.StatusNoContent {
				t.Fatalf("setup finish status = %d, body=%s", finish.Code, finish.Body.String())
			}
			if s.cfg.Mode != tc.override {
				t.Fatalf("server effective mode = %q, want environment mode %q", s.cfg.Mode, tc.override)
			}
			if s.ag == nil || s.ag.ConfigSnapshot().Mode != tc.override {
				t.Fatalf("setup agent did not retain effective environment mode %q", tc.override)
			}

			t.Setenv("PICOGENT_MODE", "")
			reloaded, err := config.Load()
			if err != nil {
				t.Fatal(err)
			}
			if reloaded.Mode != tc.persisted || !reloaded.SetupComplete {
				t.Fatalf("reloaded config = %#v, want saved mode %q and completed setup", reloaded, tc.persisted)
			}
		})
	}
}

func TestFollowUpQueuePreservesFIFOAndBounds(t *testing.T) {
	s := &server{}
	mode := agent.TaskPlan
	if !s.queueSteerScoped("first", []llm.Part{{Type: "text", Text: "one"}}, "first display", &mode, true, "scope notice", "scope boundary") {
		t.Fatal("first follow-up was rejected")
	}
	if first := s.steerQueue[0]; !first.automaticScope || first.scopeNotice != "scope notice" || first.scopeBoundary != "scope boundary" {
		t.Fatalf("queued scope metadata = %#v", first)
	}
	if !s.queueSteer("second", nil, "second display") {
		t.Fatal("second follow-up was rejected")
	}
	for i := 2; i < maxQueuedTurns; i++ {
		if !s.queueSteer(fmt.Sprintf("queued-%d", i), nil, "") {
			t.Fatalf("follow-up %d was rejected before queue bound", i)
		}
	}
	if s.queueSteer("overflow", nil, "") {
		t.Fatal("follow-up queue accepted an entry past its bound")
	}

	prompt, parts, display, gotMode, ok := s.popSteer()
	if !ok || prompt != "first" || display != "first display" || len(parts) != 1 || gotMode == nil || *gotMode != mode {
		t.Fatalf("first queued turn = %q, %#v, %q, %#v, %v", prompt, parts, display, gotMode, ok)
	}
	prompt, _, display, gotMode, ok = s.popSteer()
	if !ok || prompt != "second" || display != "second display" || gotMode != nil {
		t.Fatalf("second queued turn = %q, %q, %#v, %v", prompt, display, gotMode, ok)
	}
}

func TestQueuedFollowUpCannotBeLostDuringDoneHandoff(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	scripted := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", Content: "first done"}},
		{Message: llm.Message{Role: "assistant", Content: "second done"}},
		{Message: llm.Message{Role: "assistant", Content: "third done"}},
	}}
	ag := agent.New(cfg, scripted, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	events := make(chan event)
	s := &server{
		cfg:       cfg,
		ag:        ag,
		sessionID: "handoff-session",
		permCh:    make(chan perm.Decision, 1),
		subs:      []chan event{events},
	}
	if err := (&session.Session{ID: "handoff-session", Title: "Pinned", Workspace: workspace}).Save(); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	s.beforeAgentRun = func() {
		s.mu.Lock()
		if s.ag != nil {
			s.ag.SetClient(scripted)
		}
		s.mu.Unlock()
		first := false
		once.Do(func() {
			first = true
			close(entered)
		})
		if first {
			<-release
		}
	}
	finalDone := make(chan struct{})
	go func() {
		doneCount := 0
		for e := range events {
			if e.Type != "done" {
				continue
			}
			if doneCount == 0 {
				doneCount++
				// A competing request is allowed to arrive exactly when the
				// browser observes done. A correct handoff keeps the queued
				// second turn ahead of this third request.
				go s.startAgentTurn("third", nil)
				continue
			}
			close(finalDone)
			return
		}
	}()

	s.startAgentTurn("first", nil)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first turn did not reach its barrier")
	}
	if !s.queueSteer("second", nil, "second") {
		t.Fatal("failed to queue second turn")
	}
	close(release)

	select {
	case <-finalDone:
	case <-time.After(15 * time.Second):
		s.mu.Lock()
		t.Fatalf("turn handoff did not finish: busy=%v active=%d queue=%d hist=%#v calls=%d", s.busy, s.activeTurns, len(s.steerQueue), s.hist, len(scripted.Calls))
		s.mu.Unlock()
	}
	s.mu.Lock()
	hist := append([]llm.Message(nil), s.hist...)
	queueLen := len(s.steerQueue)
	active := s.activeTurns
	s.mu.Unlock()
	if queueLen != 0 || active != 0 {
		t.Fatalf("handoff state queue=%d active=%d", queueLen, active)
	}
	var users []string
	for _, msg := range hist {
		if msg.Role == "user" {
			users = append(users, msg.Content)
		}
	}
	if !strings.Contains(strings.Join(users, "\n"), "second") || !strings.Contains(strings.Join(users, "\n"), "third") {
		t.Fatalf("queued turns were not preserved: users=%v", users)
	}
}

func TestCanceledTurnCannotPersistInferredState(t *testing.T) {
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	ag := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	s := &server{cfg: cfg, ag: ag, sessionID: "canceled", turnGen: 2}
	s.autoApplyFromUserPrompt("fix all failing tests", 1)
	if got := ag.GoalSnapshot(); got != "" {
		t.Fatalf("stale canceled turn applied goal %q", got)
	}
	if got, _ := goal.Load(workspace); got != "" {
		t.Fatalf("stale canceled turn persisted goal %q", got)
	}
}

func TestGUIInferenceSaveFailureDrainsQueuedTurns(t *testing.T) {
	home := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(home, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PICOGENT_HOME", home)
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	ag := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	s := &server{
		cfg:         cfg,
		ag:          ag,
		busy:        true,
		activeTurns: 1,
		turnGen:     1,
		sessionID:   "queued-save-failure",
		steerQueue: []queuedTurn{{
			prompt:  "fix all failing tests",
			display: "fix all failing tests",
		}},
	}
	admitted := turnAdmission{
		runAgent:   ag,
		runSession: s.sessionID,
		workspace:  workspace,
		myGen:      1,
		ctx:        context.Background(),
		goalEpoch:  0,
	}
	s.runAdmittedTurn(admitted, "fix all failing tests", nil, "fix all failing tests", nil)
	s.mu.Lock()
	active, busy, pending := s.activeTurns, s.busy, len(s.steerQueue)
	s.mu.Unlock()
	if active != 0 || busy || pending != 0 {
		t.Fatalf("save failure stranded queue: active=%d busy=%v pending=%d", active, busy, pending)
	}
}

func TestCompletionPromptPersistsDurableGoalInGUI(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	ag := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	s := &server{cfg: cfg, ag: ag, sessionID: "completion-goal", turnGen: 1}

	for _, prompt := range []string{"finish this project", "finish the project"} {
		t.Run(prompt, func(t *testing.T) {
			if err := goal.Clear(workspace); err != nil {
				t.Fatal(err)
			}
			ag.SetGoal("")
			s.autoApplyFromUserPrompt(prompt, 1)
			if got := ag.GoalSnapshot(); got != prompt {
				t.Fatalf("agent goal = %q, want %q", got, prompt)
			}
			if got, err := goal.Load(workspace); err != nil || got != prompt {
				t.Fatalf("stored goal = %q, err=%v", got, err)
			}
		})
	}
}

func TestGUIStaleCompletedTurnCannotClearNewerGoal(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	ag := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	ag.SetGoal("newer project goal")
	if err := goal.Set(workspace, "newer project goal"); err != nil {
		t.Fatal(err)
	}
	s := &server{cfg: cfg, ag: ag, turnGen: 2}
	if err := s.clearGoalIf("finish this project", 0, 0, 2); err != nil {
		t.Fatal(err)
	}
	if got, _ := goal.Load(workspace); got != "newer project goal" || ag.GoalSnapshot() != "newer project goal" {
		t.Fatalf("stale completion erased newer goal: stored=%q agent=%q", got, ag.GoalSnapshot())
	}
}

func TestGUISameTextGoalReplacementCannotClearOlderCompletion(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	ag := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	firstRevision, err := goal.SetState(workspace, "finish this project")
	if err != nil {
		t.Fatal(err)
	}
	ag.SetGoalState("finish this project", firstRevision)
	s := &server{cfg: cfg, ag: ag, turnGen: 2}
	oldEpoch := s.goalEpoch
	if err := s.setGoal("finish this project"); err != nil {
		t.Fatal(err)
	}
	state, err := goal.LoadState(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if state.Revision == firstRevision || s.goalEpoch == oldEpoch {
		t.Fatalf("same-text replacement did not advance identity: old=%d new=%d epoch=%d", firstRevision, state.Revision, s.goalEpoch)
	}
	replacementRevision := state.Revision
	if err := s.clearGoalIf("finish this project", firstRevision, oldEpoch, 2); err != nil {
		t.Fatal(err)
	}
	state, err = goal.LoadState(workspace)
	if err != nil || state.Text != "finish this project" || state.Revision != replacementRevision {
		t.Fatalf("stale completion erased same-text replacement: %#v err=%v", state, err)
	}
}

func TestGUIQueuedSameTextGoalReplacementSurvivesOlderCompletion(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	firstRevision, err := goal.SetState(workspace, "finish this project")
	if err != nil {
		t.Fatal(err)
	}
	scripted := &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "Goal complete: done"}}}}
	reg := tools.NewRegistry(tools.Context{
		Workspace: workspace,
		Verify: func(context.Context) (string, error) {
			return "verify PASS", nil
		},
	})
	gate := perm.New(config.ModeFast, workspace, nil)
	gate.AddAlwaysAllowed("verify")
	ag := agent.New(cfg, scripted, reg, gate)
	ag.SetGoalState("finish this project", firstRevision)
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	s := &server{
		cfg:       cfg,
		ag:        ag,
		sessionID: "same-text-aba",
		permCh:    make(chan perm.Decision, 1),
	}
	s.beforeAgentRun = func() {
		s.mu.Lock()
		if s.ag != nil {
			s.ag.SetClient(scripted)
		}
		s.mu.Unlock()
		first := false
		once.Do(func() {
			first = true
			close(entered)
		})
		if first {
			<-release
		}
	}

	s.startAgentTurn("work under the current goal", nil)
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("initial turn did not reach the pre-run barrier")
	}
	queued := httptest.NewRecorder()
	s.chat(queued, httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"prompt":"/goal finish this project"}`)))
	if queued.Code != http.StatusAccepted {
		t.Fatalf("queued /goal status = %d, body=%s", queued.Code, queued.Body.String())
	}
	var queuedBody struct {
		Queued bool `json:"queued"`
	}
	if err := json.Unmarshal(queued.Body.Bytes(), &queuedBody); err != nil {
		t.Fatal(err)
	}
	if !queuedBody.Queued {
		t.Fatalf("queued /goal response = %s", queued.Body.String())
	}
	replacement, err := goal.LoadState(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if replacement.Text != "finish this project" || replacement.Revision == firstRevision {
		t.Fatalf("same-text replacement state = %#v, old revision=%d", replacement, firstRevision)
	}
	close(release)

	deadline := time.Now().Add(15 * time.Second)
	for {
		s.mu.Lock()
		active, pending := s.activeTurns, len(s.steerQueue)
		s.mu.Unlock()
		if active == 0 && pending == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("queued same-text goal did not finish: active=%d pending=%d", active, pending)
		}
		time.Sleep(5 * time.Millisecond)
	}
	final, err := goal.LoadState(workspace)
	if err != nil || final.Text != replacement.Text || final.Revision != replacement.Revision {
		t.Fatalf("older completion erased queued same-text goal: %#v err=%v", final, err)
	}
	currentGoal, currentRevision := ag.GoalStateSnapshot()
	if currentGoal != replacement.Text || currentRevision != replacement.Revision {
		t.Fatalf("live agent goal = %q/%d, want %q/%d", currentGoal, currentRevision, replacement.Text, replacement.Revision)
	}
}

func TestSessionLoadRequiresCurrentWorkspace(t *testing.T) {
	current := t.TempDir()
	other := t.TempDir()
	if !sessionWorkspaceMatches(&session.Session{Workspace: current}, current) {
		t.Fatal("matching workspace was rejected")
	}
	if sessionWorkspaceMatches(&session.Session{Workspace: other}, current) {
		t.Fatal("foreign workspace was accepted")
	}
	if sessionWorkspaceMatches(&session.Session{Workspace: ""}, current) {
		t.Fatal("empty session workspace was accepted")
	}
}

func TestChatRejectsInvalidExplicitScopeChoice(t *testing.T) {
	s := &server{cfg: config.Config{Workspace: t.TempDir()}}
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"prompt":"build something","scope_choice":"everything"}`))
	s.chat(res, req)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusBadRequest)
	}
}

func TestChatDefaultsScopeAndKeepsTranscriptReadable(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	client := &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "done"}}}}
	ag := agent.New(cfg, client, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	s := &server{
		cfg:       cfg,
		ag:        ag,
		sessionID: "scope-session",
		permCh:    make(chan perm.Decision, 1),
		subs:      []chan event{make(chan event, 64)},
	}
	events := s.subs[0]
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"prompt":"build something"}`))
	s.chat(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusAccepted)
	}
	var body struct {
		ScopeNotice string `json:"scope_notice"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.ScopeNotice != "Starting with a small working version by default." {
		t.Fatalf("scope notice = %q", body.ScopeNotice)
	}
	// The first GUI turn may have to initialize trace/extension state on a
	// slower Windows or macOS runner; allow startup variance without changing
	// the actual turn contract.
	deadline := time.Now().Add(10 * time.Second)
	for {
		s.mu.Lock()
		active := s.activeTurns
		hist := append([]llm.Message(nil), s.hist...)
		s.mu.Unlock()
		if active == 0 {
			if len(hist) < 2 {
				t.Fatalf("history = %#v", hist)
			}
			if hist[len(hist)-2].Role != "user" || hist[len(hist)-2].Content != "build something" {
				t.Fatalf("saved user message leaked scope guidance: %#v", hist[len(hist)-2])
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("scoped turn did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
	noticeIndex, assistantIndex, noticeCount := -1, -1, 0
	for index := 0; ; index++ {
		select {
		case e := <-events:
			if e.Type == "system" && e.Text == body.ScopeNotice {
				noticeCount++
				if noticeIndex < 0 {
					noticeIndex = index
				}
			}
			if (e.Type == "assistant_delta" || e.Type == "assistant" || e.Type == "assistant_final") && assistantIndex < 0 {
				assistantIndex = index
			}
		default:
			if noticeIndex < 0 {
				t.Fatal("automatic scope notice was not published to the event stream")
			}
			if noticeCount != 1 {
				t.Fatalf("automatic scope notice event count = %d, want 1", noticeCount)
			}
			if assistantIndex < 0 {
				t.Fatal("scoped turn did not publish assistant output")
			}
			if noticeIndex > assistantIndex {
				t.Fatalf("scope notice event index %d followed assistant event index %d", noticeIndex, assistantIndex)
			}
			goto checkedEventOrder
		}
	}

checkedEventOrder:
	if len(client.Calls) == 0 {
		t.Fatal("defaulted scope did not reach the model")
	}
	var scoped string
	for i := len(client.Calls[0].Messages) - 1; i >= 0; i-- {
		if client.Calls[0].Messages[i].Role == "user" {
			scoped = client.Calls[0].Messages[i].Content
			break
		}
	}
	if !strings.Contains(scoped, "Picogent scope choice: A small working version") {
		t.Fatalf("model prompt = %q, want recommended scope guidance", scoped)
	}
}

func TestAutomaticScopePrioritizesFocusedTurnOverDurableGoal(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	scripted := &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "done"}}}}
	ag := agent.New(cfg, scripted, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	s := &server{cfg: cfg, ag: ag, sessionID: "scope-boundary", permCh: make(chan perm.Decision, 1)}
	res := httptest.NewRecorder()
	const broadGoal = "fix all flaky tests and make CI green"
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"prompt":"`+broadGoal+`"}`))
	s.chat(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusAccepted)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		s.mu.Lock()
		active := s.activeTurns
		s.mu.Unlock()
		if active == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("automatic scoped turn did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := ag.GoalSnapshot(); got != broadGoal {
		t.Fatalf("automatic scope goal = %q, want %q", got, broadGoal)
	}
	if got, _ := goal.Load(workspace); got != broadGoal {
		t.Fatalf("automatic scope saved goal = %q, want %q", got, broadGoal)
	}
	if len(scripted.Calls) == 0 {
		t.Fatal("automatic scoped turn did not reach the model")
	}
	var system string
	for _, message := range scripted.Calls[0].Messages {
		if message.Role == "system" {
			system = message.Content
			break
		}
	}
	boundary := scope.TurnBoundary(scope.Choice{Label: "A focused fix"})
	goalAt := strings.Index(system, broadGoal)
	boundaryAt := strings.Index(system, boundary)
	if goalAt < 0 || boundaryAt <= goalAt {
		t.Fatalf("system prompt did not prioritize turn boundary: %q", system)
	}
}

func TestQueuedAutomaticScopeRunsAfterOneOrderedNotice(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	scripted := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", Content: "first done"}},
		{Message: llm.Message{Role: "assistant", Content: "scoped second done"}},
	}}
	ag := agent.New(cfg, scripted, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	events := make(chan event, 128)
	s := &server{cfg: cfg, ag: ag, sessionID: "queued-scope-order", permCh: make(chan perm.Decision, 1), subs: []chan event{events}}
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	s.beforeAgentRun = func() {
		s.mu.Lock()
		if s.ag != nil {
			s.ag.SetClient(scripted)
		}
		s.mu.Unlock()
		first := false
		once.Do(func() {
			first = true
			close(entered)
		})
		if first {
			<-release
		}
	}

	s.startAgentTurn("first", nil)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first turn did not reach its barrier")
	}

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"prompt":"build something"}`))
	s.chat(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusAccepted)
	}
	var body struct {
		Queued      bool   `json:"queued"`
		ScopeNotice string `json:"scope_notice"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Queued || body.ScopeNotice == "" {
		t.Fatalf("queued response = %#v", body)
	}
	close(release)

	deadline := time.Now().Add(15 * time.Second)
	for {
		s.mu.Lock()
		active := s.activeTurns
		queued := len(s.steerQueue)
		s.mu.Unlock()
		if active == 0 && queued == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("queued scoped turn did not finish: active=%d queued=%d", active, queued)
		}
		time.Sleep(5 * time.Millisecond)
	}

	noticeIndex, assistantIndex, noticeCount := -1, -1, 0
	var seen []string
	for index := 0; ; index++ {
		select {
		case e := <-events:
			seen = append(seen, e.Type+":"+e.Text)
			if e.Type == "system" && e.Text == body.ScopeNotice {
				noticeCount++
				if noticeIndex < 0 {
					noticeIndex = index
				}
			}
			if strings.HasPrefix(e.Type, "assistant") && noticeIndex >= 0 && assistantIndex < 0 {
				assistantIndex = index
			}
		default:
			if noticeCount != 1 || noticeIndex < 0 {
				t.Fatalf("queued scope notice count/index = %d/%d", noticeCount, noticeIndex)
			}
			if assistantIndex < 0 || noticeIndex > assistantIndex {
				t.Fatalf("queued scope notice/assistant order = %d/%d; events=%v", noticeIndex, assistantIndex, seen)
			}
			var scopedSystem string
			for _, call := range scripted.Calls {
				for _, message := range call.Messages {
					if message.Role == "user" && strings.Contains(message.Content, "Picogent scope choice: A small working version") {
						for _, candidate := range call.Messages {
							if candidate.Role == "system" {
								scopedSystem = candidate.Content
								break
							}
						}
						break
					}
				}
			}
			if !strings.Contains(scopedSystem, scope.TurnBoundary(scope.Choice{Label: "A small working version"})) {
				t.Fatalf("queued scoped call lost turn boundary: %q", scopedSystem)
			}
			return
		}
	}
}

func TestAutomaticScopeKeepsPlanIntent(t *testing.T) {
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	ag := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "done"}}}}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	s := &server{cfg: cfg, ag: ag, liveTask: agent.TaskAgent, sessionID: "scope-plan", permCh: make(chan perm.Decision, 1)}
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"prompt":"build something, but plan it first"}`))
	s.chat(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusAccepted)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		s.mu.Lock()
		active := s.activeTurns
		s.mu.Unlock()
		if active == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("automatic scoped turn did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := ag.TaskModeSnapshot(); got != agent.TaskPlan {
		t.Fatalf("task mode = %q, want plan", got)
	}
}

func TestAutomaticScopePreservesSelectedTaskBoundary(t *testing.T) {
	for _, tt := range []struct {
		prompt string
		mode   agent.TaskMode
	}{
		{"create something", agent.TaskPlan},
		{"fix everything", agent.TaskAsk},
		{"remove everything", agent.TaskDebug},
	} {
		t.Run(tt.mode.Label(), func(t *testing.T) {
			workspace := t.TempDir()
			cfg := config.Default()
			cfg.Provider = config.ProviderOllama
			cfg.Workspace = workspace
			ag := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "done"}}}}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
			ag.SetTaskMode(tt.mode)
			s := &server{cfg: cfg, ag: ag, liveTask: tt.mode, sessionID: "scope-boundary", permCh: make(chan perm.Decision, 1)}
			res := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"prompt":`+strconv.Quote(tt.prompt)+`}`))
			s.chat(res, req)
			if res.Code != http.StatusAccepted {
				t.Fatalf("status = %d, want %d", res.Code, http.StatusAccepted)
			}
			deadline := time.Now().Add(10 * time.Second)
			for {
				s.mu.Lock()
				active := s.activeTurns
				s.mu.Unlock()
				if active == 0 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("automatic scoped turn did not finish")
				}
				time.Sleep(5 * time.Millisecond)
			}
			if got := ag.TaskModeSnapshot(); got != tt.mode {
				t.Fatalf("task mode = %q, want preserved %q", got, tt.mode)
			}
		})
	}
}

func TestScopedTurnReportsTemporaryTaskModeAndRestoresIt(t *testing.T) {
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	events := make(chan event, 32)
	ag := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "done"}}}}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	s := &server{
		cfg:       cfg,
		ag:        ag,
		liveTask:  agent.TaskAgent,
		sessionID: "temporary-mode",
		permCh:    make(chan perm.Decision, 1),
		subs:      []chan event{events},
	}
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"prompt":"build something","scope_choice":"plan"}`))
	s.chat(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusAccepted)
	}
	// Keep this asynchronous boundary tolerant of first-run runtime startup;
	// the test still fails if the turn genuinely never unwinds.
	deadline := time.Now().Add(10 * time.Second)
	for {
		s.mu.Lock()
		active := s.activeTurns
		s.mu.Unlock()
		if active == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("scoped turn did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
	foundPlan, foundAgent := false, false
	eventDeadline := time.NewTimer(2 * time.Second)
	defer eventDeadline.Stop()
	for !foundPlan || !foundAgent {
		select {
		case e := <-events:
			if e.Type != "task_mode" {
				continue
			}
			foundPlan = foundPlan || e.Text == string(agent.TaskPlan)
			foundAgent = foundAgent || e.Text == string(agent.TaskAgent)
		case <-eventDeadline.C:
			t.Fatalf("task mode events did not show temporary plan and restore: plan=%v agent=%v", foundPlan, foundAgent)
		}
	}
}

func TestQueuedScopedTurnDoesNotPersistGoalBeforeAdmission(t *testing.T) {
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	ag := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	s := &server{
		cfg:         cfg,
		ag:          ag,
		busy:        true,
		activeTurns: 1,
		sessionID:   "queued-scope",
		permCh:      make(chan perm.Decision, 1),
	}
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"prompt":"fix all","scope_choice":"focused"}`))
	s.chat(res, req)
	if res.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusAccepted)
	}
	if got := ag.GoalSnapshot(); got != "" {
		t.Fatalf("queued scope inferred goal %q before admission", got)
	}
	if got, _ := goal.Load(workspace); got != "" {
		t.Fatalf("queued scope persisted goal %q before admission", got)
	}
}

func TestChatRejectsUndoWhileAgentTurnIsActive(t *testing.T) {
	tests := []struct {
		name        string
		busy        bool
		activeTurns int
	}{
		{name: "busy", busy: true, activeTurns: 1},
		{name: "cancelled but still exiting", activeTurns: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := make(chan event, 1)
			s := &server{
				busy:        tt.busy,
				activeTurns: tt.activeTurns,
				subs:        []chan event{events},
			}
			req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"prompt":"/undo"}`))
			res := httptest.NewRecorder()

			s.chat(res, req)

			if res.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d", res.Code, http.StatusNoContent)
			}
			select {
			case got := <-events:
				if got.Type != "error" {
					t.Fatalf("event type = %q, want error", got.Type)
				}
				if !strings.Contains(got.Text, "cannot undo while a turn is running") {
					t.Fatalf("event text = %q, want active-turn explanation", got.Text)
				}
			default:
				t.Fatal("expected active-turn error event")
			}
		})
	}
}

func TestGUIHandlerEmitsCanonicalFinalTextReplacement(t *testing.T) {
	events := make(chan event, 1)
	s := &server{subs: []chan event{events}}
	h := &guiHandler{s: s}
	h.OnTextFinal("Undo: /undo")

	got := <-events
	if got.Type != "assistant_final" || got.Text != "Undo: /undo" {
		t.Fatalf("event = %+v", got)
	}
}

func TestReadFileRejectsOutsideSymlink(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "escape")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("symlink creation requires privileges on Windows")
		}
		t.Fatal(err)
	}
	s := &server{cfg: config.Config{Workspace: workspace}}
	res := httptest.NewRecorder()
	s.readFile(res, httptest.NewRequest(http.MethodGet, "/api/file?path=escape%2Fsecret.txt", nil))
	if res.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusForbidden)
	}
}

func TestSnapshotIncludesDurableTaskForCurrentSession(t *testing.T) {
	store := taskstate.NewStore(t.TempDir())
	task, err := taskstate.New("session-current", "finish the loop", []string{"implement", "verify"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}
	ag := &agent.Agent{TaskStore: store}
	ag.SetTaskSession("session-current")
	s := &server{ag: ag, sessionID: "session-current"}

	got, ok := s.snapshot()["task"].(*taskstate.Task)
	if !ok || got == nil || got.ID != task.ID {
		t.Fatalf("snapshot task = %#v", s.snapshot()["task"])
	}
}

func TestGUIHandlerDropsStaleTaskProgress(t *testing.T) {
	events := make(chan event, 1)
	s := &server{subs: []chan event{events}, sessionID: "session-live", turnGen: 2}
	task, err := taskstate.New("session-live", "finish the loop", []string{"implement"})
	if err != nil {
		t.Fatal(err)
	}
	stale := &guiHandler{s: s, sessionID: "session-live", turnGen: 1}
	stale.OnTaskState(task)
	select {
	case got := <-events:
		t.Fatalf("stale handler emitted %#v", got)
	default:
	}

	live := &guiHandler{s: s, sessionID: "session-live", turnGen: 2}
	live.OnTaskState(task)
	got := <-events
	if got.Type != "task_progress" || got.SessionID != "session-live" || got.Task == nil || got.Task.ID != task.ID {
		t.Fatalf("event = %#v", got)
	}
}

func TestGUIHandlerDropsStaleTurnCallbacks(t *testing.T) {
	events := make(chan event, 8)
	s := &server{subs: []chan event{events}, sessionID: "session-live", turnGen: 2}
	stale := &guiHandler{s: s, sessionID: "session-live", turnGen: 1, permCh: make(chan perm.Decision, 1)}
	stale.OnText("old assistant text")
	stale.OnTextDelta("old delta")
	stale.OnTextFinal("old final")
	stale.OnToolStart(llm.ToolCall{Name: "read_file", Arguments: `{"path":"old.go"}`})
	stale.OnError(errors.New("old error"))
	if decision, err := stale.OnNeedPermission(context.Background(), perm.Request{Tool: "write_file"}); decision != perm.Deny || !errors.Is(err, context.Canceled) {
		t.Fatalf("stale permission = %s, %v; want canceled denial", decision, err)
	}
	select {
	case got := <-events:
		t.Fatalf("stale callback emitted %#v", got)
	default:
	}

	live := &guiHandler{s: s, sessionID: "session-live", turnGen: 2, permCh: make(chan perm.Decision, 1)}
	live.OnText("current assistant text")
	select {
	case got := <-events:
		if got.Type != "assistant" || got.Text != "current assistant text" || got.SessionID != "session-live" || got.turnGen != 2 {
			t.Fatalf("live callback event = %#v", got)
		}
	default:
		t.Fatal("live callback did not emit")
	}
}

func TestClearRotatesDurableTaskSession(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	store := taskstate.NewStore(t.TempDir())
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	reg := tools.NewRegistry(tools.Context{Workspace: workspace})
	ag := agent.New(cfg, &llm.Scripted{}, reg, perm.New(config.ModeFast, workspace, nil))
	ag.TaskStore = store
	ag.SetTaskSession("session-before-clear")
	task, err := taskstate.New("session-before-clear", "finish the loop", []string{"implement"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(task); err != nil {
		t.Fatal(err)
	}
	s := &server{
		cfg:       cfg,
		ag:        ag,
		sessionID: "session-before-clear",
		hist:      []llm.Message{{Role: "user", Content: "old"}},
		liveTask:  agent.TaskAgent,
		permCh:    make(chan perm.Decision, 1),
	}

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(`{"prompt":"/clear"}`))
	s.chat(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusNoContent)
	}
	s.mu.Lock()
	newID := s.sessionID
	newAgent := s.ag
	histLen := len(s.hist)
	s.mu.Unlock()
	if newID == "session-before-clear" {
		t.Fatal("clear reused the durable task session id")
	}
	if newAgent == nil || newAgent == ag {
		t.Fatal("clear did not replace the agent instance")
	}
	if newAgent.TaskSession != newID {
		t.Fatalf("agent task session = %q, want %q", newAgent.TaskSession, newID)
	}
	if histLen != 0 {
		t.Fatalf("history length after clear = %d, want 0", histLen)
	}
	if got := newAgent.TaskSnapshot(); got != nil {
		t.Fatalf("new session inherited task state: %#v", got)
	}
	if got, err := store.Load("session-before-clear"); err != nil || got == nil {
		t.Fatalf("old session task was lost: task=%#v err=%v", got, err)
	}
}

func TestStaleTurnUsesCapturedAgentSessionAfterReset(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	store := taskstate.NewStore(t.TempDir())
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = workspace
	reg := tools.NewRegistry(tools.Context{Workspace: workspace})
	ag := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "done"}}}}, reg, perm.New(config.ModeFast, workspace, nil))
	ag.TaskStore = store
	ag.SetTaskSession("session-before-reset")
	beforeRun := make(chan struct{})
	release := make(chan struct{})
	s := &server{
		cfg:       cfg,
		ag:        ag,
		sessionID: "session-before-reset",
		liveTask:  agent.TaskAgent,
		permCh:    make(chan perm.Decision, 1),
		beforeAgentRun: func() {
			close(beforeRun)
			<-release
		},
	}

	s.startAgentTurn("fix the broken signup flow", nil)
	// The full cross-platform package run can spend several seconds starting a
	// provider-backed turn on Windows before it reaches this test barrier.
	select {
	case <-beforeRun:
	case <-time.After(10 * time.Second):
		t.Fatal("turn did not reach the pre-run barrier")
	}
	res := httptest.NewRecorder()
	s.reset(res, httptest.NewRequest(http.MethodPost, "/api/reset", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("reset status = %d, want %d", res.Code, http.StatusOK)
	}
	var resetBody map[string]string
	if err := json.Unmarshal(res.Body.Bytes(), &resetBody); err != nil {
		t.Fatal(err)
	}
	newID := resetBody["id"]
	if newID == "" || newID == "session-before-reset" {
		t.Fatalf("reset id = %q", newID)
	}
	close(release)

	deadline := time.Now().Add(10 * time.Second)
	for {
		s.mu.Lock()
		active := s.activeTurns
		s.mu.Unlock()
		if active == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("stale turn did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, err := store.Load(newID); !errors.Is(err, taskstate.ErrNotFound) {
		t.Fatalf("new session received stale task state: err=%v", err)
	}
	if got, err := store.Load("session-before-reset"); err != nil || got == nil {
		t.Fatalf("captured session task was not persisted: task=%#v err=%v", got, err)
	}
	s.mu.Lock()
	currentAgent := s.ag
	s.mu.Unlock()
	if currentAgent == ag {
		t.Fatal("reset did not replace the agent used by the stale turn")
	}
	if currentAgent.TaskSession != newID {
		t.Fatalf("current agent task session = %q, want %q", currentAgent.TaskSession, newID)
	}
}

func TestSettingsWorkspaceChangeRebuildsProjectRuntime(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	oldWorkspace := t.TempDir()
	newWorkspace := t.TempDir()
	store := taskstate.NewStore(t.TempDir())
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Workspace = oldWorkspace
	ag := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "old turn"}}}}, tools.NewRegistry(tools.Context{Workspace: oldWorkspace}), perm.New(config.ModeFast, oldWorkspace, nil))
	ag.SetTaskStore(store)
	ag.SetTaskSession("old-session")
	oldTask, err := taskstate.New("old-session", "finish the old workspace task", []string{"implement"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(oldTask); err != nil {
		t.Fatal(err)
	}
	ag.SetTaskSession("old-session")
	entered := make(chan struct{})
	release := make(chan struct{})
	s := &server{
		cfg:       cfg,
		ag:        ag,
		sessionID: "old-session",
		permCh:    make(chan perm.Decision, 1),
		beforeAgentRun: func() {
			close(entered)
			<-release
		},
	}
	s.startAgentTurn("work in the old workspace", nil)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("old turn did not reach its barrier")
	}

	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(fmt.Sprintf(`{"workspace":%q}`, newWorkspace)))
	s.settings(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("settings status = %d, body=%s", res.Code, res.Body.String())
	}
	s.mu.Lock()
	currentCfg := s.cfg
	currentAgent := s.ag
	currentSession := s.sessionID
	s.mu.Unlock()
	if currentCfg.Workspace != newWorkspace {
		t.Fatalf("workspace = %q, want %q", currentCfg.Workspace, newWorkspace)
	}
	if currentAgent == nil || currentAgent == ag {
		t.Fatal("settings workspace change did not replace the agent")
	}
	if currentSession == "old-session" || currentAgent.TaskSession == "old-session" {
		t.Fatalf("workspace switch retained old session: server=%q agent=%q", currentSession, currentAgent.TaskSession)
	}
	if got := currentAgent.TaskSnapshot(); got != nil {
		t.Fatalf("new workspace inherited old durable task: %#v", got)
	}

	close(release)
	deadline := time.Now().Add(10 * time.Second)
	for {
		s.mu.Lock()
		active := s.activeTurns
		s.mu.Unlock()
		if active == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("stale old-workspace turn did not finish")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got, err := store.Load("old-session"); err != nil || got == nil {
		t.Fatalf("old durable task was lost: task=%#v err=%v", got, err)
	}
}
