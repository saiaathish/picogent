package gui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/projects"
	"github.com/saiaathish/picogent/internal/tools"
)

func preparedConfigFixture(t *testing.T) *server {
	t.Helper()
	s, _ := permissionAckFixture(t, true)
	s.cfg.Provider = config.ProviderOllama
	s.sessionID = "owned-old-session"
	s.ag = isolatedConfigAgent(t, s.cfg)
	s.buildConfig = func(cfg config.Config) (*agent.Agent, error) {
		return isolatedConfigAgent(t, cfg), nil
	}
	if err := config.Save(s.cfg); err != nil {
		t.Fatal(err)
	}
	return s
}

func isolatedConfigAgent(t *testing.T, cfg config.Config) *agent.Agent {
	t.Helper()
	a := agent.New(cfg, &llm.Scripted{}, tools.NewRegistry(tools.Context{Workspace: cfg.Workspace}), perm.New(cfg.Mode, cfg.Workspace, nil))
	t.Cleanup(a.Close)
	return a
}

func TestSetupFinishBuildFailureDoesNotPersist(t *testing.T) {
	s := preparedConfigFixture(t)
	before := s.cfg
	s.buildConfig = func(config.Config) (*agent.Agent, error) { return nil, errors.New("owned build failure") }
	body, _ := json.Marshal(map[string]string{"workspace": t.TempDir(), "mode": "fast"})
	res := httptest.NewRecorder()
	s.setupFinish(res, loopbackAPIRequest(http.MethodPost, "/api/setup/finish", string(body)))
	loaded, err := config.Load()
	if res.Code != 500 || err != nil || loaded.Workspace != before.Workspace || loaded.PersistentMode() != before.PersistentMode() || loaded.SetupComplete {
		t.Fatalf("failed setup changed disk: status=%d workspace=%q mode=%s setup=%t err=%v", res.Code, loaded.Workspace, loaded.PersistentMode(), loaded.SetupComplete, err)
	}
}

func TestPreparedConfigSaveFailureDoesNotPublish(t *testing.T) {
	for _, surface := range []string{"setup", "project"} {
		t.Run(surface, func(t *testing.T) {
			s := preparedConfigFixture(t)
			before, oldAgent, oldSession, oldGen := s.cfg, s.ag, s.sessionID, s.turnGen
			calls := 0
			s.saveConfig = func(config.Config) error {
				calls++
				if s.ag != oldAgent || s.cfg.Workspace != before.Workspace || s.sessionID != oldSession || s.turnGen != oldGen {
					t.Error("runtime published or turn invalidated before save")
				}
				return errors.New("owned save failure")
			}
			res := httptest.NewRecorder()
			if surface == "setup" {
				body, _ := json.Marshal(map[string]string{"workspace": t.TempDir(), "mode": "fast"})
				s.setupFinish(res, loopbackAPIRequest(http.MethodPost, "/api/setup/finish", string(body)))
			} else {
				body, _ := json.Marshal(map[string]string{"action": "add", "path": t.TempDir()})
				s.projectsAPI(res, loopbackAPIRequest(http.MethodPost, "/api/projects", string(body)))
				reg, err := projects.Load()
				if err != nil || len(reg.Projects) != 0 {
					t.Fatalf("failed project persisted selection: %+v err=%v", reg, err)
				}
			}
			if res.Code != 500 || calls != 1 || !reflect.DeepEqual(s.cfg, before) || s.ag != oldAgent || s.sessionID != oldSession || s.turnGen != oldGen {
				t.Fatalf("%s published unsaved state: status=%d saves=%d generation=%d", surface, res.Code, calls, s.turnGen)
			}
		})
	}
}

func setOwnedRouterObservation(cfg *config.Config) {
	cfg.Router.LastTier = "deep"
	cfg.Router.LastModel = "owned-model"
	cfg.Router.LastReason = "owned-reason"
	cfg.Router.LastReasoning = "high"
	cfg.Router.LastTaskKind = "debug"
	cfg.Router.LastRouteMode = "test"
}

func TestPreparedConfigRetainsRouterObservations(t *testing.T) {
	for _, surface := range []string{"settings", "setup", "project"} {
		t.Run(surface, func(t *testing.T) {
			s := preparedConfigFixture(t)
			s.buildConfig = func(cfg config.Config) (*agent.Agent, error) {
				s.mu.Lock()
				setOwnedRouterObservation(&s.cfg)
				s.mu.Unlock()
				return isolatedConfigAgent(t, cfg), nil
			}
			res := httptest.NewRecorder()
			switch surface {
			case "settings":
				s.settings(res, loopbackAPIRequest(http.MethodPost, "/api/settings", `{"provider":"ollama"}`))
			case "setup":
				body, _ := json.Marshal(map[string]string{"workspace": s.cfg.Workspace, "mode": "safe"})
				s.setupFinish(res, loopbackAPIRequest(http.MethodPost, "/api/setup/finish", string(body)))
			case "project":
				body, _ := json.Marshal(map[string]string{"action": "add", "path": t.TempDir()})
				s.projectsAPI(res, loopbackAPIRequest(http.MethodPost, "/api/projects", string(body)))
			}
			loaded, err := config.Load()
			want := config.Default()
			setOwnedRouterObservation(&want)
			for _, got := range []config.Config{s.cfg, s.ag.ConfigSnapshot(), loaded} {
				if res.Code < 200 || res.Code >= 300 || err != nil || got.Router.LastTier != want.Router.LastTier || got.Router.LastModel != want.Router.LastModel || got.Router.LastReason != want.Router.LastReason || got.Router.LastReasoning != want.Router.LastReasoning || got.Router.LastTaskKind != want.Router.LastTaskKind || got.Router.LastRouteMode != want.Router.LastRouteMode {
					t.Fatalf("%s lost current router observations: status=%d router=%+v err=%v", surface, res.Code, got.Router, err)
				}
			}
		})
	}
}

func TestSetupConfigInitializationUsesCurrentConfig(t *testing.T) {
	s, _ := permissionAckFixture(t, true)
	s.cfg.TaskMode = "plan"
	s.cfg.MaxToolRounds = 99
	s.cfg.Extensions.AlwaysAllowTools = []string{"write_file"}
	calls := 0
	s.saveConfig = func(cfg config.Config) error { calls++; return config.Save(cfg) }
	if err := s.ensureSetupConfig(); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load()
	if err != nil || loaded.TaskMode != "plan" || loaded.MaxToolRounds != 99 || !containsString(loaded.Extensions.AlwaysAllowTools, "write_file") || calls != 1 {
		t.Fatalf("installer replaced current state with defaults: %+v saves=%d err=%v", loaded, calls, err)
	}
	if err := s.ensureSetupConfig(); err != nil || calls != 1 {
		t.Fatalf("installer overwrote existing config: saves=%d err=%v", calls, err)
	}
}

func TestSetupFinishSerializesPreferencesWithoutBlockingPermissionDelivery(t *testing.T) {
	s := preparedConfigFixture(t)
	s.cfg.Extensions.AlwaysAllowTools = []string{"read_file"}
	if err := config.Save(s.cfg); err != nil {
		t.Fatal(err)
	}
	events := make(chan event, 64)
	s.subs = []chan event{events}
	decisions := s.pendingPermCh
	preparing, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	s.buildConfig = func(cfg config.Config) (*agent.Agent, error) {
		close(preparing)
		<-release
		return isolatedConfigAgent(t, cfg), nil
	}
	body, _ := json.Marshal(map[string]string{"workspace": s.cfg.Workspace, "mode": "fast"})
	setupDone := make(chan int, 1)
	go func() {
		res := httptest.NewRecorder()
		s.setupFinish(res, loopbackAPIRequest(http.MethodPost, "/api/setup/finish", string(body)))
		setupDone <- res.Code
	}()
	select {
	case <-preparing:
	case <-time.After(5 * time.Second):
		t.Fatal("setup did not reach preparation")
	}
	permissionDone, settingsDone := make(chan int, 1), make(chan int, 1)
	go func() {
		permissionDone <- postPermissionAck(s, context.Background(), `{"always":true,"permission_id":"7"}`).Code
	}()
	go func() {
		res := httptest.NewRecorder()
		s.settings(res, loopbackAPIRequest(http.MethodPost, "/api/settings", `{"max_tool_rounds":99}`))
		settingsDone <- res.Code
	}()
	select {
	case decision := <-decisions:
		if decision != perm.AllowAlways {
			t.Fatal("permission choice changed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("setup preparation blocked permission delivery on server lock")
	}
	// Releasing through a separate channel makes deferred cleanup safe on any
	// failure without closing the build barrier twice.
	select {
	case release <- struct{}{}:
	case <-time.After(5 * time.Second):
		t.Fatal("setup preparation did not release")
	}
	for _, done := range []<-chan int{setupDone, permissionDone, settingsDone} {
		select {
		case status := <-done:
			if status < 200 || status >= 300 {
				t.Fatalf("config request status=%d", status)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("serialized setup config request did not finish")
		}
	}
	loaded, err := config.Load()
	if err != nil || !loaded.SetupComplete || loaded.PersistentMode() != config.ModeFast || loaded.MaxToolRounds != 99 || !containsString(loaded.Extensions.AlwaysAllowTools, "read_file") || containsString(loaded.Extensions.AlwaysAllowTools, "write_file") {
		t.Fatalf("restart lost acknowledged preferences or granted an old generation policy: %+v err=%v", loaded, err)
	}
	// Setup replaces the turn generation. The delivered old-turn Always choice
	// still applies once, but cannot authorize a new generation silently.
	for len(events) > 0 {
		if e := <-events; e.Type == "error" && e.Text == "Your choice applied, but saving it for future sessions could not be confirmed." {
			return
		}
	}
	t.Fatal("old-generation Always delivery did not report unconfirmed future policy")
}
