//go:build rendered_fixture

package gui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/taskstate"
)

const renderedRecoveryFixtureTestPrompt = "Create the rendered recovery probe file"

// TestRenderedRecoveryFixtureAPIBoundary drives allow → undo → fresh reload
// through the same HTTP boundaries the embedded GUI uses. It is not a browser
// DOM claim and does not exercise a live provider.
func TestRenderedRecoveryFixtureAPIBoundary(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	workspace := filepath.Join(home, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PICOGENT_HOME", home)

	seedServer, seedAgent, err := newRenderedRecoveryFixtureServer("seed", home, workspace)
	if err != nil {
		t.Fatal(err)
	}
	seedHTTP := httptest.NewServer(seedServer.Handler())
	defer func() {
		seedServer.stopForShutdown()
		seedServer.waitForTurns()
		seedHTTP.Close()
		seedAgent.Close()
	}()

	driveRenderedRecoveryChat(t, seedHTTP.URL, renderedRecoveryFixtureTestPrompt, 0)
	seedTask := waitForRenderedRecoveryTask(t, seedAgent, 1)
	assertRenderedRecoveryProbe(t, workspace, true)
	assertRenderedRecoveryState(t, seedHTTP.URL, true)

	driveRenderedRecoveryChat(t, seedHTTP.URL, "/undo", seedTask.LastTurn().Sequence)
	assertRenderedRecoveryProbe(t, workspace, false)
	assertRenderedRecoveryState(t, seedHTTP.URL, false)
	if seedAgent.UndoAvailable() {
		t.Fatal("undo remained available after /undo")
	}
	undone := seedAgent.TaskSnapshot()
	if undone == nil {
		t.Fatal("durable task missing after undo")
	}
	if undone.CompletionReady() {
		t.Fatal("task stayed completion-ready after undo invalidated proof")
	}

	seedServer.stopForShutdown()
	seedServer.waitForTurns()
	seedHTTP.Close()
	seedAgent.Close()

	reloadServer, reloadAgent, err := newRenderedRecoveryFixtureServer("reload", home, workspace)
	if err != nil {
		t.Fatal(err)
	}
	reloadHTTP := httptest.NewServer(reloadServer.Handler())
	defer func() {
		reloadServer.stopForShutdown()
		reloadServer.waitForTurns()
		reloadHTTP.Close()
		reloadAgent.Close()
	}()

	assertRenderedRecoveryProbe(t, workspace, false)
	assertRenderedRecoveryState(t, reloadHTTP.URL, false)
	reloaded := reloadAgent.TaskSnapshot()
	if reloaded == nil || reloaded.SessionID != renderedRecoveryFixtureSession {
		t.Fatalf("reloaded task = %#v, want durable fixture session", reloaded)
	}
	if reloaded.CompletionReady() {
		t.Fatal("reloaded task projected stale completion")
	}
	if reloadAgent.UndoAvailable() {
		t.Fatal("reloaded process advertised undo after restored absent probe")
	}
}

func driveRenderedRecoveryChat(t *testing.T, baseURL, prompt string, previousRevision uint64) {
	t.Helper()
	payload, err := json.Marshal(map[string]string{"prompt": prompt})
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, baseURL+"/api/chat", strings.NewReader(string(payload)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://"+request.URL.Host)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	wantStatus := http.StatusAccepted
	if prompt == "/undo" {
		// Local slash commands complete synchronously and return 204.
		wantStatus = http.StatusNoContent
	}
	if response.StatusCode != wantStatus {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		t.Fatalf("chat status = %d, want %d: %s", response.StatusCode, wantStatus, data)
	}
	if prompt == "/undo" {
		return
	}

	seenPermissions := map[string]bool{}
	deadline := time.Now().Add(10 * time.Second)
	started := false
	for time.Now().Before(deadline) {
		state := readRenderedRecoveryState(t, baseURL)
		busy, _ := state["busy"].(bool)
		started = started || busy
		if pending, ok := state["pending_perm"].(map[string]any); ok {
			permissionID, _ := pending["permission_id"].(string)
			if permissionID != "" && !seenPermissions[permissionID] {
				postRenderedRecoveryPermission(t, baseURL, permissionID)
				seenPermissions[permissionID] = true
			}
		}
		currentRevision := renderedRecoveryStateTurnRevision(state)
		if !busy && (started || currentRevision > previousRevision) && !renderedRecoveryStateHasPendingPermission(state) && currentRevision > previousRevision {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("chat prompt %q did not finish after turn %d", prompt, previousRevision)
}

func postRenderedRecoveryPermission(t *testing.T, baseURL, permissionID string) {
	t.Helper()
	payload := fmt.Sprintf(`{"allow":true,"permission_id":%q}`, permissionID)
	request, err := http.NewRequest(http.MethodPost, baseURL+"/api/permission", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", "http://"+request.URL.Host)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("permission status = %d, want %d", response.StatusCode, http.StatusNoContent)
	}
}

func readRenderedRecoveryState(t *testing.T, baseURL string) map[string]any {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, baseURL+"/api/state", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("state status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	var state map[string]any
	if err := json.NewDecoder(response.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

func assertRenderedRecoveryState(t *testing.T, baseURL string, wantUndo bool) {
	t.Helper()
	state := readRenderedRecoveryState(t, baseURL)
	got, ok := state["undo_available"].(bool)
	if !ok || got != wantUndo {
		t.Fatalf("undo_available = %#v, want %v", state["undo_available"], wantUndo)
	}
	task, ok := state["task"].(map[string]any)
	if !ok {
		t.Fatalf("rendered task = %#v", state["task"])
	}
	if got, _ := task["session_id"].(string); got != renderedRecoveryFixtureSession {
		t.Fatalf("rendered session = %q, want %q", got, renderedRecoveryFixtureSession)
	}
}

func assertRenderedRecoveryProbe(t *testing.T, workspace string, wantPresent bool) {
	t.Helper()
	path := filepath.Join(workspace, renderedRecoveryFixturePath)
	_, err := os.Stat(path)
	if wantPresent {
		if err != nil {
			t.Fatalf("probe missing: %v", err)
		}
		got, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if string(got) != renderedRecoveryFixtureContent {
			t.Fatalf("probe = %q, want %q", got, renderedRecoveryFixtureContent)
		}
		return
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("probe presence = %v, want absent", err)
	}
}

func waitForRenderedRecoveryTask(t *testing.T, ag *agent.Agent, previousRevision uint64) *taskstate.Task {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		task := ag.TaskSnapshot()
		if task != nil {
			last := task.LastTurn()
			if last != nil && last.Sequence >= previousRevision && last.State != taskstate.TurnActive {
				return task
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("agent did not expose a closed turn at revision %d", previousRevision)
	return nil
}

func renderedRecoveryStateTurnRevision(state map[string]any) uint64 {
	task, ok := state["task"].(map[string]any)
	if !ok {
		return 0
	}
	turns, ok := task["turns"].([]any)
	if !ok || len(turns) == 0 {
		return 0
	}
	last, ok := turns[len(turns)-1].(map[string]any)
	if !ok {
		return 0
	}
	switch seq := last["sequence"].(type) {
	case float64:
		return uint64(seq)
	case json.Number:
		v, _ := seq.Int64()
		return uint64(v)
	default:
		return 0
	}
}

func renderedRecoveryStateHasPendingPermission(state map[string]any) bool {
	_, ok := state["pending_perm"].(map[string]any)
	return ok
}
