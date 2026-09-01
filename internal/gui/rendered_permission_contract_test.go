package gui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
)

func TestRenderedPermissionContractKeepsDecisionBeforeSideEffect(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = workspace
	s := &server{
		cfg:       cfg,
		sessionID: "rendered-permission-contract",
		turnGen:   1,
		permCh:    make(chan perm.Decision, 1),
	}
	h := newGUIHandlerAtWithPerm(s, s.sessionID, s.turnGen, s.permCh)

	for _, tc := range []struct {
		name      string
		allow     bool
		want      perm.Decision
		wantBytes []byte
	}{
		{name: "deny leaves file absent", allow: false, want: perm.Deny},
		{name: "allow writes only after approval", allow: true, want: perm.Allow, wantBytes: []byte("rendered permission probe\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(workspace, "rendered-probe.txt")
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			resultCh := make(chan struct {
				decision perm.Decision
				err      error
			}, 1)
			go func() {
				decision, err := h.OnNeedPermission(ctx, perm.Request{
					Tool:    "write_file",
					Summary: "write rendered-probe.txt",
					Hint:    "Safe mode requires approval before a file change.",
				})
				if err == nil && decision == perm.Allow {
					err = os.WriteFile(path, tc.wantBytes, 0o600)
				}
				resultCh <- struct {
					decision perm.Decision
					err      error
				}{decision: decision, err: err}
			}()

			deadline := time.NewTimer(time.Second)
			defer deadline.Stop()
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()
			for {
				s.mu.Lock()
				pending := s.pendingPerm.Tool
				s.mu.Unlock()
				if pending == "write_file" {
					break
				}
				select {
				case <-deadline.C:
					t.Fatal("permission request was not exposed to the GUI boundary")
				case <-ticker.C:
				}
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("side effect occurred before permission decision: err=%v", err)
			}

			body := `{"allow":false}`
			if tc.allow {
				body = `{"allow":true}`
			}
			req := loopbackAPIRequest(http.MethodPost, "/api/permission", body)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", "http://"+loopbackTestHost)
			res := httptest.NewRecorder()
			s.Handler().ServeHTTP(res, req)
			if res.Code != http.StatusNoContent {
				t.Fatalf("permission response status = %d, want %d", res.Code, http.StatusNoContent)
			}

			var result struct {
				decision perm.Decision
				err      error
			}
			select {
			case result = <-resultCh:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if result.err != nil {
				t.Fatal(result.err)
			}
			if result.decision != tc.want {
				t.Fatalf("permission decision = %s, want %s", result.decision, tc.want)
			}

			got, err := os.ReadFile(path)
			switch {
			case tc.allow && err != nil:
				t.Fatalf("allowed mutation was not published: %v", err)
			case tc.allow && string(got) != string(tc.wantBytes):
				t.Fatalf("allowed mutation = %q, want %q", got, tc.wantBytes)
			case !tc.allow && !errors.Is(err, os.ErrNotExist):
				t.Fatalf("denied mutation changed the workspace: bytes=%q err=%v", got, err)
			}

			s.mu.Lock()
			remaining := s.pendingPerm.Tool
			s.mu.Unlock()
			if remaining != "" {
				t.Fatalf("permission request remained visible after decision: %q", remaining)
			}
		})
	}
}

func TestRenderedPermissionPersistsDecisionAndMutation(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	for _, tc := range []struct {
		name          string
		allow         bool
		wantDecision  perm.Decision
		wantStatus    taskstate.Status
		wantFile      bool
		wantEvidence  string
		wantUndo      bool
		wantChangeSeq int
	}{
		{name: "deny blocks without mutation", allow: false, wantDecision: perm.Deny, wantStatus: taskstate.StatusBlocked, wantEvidence: "DENIED"},
		{name: "allow persists contained mutation", allow: true, wantDecision: perm.Allow, wantStatus: taskstate.StatusVerifying, wantFile: true, wantEvidence: "APPROVED", wantUndo: true, wantChangeSeq: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newRenderedPermissionAgent(t)
			defer fixture.agent.Close()

			runCh := make(chan struct {
				result agent.Result
				err    error
			}, 1)
			go func() {
				_, result, err := fixture.agent.RunWithOptions(context.Background(), nil, llm.Message{
					Role:    "user",
					Content: "create the permission-protected rendered probe file",
				}, fixture.handler, agent.RunOptions{})
				runCh <- struct {
					result agent.Result
					err    error
				}{result: result, err: err}
			}()

			waitForRenderedPermission(t, fixture.server)
			before := readRenderedState(t, fixture.server)
			pending, ok := before["pending_perm"].(map[string]any)
			if !ok || pending["tool"] != "write_file" {
				t.Fatalf("pending permission = %#v, want write_file", before["pending_perm"])
			}
			if _, err := os.Stat(fixture.path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("workspace changed before decision: %v", err)
			}

			postRenderedPermission(t, fixture.server, tc.allow)
			var run struct {
				result agent.Result
				err    error
			}
			select {
			case run = <-runCh:
			case <-time.After(5 * time.Second):
				t.Fatal("deterministic provider turn did not finish")
			}
			if run.err != nil {
				t.Fatal(run.err)
			}
			if run.result.Task == nil {
				t.Fatal("agent returned no durable task")
			}
			if run.result.Task.Status != tc.wantStatus {
				t.Fatalf("result task status = %s, want %s", run.result.Task.Status, tc.wantStatus)
			}
			if run.result.UndoAvailable != tc.wantUndo {
				t.Fatalf("result undo_available = %v, want %v", run.result.UndoAvailable, tc.wantUndo)
			}

			if tc.wantFile {
				got, err := os.ReadFile(fixture.path)
				if err != nil {
					t.Fatalf("allowed file missing: %v", err)
				}
				if string(got) != renderedPermissionProbeContent {
					t.Fatalf("allowed file = %q, want %q", got, renderedPermissionProbeContent)
				}
			} else if _, err := os.Stat(fixture.path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("denied request changed the workspace: %v", err)
			}

			task := fixture.agent.TaskSnapshot()
			if task == nil || len(task.ChangedFiles) != tc.wantChangeSeq {
				t.Fatalf("durable changed files = %#v, want %d entries", task, tc.wantChangeSeq)
			}
			if task != nil && task.ChangeSeq != tc.wantChangeSeq {
				t.Fatalf("durable change sequence = %d, want %d", task.ChangeSeq, tc.wantChangeSeq)
			}
			if tc.wantFile && (task == nil || !containsString(task.ChangedFiles, "rendered-probe.txt")) {
				t.Fatalf("durable changed files = %#v, want rendered-probe.txt", task)
			}
			if task == nil || len(task.Evidence) == 0 {
				t.Fatalf("durable permission evidence missing: %#v", task)
			}
			latest := task.Evidence[len(task.Evidence)-1]
			if latest.Kind != taskstate.EvidenceKindApproval || latest.Status != tc.wantEvidence || latest.Origin != taskstate.EvidenceOriginUserApproval {
				t.Fatalf("latest permission evidence = %#v, want approval/%s/user_approval", latest, tc.wantEvidence)
			}
			status, current, origin := task.RequirementEvidenceState(taskstate.EvidenceKindApproval)
			if status != tc.wantEvidence || !current || origin != taskstate.EvidenceOriginUserApproval {
				t.Fatalf("approval requirement = status:%s current:%v origin:%s", status, current, origin)
			}

			after := readRenderedState(t, fixture.server)
			if _, present := after["pending_perm"]; present {
				t.Fatalf("pending permission remained after decision: %#v", after["pending_perm"])
			}
			if got, ok := after["undo_available"].(bool); !ok || got != tc.wantUndo {
				t.Fatalf("rendered undo_available = %#v, want %v", after["undo_available"], tc.wantUndo)
			}
			stateTask, ok := after["task"].(map[string]any)
			if !ok {
				t.Fatalf("rendered task projection = %#v", after["task"])
			}
			if got := renderedTaskEvidenceStatus(stateTask); got != tc.wantEvidence {
				t.Fatalf("rendered approval evidence = %q, want %q", got, tc.wantEvidence)
			}
			if got := renderedTaskChangedFiles(stateTask); tc.wantFile && !containsString(got, "rendered-probe.txt") {
				t.Fatalf("rendered changed files = %#v, want probe", got)
			}

			events := drainRenderedEvents(fixture.events)
			if !hasPermissionEvent(events, tc.wantDecision) {
				t.Fatalf("events did not include permission prompt: %#v", eventTypes(events))
			}
			if !hasTaskProjection(events, tc.wantEvidence, tc.wantFile) {
				t.Fatalf("events did not project permission/mutation evidence: %#v", eventTypes(events))
			}
		})
	}
}

func TestRenderedPermissionRepeatedAndStateIsolated(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	for iteration := 0; iteration < 8; iteration++ {
		allow := iteration%2 == 1
		name := "deny"
		if allow {
			name = "allow"
		}
		t.Run(name+"-"+string(rune('0'+iteration)), func(t *testing.T) {
			fixture := newRenderedPermissionAgent(t)
			defer fixture.agent.Close()
			runCh := runRenderedPermissionTurn(fixture)
			waitForRenderedPermission(t, fixture.server)
			postRenderedPermission(t, fixture.server, allow)
			select {
			case run := <-runCh:
				if run.err != nil {
					t.Fatal(run.err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("repeated permission turn did not finish")
			}
			_, err := os.Stat(fixture.path)
			if allow && err != nil {
				t.Fatalf("iteration %d allowed file missing: %v", iteration, err)
			}
			if !allow && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("iteration %d denied file exists: %v", iteration, err)
			}
			state := readRenderedState(t, fixture.server)
			if _, present := state["pending_perm"]; present {
				t.Fatalf("iteration %d leaked pending permission", iteration)
			}
		})
	}
}

func TestRenderedPermissionStateReadRaceWithDecision(t *testing.T) {
	t.Setenv("PICOGENT_HOME", t.TempDir())
	fixture := newRenderedPermissionAgent(t)
	defer fixture.agent.Close()
	runCh := runRenderedPermissionTurn(fixture)
	waitForRenderedPermission(t, fixture.server)

	readersDone := make(chan struct{})
	for i := 0; i < 6; i++ {
		go func() {
			for j := 0; j < 40; j++ {
				_ = readRenderedState(t, fixture.server)
			}
			readersDone <- struct{}{}
		}()
	}
	postRenderedPermission(t, fixture.server, true)
	for i := 0; i < 6; i++ {
		select {
		case <-readersDone:
		case <-time.After(5 * time.Second):
			t.Fatal("state reader did not finish")
		}
	}
	select {
	case run := <-runCh:
		if run.err != nil {
			t.Fatal(run.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("raced permission turn did not finish")
	}
	if _, err := os.Stat(fixture.path); err != nil {
		t.Fatalf("raced allow did not publish file: %v", err)
	}
}

const renderedPermissionProbeContent = "rendered permission probe\n"

type renderedPermissionFixture struct {
	server    *server
	agent     *agent.Agent
	handler   *guiHandler
	store     *taskstate.Store
	events    chan event
	workspace string
	path      string
}

func newRenderedPermissionAgent(t *testing.T) renderedPermissionFixture {
	t.Helper()
	workspace := t.TempDir()
	store := taskstate.NewStore(t.TempDir())
	args, err := json.Marshal(map[string]string{
		"path":    "rendered-probe.txt",
		"content": renderedPermissionProbeContent,
	})
	if err != nil {
		t.Fatal(err)
	}
	client := &llm.Scripted{Responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "write-probe", Name: "write_file", Arguments: string(args)}}}},
		{Message: llm.Message{Role: "assistant", Content: "The requested file change is complete."}},
	}}
	cfg := config.Default()
	cfg.Provider = config.ProviderOllama
	cfg.Mode = config.ModeSafe
	cfg.Workspace = workspace
	reg := tools.NewRegistry(tools.Context{Workspace: workspace})
	ag := agent.New(cfg, client, reg, perm.New(config.ModeSafe, workspace, nil))
	ag.SetTaskStore(store)
	const sessionID = "rendered-permission-integration"
	if err := ag.SetTaskSession(sessionID); err != nil {
		t.Fatal(err)
	}
	events := make(chan event, 256)
	s := &server{
		cfg:       cfg,
		ag:        ag,
		sessionID: sessionID,
		turnGen:   1,
		permCh:    make(chan perm.Decision, 1),
		subs:      []chan event{events},
	}
	h := newGUIHandlerAtWithPerm(s, sessionID, 1, s.permCh, workspace)
	return renderedPermissionFixture{
		server:    s,
		agent:     ag,
		handler:   h,
		store:     store,
		events:    events,
		workspace: workspace,
		path:      filepath.Join(workspace, "rendered-probe.txt"),
	}
}

func runRenderedPermissionTurn(fixture renderedPermissionFixture) <-chan struct {
	result agent.Result
	err    error
} {
	runCh := make(chan struct {
		result agent.Result
		err    error
	}, 1)
	go func() {
		_, result, err := fixture.agent.RunWithOptions(context.Background(), nil, llm.Message{
			Role:    "user",
			Content: "create the permission-protected rendered probe file",
		}, fixture.handler, agent.RunOptions{})
		runCh <- struct {
			result agent.Result
			err    error
		}{result: result, err: err}
	}()
	return runCh
}

func waitForRenderedPermission(t *testing.T, s *server) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		s.mu.Lock()
		pending := s.pendingPerm.Tool
		s.mu.Unlock()
		if pending == "write_file" {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("permission request was not exposed to the GUI boundary")
		case <-ticker.C:
		}
	}
}

func postRenderedPermission(t *testing.T, s *server, allow bool) {
	t.Helper()
	body := `{"allow":false}`
	if allow {
		body = `{"allow":true}`
	}
	req := loopbackAPIRequest(http.MethodPost, "/api/permission", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://"+loopbackTestHost)
	res := httptest.NewRecorder()
	s.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("permission response status = %d, want %d", res.Code, http.StatusNoContent)
	}
}

func readRenderedState(t *testing.T, s *server) map[string]any {
	t.Helper()
	res := httptest.NewRecorder()
	s.Handler().ServeHTTP(res, loopbackAPIRequest(http.MethodGet, "/api/state", ""))
	if res.Code != http.StatusOK {
		t.Fatalf("state response status = %d, want %d", res.Code, http.StatusOK)
	}
	var state map[string]any
	if err := json.NewDecoder(res.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

func drainRenderedEvents(events <-chan event) []event {
	var out []event
	for {
		select {
		case e := <-events:
			out = append(out, e)
		default:
			return out
		}
	}
}

func hasPermissionEvent(events []event, want perm.Decision) bool {
	for _, e := range events {
		if e.Type == "permission" && e.Text == "write_file" {
			return want == perm.Deny || e.Summary != ""
		}
	}
	return false
}

func hasTaskProjection(events []event, wantEvidence string, wantFile bool) bool {
	for _, e := range events {
		if e.Type != "task_progress" || e.Task == nil {
			continue
		}
		if renderedTaskEvidenceStatusFromTask(e.Task) != wantEvidence {
			continue
		}
		if wantFile && !containsString(e.Task.ChangedFiles, "rendered-probe.txt") {
			continue
		}
		return true
	}
	return false
}

func renderedTaskEvidenceStatus(task map[string]any) string {
	evidence, ok := task["evidence"].([]any)
	if !ok || len(evidence) == 0 {
		return ""
	}
	latest, ok := evidence[len(evidence)-1].(map[string]any)
	if !ok {
		return ""
	}
	status, _ := latest["status"].(string)
	return status
}

func renderedTaskEvidenceStatusFromTask(task *taskstate.Task) string {
	if task == nil || len(task.Evidence) == 0 {
		return ""
	}
	return task.Evidence[len(task.Evidence)-1].Status
}

func renderedTaskChangedFiles(task map[string]any) []string {
	values, ok := task["changed_files"].([]any)
	if !ok {
		return nil
	}
	files := make([]string, 0, len(values))
	for _, value := range values {
		if file, ok := value.(string); ok {
			files = append(files, file)
		}
	}
	return files
}

func eventTypes(events []event) string {
	types := make([]string, 0, len(events))
	for _, e := range events {
		types = append(types, e.Type+":"+strings.TrimSpace(e.Text))
	}
	return strings.Join(types, ",")
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
