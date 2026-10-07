package gui

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/perm"
)

func TestExtensionPreferencesSaveBeforePublication(t *testing.T) {
	for _, action := range []string{"dismiss", "essential"} {
		t.Run(action, func(t *testing.T) {
			s, _ := permissionAckFixture(t, true)
			s.cfg.Extensions.AlwaysAllowTools = []string{"write_file"}
			before := s.cfg
			s.saveConfig = func(config.Config) error { return errors.New("owned save failure") }
			res := httptest.NewRecorder()
			if action == "dismiss" {
				s.extensionsDismiss(res, "owned-extension")
			} else {
				s.extensionsEssential(res, "owned-extension")
			}
			if res.Code != http.StatusInternalServerError || !reflect.DeepEqual(s.cfg, before) {
				t.Fatalf("failed preference save was acknowledged or published: status=%d", res.Code)
			}
		})
	}
}

func TestExtensionPreferencesSerializeWithSettingsAndAlways(t *testing.T) {
	s, decisions := permissionAckFixture(t, true)
	saving := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseSaving := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseSaving()
	s.saveConfig = func(cfg config.Config) error {
		if len(cfg.Extensions.AlwaysAllowTools) == 0 && len(cfg.Extensions.Dismissed) == 0 {
			close(saving)
			<-release
		}
		return config.Save(cfg)
	}
	settingsDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		res := httptest.NewRecorder()
		s.settings(res, loopbackAPIRequest(http.MethodPost, "/api/settings", `{"max_tool_rounds":99}`))
		settingsDone <- res
	}()
	select {
	case <-saving:
	case <-time.After(5 * time.Second):
		t.Fatal("settings did not reach prospective persistence")
	}
	extensionDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		res := httptest.NewRecorder()
		s.extensionsDismiss(res, "owned-extension")
		extensionDone <- res
	}()
	permissionDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		permissionDone <- postPermissionAck(s, context.Background(), `{"always":true,"permission_id":"7"}`)
	}()
	select {
	case decision := <-decisions:
		if decision != perm.AllowAlways {
			t.Fatal("Always choice not delivered")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("permission did not reach delivery")
	}
	releaseSaving()
	for _, done := range []<-chan *httptest.ResponseRecorder{settingsDone, extensionDone, permissionDone} {
		select {
		case res := <-done:
			if res.Code < 200 || res.Code >= 300 {
				t.Fatalf("config transaction failed: %d %s", res.Code, res.Body.String())
			}
		case <-time.After(5 * time.Second):
			t.Fatal("config transaction did not finish")
		}
	}
	loaded, err := config.Load()
	if err != nil || loaded.MaxToolRounds != 99 || !containsString(loaded.Extensions.AlwaysAllowTools, "write_file") || !containsString(loaded.Extensions.Dismissed, "owned-extension") {
		t.Fatalf("restart config lost an acknowledged change: %+v %v", loaded.Extensions, err)
	}
}

func TestExtensionProspectiveRemovalDoesNotAliasLiveConfig(t *testing.T) {
	s, _ := permissionAckFixture(t, true)
	s.cfg.Extensions.InstalledPlugins = []string{"remove", "keep"}
	s.saveConfig = func(config.Config) error { return errors.New("owned save failure") }
	s.configTxMu.Lock()
	err := s.persistExtensionUpdate(func(ext *config.ExtensionsConfig) {
		ext.InstalledPlugins = removeString(ext.InstalledPlugins, "remove")
	})
	s.configTxMu.Unlock()
	if err == nil || !reflect.DeepEqual(s.cfg.Extensions.InstalledPlugins, []string{"remove", "keep"}) {
		t.Fatal("failed prospective removal mutated live slice storage")
	}
}

func TestExtensionPreferencesPreserveCurrentConfig(t *testing.T) {
	s, _ := permissionAckFixture(t, true)
	s.cfg.Extensions.AlwaysAllowTools = []string{"write_file"}
	s.cfg.MaxToolRounds = 99
	var saved config.Config
	s.saveConfig = func(cfg config.Config) error { saved = cfg; return nil }
	res := httptest.NewRecorder()
	s.extensionsDismiss(res, "owned-extension")
	if res.Code != http.StatusOK || saved.MaxToolRounds != 99 || !containsString(saved.Extensions.AlwaysAllowTools, "write_file") || !containsString(saved.Extensions.Dismissed, "owned-extension") {
		t.Fatal("extension preference bypassed the shared save transaction or lost current settings")
	}
}
