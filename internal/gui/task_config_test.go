package gui

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/tools"
)

func TestTaskModeSaveFailureDoesNotPublish(t *testing.T) {
	for _, surface := range []string{"api", "slash"} {
		t.Run(surface, func(t *testing.T) {
			s, _ := permissionAckFixture(t, true)
			s.liveTask = agent.TaskAgent
			s.ag.SetTaskMode(agent.TaskAgent)
			before := s.cfg
			s.saveConfig = func(config.Config) error { return errors.New("owned save failure") }
			res := httptest.NewRecorder()
			if surface == "api" {
				s.setTaskMode(res, loopbackAPIRequest(http.MethodPost, "/api/task-mode", `{"task_mode":"plan"}`))
			} else {
				s.chat(res, loopbackAPIRequest(http.MethodPost, "/api/chat", `{"prompt":"/plan"}`))
			}
			if res.Code != http.StatusInternalServerError || !reflect.DeepEqual(before, s.cfg) || s.liveTask != agent.TaskAgent || s.ag.TaskModeSnapshot() != agent.TaskAgent {
				t.Fatalf("%s acknowledged/published an unsaved task mode: status=%d mode=%s", surface, res.Code, s.liveTask)
			}
		})
	}
}

func TestTaskModeSerializesWithSettingsAndAlways(t *testing.T) {
	s, decisions := permissionAckFixture(t, true)
	saving, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	releaseSave := func() { once.Do(func() { close(release) }) }
	defer releaseSave()
	s.saveConfig = func(cfg config.Config) error {
		if cfg.TaskMode != "plan" && len(cfg.Extensions.AlwaysAllowTools) == 0 {
			close(saving)
			<-release
		}
		return config.Save(cfg)
	}
	settingsDone := make(chan int, 1)
	go func() {
		res := httptest.NewRecorder()
		s.settings(res, loopbackAPIRequest(http.MethodPost, "/api/settings", `{"max_tool_rounds":99}`))
		settingsDone <- res.Code
	}()
	select {
	case <-saving:
	case <-time.After(5 * time.Second):
		t.Fatal("settings did not reach save")
	}
	modeDone, permissionDone := make(chan int, 1), make(chan int, 1)
	go func() {
		res := httptest.NewRecorder()
		s.setTaskMode(res, loopbackAPIRequest(http.MethodPost, "/api/task-mode", `{"task_mode":"plan"}`))
		modeDone <- res.Code
	}()
	go func() {
		permissionDone <- postPermissionAck(s, context.Background(), `{"always":true,"permission_id":"7"}`).Code
	}()
	select {
	case decision := <-decisions:
		if decision != perm.AllowAlways {
			t.Fatal("Always not delivered")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("permission delivery blocked on task config transaction")
	}
	releaseSave()
	for _, done := range []<-chan int{settingsDone, modeDone, permissionDone} {
		select {
		case code := <-done:
			if code < 200 || code >= 300 {
				t.Fatalf("config response=%d", code)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("task config transaction did not finish")
		}
	}
	loaded, err := config.Load()
	if err != nil || loaded.MaxToolRounds != 99 || loaded.TaskMode != "plan" || !containsString(loaded.Extensions.AlwaysAllowTools, "write_file") {
		t.Fatalf("restart lost acknowledged config: mode=%s rounds=%d ext=%+v err=%v", loaded.TaskMode, loaded.MaxToolRounds, loaded.Extensions, err)
	}
}

func TestTurnCompletionCannotOverwriteSettingsPersistence(t *testing.T) {
	s := completionConfigFixture(t)
	saving, release, completionSave := make(chan struct{}), make(chan struct{}), make(chan struct{}, 1)
	var once sync.Once
	releaseSave := func() { once.Do(func() { close(release) }) }
	defer releaseSave()
	s.saveConfig = func(cfg config.Config) error {
		if err := config.Save(cfg); err != nil {
			return err
		}
		if cfg.MaxToolRounds == 99 {
			select {
			case completionSave <- struct{}{}:
			default:
			}
			// Only the first (Settings) save is held. The later completed-turn
			// save must use this freshly published configuration as well.
			select {
			case <-saving:
			default:
				close(saving)
				<-release
			}
		}
		return nil
	}
	settingsDone := make(chan int, 1)
	go func() {
		res := httptest.NewRecorder()
		s.settings(res, loopbackAPIRequest(http.MethodPost, "/api/settings", `{"max_tool_rounds":99}`))
		settingsDone <- res.Code
	}()
	select {
	case <-saving:
	case <-time.After(5 * time.Second):
		t.Fatal("settings did not persist candidate")
	}
	<-completionSave // consume first Settings save observation
	res := httptest.NewRecorder()
	s.chat(res, loopbackAPIRequest(http.MethodPost, "/api/chat", `{"prompt":"say hello"}`))
	if res.Code != http.StatusAccepted {
		t.Fatalf("chat status=%d", res.Code)
	}
	turnDone := make(chan struct{})
	go func() { s.waitForTurns(); close(turnDone) }()
	// The uncorrected completion save can overwrite disk while Settings is
	// held before publication. A transaction-correct completion must wait.
	select {
	case <-turnDone:
		releaseSave()
		t.Fatal("turn completed config persistence inside an open Settings transaction")
	case <-time.After(100 * time.Millisecond):
	}
	releaseSave()
	select {
	case code := <-settingsDone:
		if code != http.StatusOK {
			t.Fatalf("settings status=%d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("settings did not finish")
	}
	select {
	case <-turnDone:
	case <-time.After(5 * time.Second):
		t.Fatal("turn did not finish after Settings commit")
	}
	select {
	case <-completionSave:
	default:
		t.Fatal("end-turn save bypassed shared seam or used stale config")
	}
	loaded, err := config.Load()
	if err != nil || loaded.MaxToolRounds != 99 {
		t.Fatalf("completion overwrote acknowledged Settings: rounds=%d err=%v", loaded.MaxToolRounds, err)
	}
}

func completionConfigFixture(t *testing.T) *server {
	t.Helper()
	s, _ := permissionAckFixture(t, true)
	s.cfg.Provider = config.ProviderOllama
	s.cfg.AutoTaskMode = new(bool)
	s.ag = agent.New(s.cfg, &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{Role: "assistant", Content: "hello"}}}}, tools.NewRegistry(tools.Context{Workspace: s.cfg.Workspace}), perm.New(config.ModeSafe, s.cfg.Workspace, nil))
	t.Cleanup(s.ag.Close)
	s.sessionID = "owned-completion"
	s.suppressExtensionRebuild = true
	return s
}

func TestTurnCompletionConfigSaveFailureIsVisible(t *testing.T) {
	s := completionConfigFixture(t)
	s.saveConfig = func(config.Config) error { return errors.New("owned save failure") }
	events := make(chan event, 64)
	s.subs = []chan event{events}
	res := httptest.NewRecorder()
	s.chat(res, loopbackAPIRequest(http.MethodPost, "/api/chat", `{"prompt":"say hello"}`))
	if res.Code != http.StatusAccepted {
		t.Fatalf("chat status=%d", res.Code)
	}
	turnDone := make(chan struct{})
	go func() { s.waitForTurns(); close(turnDone) }()
	select {
	case <-turnDone:
	case <-time.After(5 * time.Second):
		t.Fatal("failed save stranded completed turn")
	}
	for len(events) > 0 {
		e := <-events
		if e.Type == "error" && strings.Contains(e.Text, "couldn't save turn settings") {
			return
		}
	}
	t.Fatal("end-turn preference save failure was hidden")
}
