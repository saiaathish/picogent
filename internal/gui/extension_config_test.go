package gui

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/extensions"
	"github.com/saiaathish/picogent/internal/perm"
)

// External action is synthetic and confined to t.TempDir: real extension Undo
// may touch user-global skill/MCP paths, which this regression must not modify.
func TestExtensionUndoSaveRetryDoesNotRepeatExternalAction(t *testing.T) {
	s, _ := permissionAckFixture(t, true)
	s.cfg.Provider = "test-invalid"
	s.cfg.Extensions.InstalledPlugins = []string{"owned-plugin", "keep"}
	s.undoStack = []extensionUndoRecord{
		{UndoEntry: extensions.UndoEntry{ID: "keep-undo", ExtID: "keep", Kind: extensions.KindPlugin}},
		{UndoEntry: extensions.UndoEntry{ID: "owned-undo", ExtID: "owned-plugin", Kind: extensions.KindPlugin}},
	}
	path := filepath.Join(t.TempDir(), "external-state")
	if err := os.WriteFile(path, []byte("installed"), 0o600); err != nil {
		t.Fatal(err)
	}
	undoCalls := 0
	s.undoExtension = func(entry extensions.UndoEntry) error {
		undoCalls++
		if entry.ID != "owned-undo" {
			t.Fatal("wrong external undo identity")
		}
		return os.WriteFile(path, []byte("restored"), 0o600)
	}
	s.saveConfig = func(config.Config) error { return errors.New("owned save failure") }
	first := httptest.NewRecorder()
	s.extensionsUndo(first, "owned-undo")
	if first.Code != http.StatusInternalServerError || undoCalls != 1 || !reflect.DeepEqual(s.cfg.Extensions.InstalledPlugins, []string{"owned-plugin", "keep"}) {
		t.Fatalf("failed save published preferences or lost external-action result: status=%d calls=%d", first.Code, undoCalls)
	}
	if err := os.WriteFile(path, []byte("later user edit"), 0o600); err != nil {
		t.Fatal(err)
	}
	failedRetry := httptest.NewRecorder()
	s.extensionsUndo(failedRetry, "owned-undo")
	if failedRetry.Code != http.StatusInternalServerError || undoCalls != 1 || len(s.undoStack) != 2 || !s.undoStack[1].externalApplied {
		t.Fatal("repeated save failure lost the applied marker or repeated external undo")
	}
	s.saveConfig = config.Save
	second := httptest.NewRecorder()
	s.extensionsUndo(second, "owned-undo")
	if second.Code != http.StatusOK || undoCalls != 1 || len(s.undoStack) != 1 || s.undoStack[0].ID != "keep-undo" {
		t.Fatalf("same-ID save retry repeated external action or was lost: status=%d calls=%d pending=%d", second.Code, undoCalls, len(s.undoStack))
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "later user edit" {
		t.Fatalf("save retry overwrote a later user edit: %q %v", got, err)
	}
	loaded, err := config.Load()
	if err != nil || !reflect.DeepEqual(loaded.Extensions.InstalledPlugins, []string{"keep"}) {
		t.Fatalf("save retry failed durable preference reconciliation: %+v %v", loaded.Extensions, err)
	}
	third := httptest.NewRecorder()
	s.extensionsUndo(third, "owned-undo")
	if third.Code != http.StatusNotFound || undoCalls != 1 {
		t.Fatal("committed undo was not consumed exactly once")
	}
}

func TestExtensionUndoConcurrentSaveRetryAppliesExternalActionOnce(t *testing.T) {
	s, _ := permissionAckFixture(t, true)
	s.cfg.Provider = "test-invalid"
	s.cfg.Extensions.InstalledPlugins = []string{"owned-plugin"}
	s.undoStack = []extensionUndoRecord{{UndoEntry: extensions.UndoEntry{ID: "owned-undo", ExtID: "owned-plugin", Kind: extensions.KindPlugin}}}
	undoCalls, saveCalls := 0, 0
	s.undoExtension = func(extensions.UndoEntry) error { undoCalls++; return nil }
	saving, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releaseSaving := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseSaving()
	s.saveConfig = func(cfg config.Config) error {
		saveCalls++
		if saveCalls == 1 {
			close(saving)
			<-release
			return errors.New("owned save failure")
		}
		return config.Save(cfg)
	}
	firstDone, secondDone := make(chan int, 1), make(chan int, 1)
	run := func(done chan<- int) {
		res := httptest.NewRecorder()
		s.extensionsUndo(res, "owned-undo")
		done <- res.Code
	}
	go run(firstDone)
	select {
	case <-saving:
	case <-time.After(5 * time.Second):
		t.Fatal("first undo did not reach save")
	}
	go run(secondDone)
	releaseSaving()
	for _, expected := range []struct {
		done <-chan int
		code int
	}{{firstDone, http.StatusInternalServerError}, {secondDone, http.StatusOK}} {
		select {
		case code := <-expected.done:
			if code != expected.code {
				t.Fatalf("concurrent undo response=%d, want %d", code, expected.code)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent undo did not finish")
		}
	}
	if undoCalls != 1 || saveCalls != 2 || len(s.undoStack) != 0 {
		t.Fatalf("concurrent save retry duplicated/lost undo: external=%d save=%d pending=%d", undoCalls, saveCalls, len(s.undoStack))
	}
}

func TestExtensionUndoExternalFailureRetainsRetry(t *testing.T) {
	s, _ := permissionAckFixture(t, true)
	s.cfg.Provider = "test-invalid"
	s.cfg.Extensions.InstalledPlugins = []string{"owned-plugin"}
	s.undoStack = []extensionUndoRecord{{UndoEntry: extensions.UndoEntry{ID: "owned-undo", ExtID: "owned-plugin", Kind: extensions.KindPlugin}}}
	undoCalls, saveCalls := 0, 0
	s.undoExtension = func(extensions.UndoEntry) error {
		undoCalls++
		if undoCalls == 1 {
			return errors.New("owned external failure")
		}
		return nil
	}
	s.saveConfig = func(config.Config) error { saveCalls++; return nil }
	first := httptest.NewRecorder()
	s.extensionsUndo(first, "owned-undo")
	if first.Code != http.StatusInternalServerError || saveCalls != 0 || len(s.cfg.Extensions.InstalledPlugins) != 1 {
		t.Fatal("failed external undo changed preferences")
	}
	second := httptest.NewRecorder()
	s.extensionsUndo(second, "owned-undo")
	if second.Code != http.StatusOK || undoCalls != 2 || saveCalls != 1 || len(s.undoStack) != 0 {
		t.Fatalf("external failure was not retryable: status=%d calls=%d saves=%d", second.Code, undoCalls, saveCalls)
	}
}

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
