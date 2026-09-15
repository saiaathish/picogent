package agent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/projecthealth"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
)

const admissionHealthMarker = "repository-health-payload-must-stay-private"

type admissionModel struct {
	responses []llm.ChatResponse
	calls     []llm.ChatRequest
	order     *[]string
}

func (m *admissionModel) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	if m.order != nil {
		*m.order = append(*m.order, "model")
	}
	m.calls = append(m.calls, req)
	if len(m.responses) == 0 {
		return llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: "done"}}, nil
	}
	response := m.responses[0]
	m.responses = m.responses[1:]
	return response, nil
}

type admissionToolEnd struct {
	call   llm.ToolCall
	result string
	err    error
}

type admissionEvents struct {
	NopHandler
	starts []llm.ToolCall
	ends   []admissionToolEnd
}

func (e *admissionEvents) OnToolStart(call llm.ToolCall) {
	e.starts = append(e.starts, call)
}

func (e *admissionEvents) OnToolEnd(call llm.ToolCall, result string, err error) {
	e.ends = append(e.ends, admissionToolEnd{call: call, result: result, err: err})
}

func (e *admissionEvents) OnNeedPermission(context.Context, perm.Request) (perm.Decision, error) {
	return perm.Allow, nil
}

func newAdmissionAgent(t *testing.T, client llm.Client, mode config.Mode) (*Agent, *tools.Registry, string) {
	t.Helper()
	workspace := t.TempDir()
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Mode = mode
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{Workspace: workspace})
	a := New(cfg, client, reg, perm.New(mode, workspace, nil))
	return a, reg, workspace
}

func admissionHealthOutput(t *testing.T) string {
	t.Helper()
	report := projecthealth.Report{
		Schema: projecthealth.Schema,
		Status: projecthealth.StateAttention,
		Provenance: projecthealth.Provenance{
			HeadKnown:  true,
			DirtyKnown: true,
		},
		Findings: []projecthealth.Finding{{
			ID:         "build-health",
			Dimension:  "build",
			Priority:   90,
			Severity:   projecthealth.SeverityHigh,
			Confidence: "high",
			Title:      admissionHealthMarker,
			Evidence:   admissionHealthMarker,
			NextAction: admissionHealthMarker,
		}},
	}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func setAdmissionTask(t *testing.T, a *Agent, prompt string) *taskstate.Task {
	t.Helper()
	task, ok, err := taskstate.NewFromPrompt("admission-session", prompt)
	if err != nil || !ok || task == nil {
		t.Fatalf("new task = %#v, ok=%v, err=%v", task, ok, err)
	}
	a.taskMu.Lock()
	a.task = task
	a.taskMu.Unlock()
	return task
}

func containsAdmissionMarker(messages []llm.Message, marker string) bool {
	for _, message := range messages {
		if strings.Contains(message.Content, marker) {
			return true
		}
		for _, part := range message.Parts {
			if strings.Contains(part.Text, marker) {
				return true
			}
		}
	}
	return false
}

func containsProjectHealth(specs []llm.ToolSpec) bool {
	for _, spec := range specs {
		if spec.Name == "project_health" {
			return true
		}
	}
	return false
}

func TestEligibleForProjectHealthAdmissionUsesExistingIntentBoundary(t *testing.T) {
	for _, test := range []struct {
		name          string
		prompt        string
		mode          TaskMode
		scopeBoundary string
		want          bool
	}{
		{name: "broad outcome", prompt: "make this ready to launch", mode: TaskAgent, want: true},
		{name: "tiny request", prompt: "change the button text", mode: TaskAgent},
		{name: "concrete bug", prompt: "fix the login error", mode: TaskAgent},
		{name: "ask mode", prompt: "make this ready to launch", mode: TaskAsk},
		{name: "plan mode", prompt: "make this ready to launch", mode: TaskPlan},
		{name: "scoped broad request", prompt: "make this ready to launch", mode: TaskAgent, scopeBoundary: "only inspect the landing page"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := eligibleForProjectHealthAdmission(test.prompt, test.mode, test.scopeBoundary); got != test.want {
				t.Fatalf("eligibleForProjectHealthAdmission() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestProjectHealthAdmissionRunsBeforeFirstModelCallAndKeepsEvidenceBounded(t *testing.T) {
	var order []string
	model := &admissionModel{
		order: &order,
		responses: []llm.ChatResponse{{
			Message: llm.Message{Role: "assistant", Content: "ready"},
		}},
	}
	a, _, _ := newAdmissionAgent(t, model, config.ModeFast)
	healthOutput := admissionHealthOutput(t)
	healthCalls := 0
	a.runTool = func(_ context.Context, call llm.ToolCall, _ tools.Tool, _ tools.Context) (string, error) {
		healthCalls++
		if call.Name != "project_health" || call.ID != projectHealthAdmissionCallID || call.Arguments != "{}" {
			t.Fatalf("admission call = %#v", call)
		}
		order = append(order, "health")
		return healthOutput, nil
	}
	events := &admissionEvents{}
	history, _, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "make this ready to launch"}, events)
	if err != nil {
		t.Fatal(err)
	}
	if healthCalls != 1 {
		t.Fatalf("project_health calls = %d, want 1", healthCalls)
	}
	if !reflect.DeepEqual(order, []string{"health", "model"}) {
		t.Fatalf("call order = %#v, want health before model", order)
	}
	if len(model.calls) != 1 {
		t.Fatalf("model calls = %d, want 1", len(model.calls))
	}
	if containsProjectHealth(model.calls[0].Tools) {
		t.Fatal("project_health remained advertised after admission")
	}
	focus := ""
	for _, message := range model.calls[0].Messages {
		if strings.Contains(message.Content, "Internal outcome focus:") {
			focus = message.Content
		}
	}
	if !strings.Contains(focus, "Health observation: status=ATTENTION") {
		t.Fatalf("first model request did not receive fresh health focus: %q", focus)
	}
	if containsAdmissionMarker(model.calls[0].Messages, admissionHealthMarker) || containsAdmissionMarker(history, admissionHealthMarker) {
		t.Fatal("raw project-health payload reached model messages or returned history")
	}
	if len(events.starts) != 0 || len(events.ends) != 0 {
		t.Fatalf("invisible admission emitted tool events: starts=%#v ends=%#v", events.starts, events.ends)
	}
}

func TestProjectHealthAdmissionSkipsIneligibleRunModesAndScopes(t *testing.T) {
	for _, test := range []struct {
		name          string
		prompt        string
		mode          TaskMode
		scopeBoundary string
	}{
		{name: "tiny", prompt: "change the button text", mode: TaskAgent},
		{name: "concrete", prompt: "fix the login error", mode: TaskAgent},
		{name: "ask", prompt: "make this ready to launch", mode: TaskAsk},
		{name: "plan", prompt: "make this ready to launch", mode: TaskPlan},
		{name: "scoped", prompt: "make this ready to launch", mode: TaskAgent, scopeBoundary: "only inspect the landing page"},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := &admissionModel{responses: []llm.ChatResponse{{
				Message: llm.Message{Role: "assistant", Content: "done"},
			}}}
			a, _, _ := newAdmissionAgent(t, model, config.ModeFast)
			healthCalls := 0
			a.runTool = func(_ context.Context, call llm.ToolCall, _ tools.Tool, _ tools.Context) (string, error) {
				healthCalls++
				if call.Name == "project_health" {
					return admissionHealthOutput(t), nil
				}
				return "ok", nil
			}
			_, _, err := a.RunWithOptions(context.Background(), nil, llm.Message{Role: "user", Content: test.prompt}, &admissionEvents{}, RunOptions{
				TaskMode:      &test.mode,
				ScopeBoundary: test.scopeBoundary,
			})
			if err != nil {
				t.Fatal(err)
			}
			if healthCalls != 0 {
				t.Fatalf("project_health calls = %d, want 0", healthCalls)
			}
		})
	}
}

func TestProjectHealthAdmissionPreservesSafeAndFastReadOnlyPolicy(t *testing.T) {
	for _, mode := range []config.Mode{config.ModeSafe, config.ModeFast} {
		t.Run(string(mode), func(t *testing.T) {
			model := &admissionModel{responses: []llm.ChatResponse{{
				Message: llm.Message{Role: "assistant", Content: "done"},
			}}}
			a, _, _ := newAdmissionAgent(t, model, mode)
			healthCalls := 0
			a.runTool = func(_ context.Context, call llm.ToolCall, _ tools.Tool, _ tools.Context) (string, error) {
				if call.Name != "project_health" {
					t.Fatalf("unexpected tool during admission = %q", call.Name)
				}
				healthCalls++
				return admissionHealthOutput(t), nil
			}
			if _, _, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "make this ready to launch"}, &admissionEvents{}); err != nil {
				t.Fatal(err)
			}
			if healthCalls != 1 {
				t.Fatalf("project_health calls = %d, want 1 in %s mode", healthCalls, mode)
			}
		})
	}
}

func TestProjectHealthAdmissionHealthFocusExpiresAfterWrite(t *testing.T) {
	var order []string
	writeArgs, err := json.Marshal(map[string]string{"path": "note.txt", "content": "changed"})
	if err != nil {
		t.Fatal(err)
	}
	model := &admissionModel{
		order: &order,
		responses: []llm.ChatResponse{
			{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{
				ID:        "write-note",
				Name:      "write_file",
				Arguments: string(writeArgs),
			}}}},
			{Message: llm.Message{Role: "assistant", Content: "done"}},
		},
	}
	a, _, workspace := newAdmissionAgent(t, model, config.ModeFast)
	a.SetTaskStore(taskstate.NewStore(t.TempDir()))
	if err := a.SetTaskSession("admission-write-invalidation"); err != nil {
		t.Fatal(err)
	}
	healthOutput := admissionHealthOutput(t)
	healthCalls := 0
	a.runTool = func(ctx context.Context, call llm.ToolCall, tool tools.Tool, toolCtx tools.Context) (string, error) {
		order = append(order, call.Name)
		if call.Name == "project_health" {
			healthCalls++
			return healthOutput, nil
		}
		return tool.Run(ctx, call.Arguments, toolCtx)
	}
	if _, result, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "make this ready to launch"}, &admissionEvents{}); err != nil {
		t.Fatal(err)
	} else if !reflect.DeepEqual(result.FilesChanged, []string{"note.txt"}) {
		t.Fatalf("changed files = %#v, want note.txt", result.FilesChanged)
	}
	if healthCalls != 1 || len(model.calls) != 2 {
		t.Fatalf("health/model calls = %d/%d, want 1/2", healthCalls, len(model.calls))
	}
	if !reflect.DeepEqual(order, []string{"project_health", "model", "write_file", "model"}) {
		t.Fatalf("call order = %#v, want admission, model, write, model", order)
	}
	firstFocus := ""
	secondFocus := ""
	for _, message := range model.calls[0].Messages {
		if strings.Contains(message.Content, "Internal outcome focus:") {
			firstFocus = message.Content
		}
	}
	for _, message := range model.calls[1].Messages {
		if strings.Contains(message.Content, "Internal outcome focus:") {
			secondFocus = message.Content
		}
	}
	if !strings.Contains(firstFocus, "Health observation: status=ATTENTION") {
		t.Fatalf("first model request missing health focus: %q", firstFocus)
	}
	if !strings.Contains(secondFocus, "Health observation: status=UNKNOWN") || strings.Contains(secondFocus, "status=ATTENTION") {
		t.Fatalf("write did not invalidate health focus: %q", secondFocus)
	}
	if containsAdmissionMarker(model.calls[0].Messages, admissionHealthMarker) || containsAdmissionMarker(model.calls[1].Messages, admissionHealthMarker) {
		t.Fatal("raw health payload reached model messages")
	}
	if content, err := os.ReadFile(filepath.Join(workspace, "note.txt")); err != nil || string(content) != "changed" {
		t.Fatalf("written note = %q, err=%v", content, err)
	}
}

func TestProjectHealthAdmissionSuppressesProviderDuplicate(t *testing.T) {
	model := &admissionModel{responses: []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{
			ID:        "provider-duplicate",
			Name:      "project_health",
			Arguments: `{"ignored":true}`,
		}}}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}}
	a, _, _ := newAdmissionAgent(t, model, config.ModeFast)
	healthCalls := 0
	a.runTool = func(_ context.Context, call llm.ToolCall, _ tools.Tool, _ tools.Context) (string, error) {
		healthCalls++
		if call.Name != "project_health" {
			t.Fatalf("unexpected tool = %q", call.Name)
		}
		return admissionHealthOutput(t), nil
	}
	events := &admissionEvents{}
	history, _, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "make this ready to launch"}, events)
	if err != nil {
		t.Fatal(err)
	}
	if healthCalls != 1 || len(model.calls) != 2 {
		t.Fatalf("health/model calls = %d/%d, want 1/2", healthCalls, len(model.calls))
	}
	for i, call := range model.calls {
		if containsProjectHealth(call.Tools) {
			t.Fatalf("model call %d re-advertised project_health", i)
		}
	}
	if len(events.starts) != 0 || len(events.ends) != 0 {
		t.Fatalf("suppressed duplicate emitted tool events: starts=%#v ends=%#v", events.starts, events.ends)
	}
	foundSyntheticResult := false
	for _, message := range model.calls[1].Messages {
		if message.Role == "tool" && message.ToolCallID == "provider-duplicate" && message.Content == projectHealthAdmissionAlreadyAttempted {
			foundSyntheticResult = true
			break
		}
	}
	if !foundSyntheticResult {
		t.Fatal("duplicate project_health call did not receive the bounded synthetic result")
	}
	if containsAdmissionMarker(history, admissionHealthMarker) {
		t.Fatal("raw health payload reached returned history after duplicate suppression")
	}
}

func TestProjectHealthAdmissionFallsBackForMalformedAndFailedOutput(t *testing.T) {
	for _, test := range []struct {
		name   string
		raw    string
		runErr error
	}{
		{name: "malformed", raw: "{"},
		{name: "failed", runErr: errors.New("health scan failed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			a, reg, workspace := newAdmissionAgent(t, &admissionModel{}, config.ModeFast)
			task := setAdmissionTask(t, a, "make this ready to launch")
			a.runTool = func(_ context.Context, call llm.ToolCall, _ tools.Tool, _ tools.Context) (string, error) {
				if call.Name != "project_health" {
					t.Fatalf("unexpected tool = %q", call.Name)
				}
				return test.raw, test.runErr
			}
			state := a.RuntimeSnapshot()
			got := a.admitProjectHealth(context.Background(), "make this ready to launch", TaskAgent, "", state, reg.ContextSnapshot(), perm.New(config.ModeFast, workspace, nil))
			want := outcomeFocusForTask(task)
			if !got.attempted {
				t.Fatal("ineligible admission result reported no attempt")
			}
			if got.focus != want {
				t.Fatalf("fallback focus = %q, want %q", got.focus, want)
			}
			if strings.Contains(got.focus, admissionHealthMarker) {
				t.Fatal("malformed or failed health payload escaped into focus")
			}
		})
	}
}

func TestProjectHealthAdmissionInvalidatesHealthAfterSteeringOrWorkspaceChange(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*Agent, string)
	}{
		{name: "steering", mutate: func(a *Agent, _ string) { a.SetGoal("a different outcome") }},
		{name: "workspace change", mutate: func(a *Agent, _ string) {
			a.UpdateConfig(func(cfg *config.Config) { cfg.Workspace = filepath.Join(cfg.Workspace, "replacement") })
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			a, reg, workspace := newAdmissionAgent(t, &admissionModel{}, config.ModeFast)
			task := setAdmissionTask(t, a, "make this ready to launch")
			a.runTool = func(_ context.Context, call llm.ToolCall, _ tools.Tool, _ tools.Context) (string, error) {
				if call.Name != "project_health" {
					t.Fatalf("unexpected tool = %q", call.Name)
				}
				test.mutate(a, workspace)
				return admissionHealthOutput(t), nil
			}
			state := a.RuntimeSnapshot()
			got := a.admitProjectHealth(context.Background(), "make this ready to launch", TaskAgent, "", state, reg.ContextSnapshot(), perm.New(config.ModeFast, workspace, nil))
			want := outcomeFocusForTask(task)
			if !got.attempted {
				t.Fatal("stale admission result reported no attempt")
			}
			if got.focus != want {
				t.Fatalf("stale health focus = %q, want task-only fallback %q", got.focus, want)
			}
			if strings.Contains(got.focus, "Health observation: status=ATTENTION") {
				t.Fatal("stale health observation remained actionable after invalidation")
			}
		})
	}
}

func TestProjectHealthAdmissionFallsBackWhenRuntimeOrToolIsUnavailable(t *testing.T) {
	a, reg, workspace := newAdmissionAgent(t, &admissionModel{}, config.ModeFast)
	task := setAdmissionTask(t, a, "make this ready to launch")
	state := a.RuntimeSnapshot()
	want := outcomeFocusForTask(task)

	tests := []struct {
		name  string
		state RuntimeState
		gate  *perm.Gate
	}{
		{name: "nil tools", state: func() RuntimeState {
			copy := state
			copy.Tools = nil
			return copy
		}(), gate: perm.New(config.ModeFast, workspace, nil)},
		{name: "nil gate", state: state, gate: nil},
		{name: "missing health tool", state: func() RuntimeState {
			copy := state
			copy.Tools = &tools.Registry{}
			return copy
		}(), gate: perm.New(config.ModeFast, workspace, nil)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := a.admitProjectHealth(context.Background(), "make this ready to launch", TaskAgent, "", test.state, reg.ContextSnapshot(), test.gate)
			if !got.attempted {
				t.Fatal("fallback admission reported no attempt")
			}
			if got.focus != want {
				t.Fatalf("fallback focus = %q, want %q", got.focus, want)
			}
		})
	}
}

func TestProjectHealthAdmissionFallsBackOnPermissionDenialOrError(t *testing.T) {
	a, reg, workspace := newAdmissionAgent(t, &admissionModel{}, config.ModeFast)
	task := setAdmissionTask(t, a, "make this ready to launch")
	reg.UpdateContext(func(c *tools.Context) {
		c.ClassifyPath = func(tool, path, _, _ string) perm.Request {
			return perm.Request{Tool: tool, Path: path, OutsideWorkspace: true}
		}
	})
	state := a.RuntimeSnapshot()
	regCtx := reg.ContextSnapshot()
	want := outcomeFocusForTask(task)
	for _, test := range []struct {
		name     string
		decision perm.Decision
		err      error
	}{
		{name: "denied", decision: perm.Deny},
		{name: "prompt error", decision: perm.Deny, err: errors.New("permission prompt failed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			runCalls := 0
			a.runTool = func(context.Context, llm.ToolCall, tools.Tool, tools.Context) (string, error) {
				runCalls++
				return admissionHealthOutput(t), nil
			}
			gate := perm.New(config.ModeSafe, workspace, func(context.Context, perm.Request) (perm.Decision, error) {
				return test.decision, test.err
			})
			got := a.admitProjectHealth(context.Background(), "make this ready to launch", TaskAgent, "", state, regCtx, gate)
			if !got.attempted {
				t.Fatal("permission fallback reported no attempt")
			}
			if got.focus != want {
				t.Fatalf("permission fallback focus = %q, want %q", got.focus, want)
			}
			if runCalls != 0 {
				t.Fatalf("project_health ran after permission failure: %d calls", runCalls)
			}
		})
	}
}

func TestProjectHealthAdmissionRejectsFreshnessChangeBeforeNativeRun(t *testing.T) {
	a, reg, workspace := newAdmissionAgent(t, &admissionModel{}, config.ModeFast)
	task := setAdmissionTask(t, a, "make this ready to launch")
	reg.UpdateContext(func(c *tools.Context) {
		c.ClassifyPath = func(tool, path, _, _ string) perm.Request {
			return perm.Request{Tool: tool, Path: path, OutsideWorkspace: true}
		}
	})
	state := a.RuntimeSnapshot()
	regCtx := reg.ContextSnapshot()
	runCalls := 0
	a.runTool = func(context.Context, llm.ToolCall, tools.Tool, tools.Context) (string, error) {
		runCalls++
		return admissionHealthOutput(t), nil
	}
	gate := perm.New(config.ModeSafe, workspace, func(context.Context, perm.Request) (perm.Decision, error) {
		a.SetGoal("replacement outcome")
		return perm.Allow, nil
	})
	got := a.admitProjectHealth(context.Background(), "make this ready to launch", TaskAgent, "", state, regCtx, gate)
	if !got.attempted {
		t.Fatal("stale pre-run admission reported no attempt")
	}
	if got.focus != outcomeFocusForTask(task) {
		t.Fatalf("stale pre-run focus = %q, want task-only focus %q", got.focus, outcomeFocusForTask(task))
	}
	if runCalls != 0 {
		t.Fatalf("stale pre-run health was executed: %d calls", runCalls)
	}
}

func TestProjectHealthAdmissionRejectsPostRunIdentityChange(t *testing.T) {
	a, reg, workspace := newAdmissionAgent(t, &admissionModel{}, config.ModeFast)
	task := setAdmissionTask(t, a, "make this ready to launch")
	moved := workspace + "-replaced"
	t.Cleanup(func() {
		_ = os.RemoveAll(workspace)
		_ = os.RemoveAll(moved)
	})
	a.runTool = func(context.Context, llm.ToolCall, tools.Tool, tools.Context) (string, error) {
		if err := os.Rename(workspace, moved); err != nil {
			return "", err
		}
		if err := os.Mkdir(workspace, 0o700); err != nil {
			return "", err
		}
		return admissionHealthOutput(t), nil
	}
	state := a.RuntimeSnapshot()
	got := a.admitProjectHealth(context.Background(), "make this ready to launch", TaskAgent, "", state, reg.ContextSnapshot(), perm.New(config.ModeFast, workspace, nil))
	if !got.attempted {
		t.Fatal("stale post-run admission reported no attempt")
	}
	if got.focus != outcomeFocusForTask(task) {
		t.Fatalf("stale post-run focus = %q, want task-only focus %q", got.focus, outcomeFocusForTask(task))
	}
	if strings.Contains(got.focus, "status=ATTENTION") {
		t.Fatalf("stale health observation remained actionable: %q", got.focus)
	}
}

func TestProjectHealthAdmissionUsesNativeToolRunner(t *testing.T) {
	a, reg, workspace := newAdmissionAgent(t, &admissionModel{}, config.ModeFast)
	task := setAdmissionTask(t, a, "make this ready to launch")
	state := a.RuntimeSnapshot()
	got := a.admitProjectHealth(context.Background(), "make this ready to launch", TaskAgent, "", state, reg.ContextSnapshot(), perm.New(config.ModeFast, workspace, nil))
	if !got.attempted {
		t.Fatal("native admission reported no attempt")
	}
	if got.focus == outcomeFocusForTask(task) {
		t.Fatalf("native project_health did not replace unknown focus: %q", got.focus)
	}
	if !strings.Contains(got.focus, "Health observation: status=") {
		t.Fatalf("native health focus = %q", got.focus)
	}
	if strings.Contains(got.focus, admissionHealthMarker) {
		t.Fatal("native health payload marker escaped into focus")
	}
}

func TestProjectHealthAdmissionRejectsRegistryPointerReplacement(t *testing.T) {
	a, reg, workspace := newAdmissionAgent(t, &admissionModel{}, config.ModeFast)
	task := setAdmissionTask(t, a, "make this ready to launch")
	state := a.RuntimeSnapshot()
	a.runTool = func(context.Context, llm.ToolCall, tools.Tool, tools.Context) (string, error) {
		a.stateMu.Lock()
		a.Tools = tools.NewRegistry(tools.Context{Workspace: workspace})
		a.stateMu.Unlock()
		return admissionHealthOutput(t), nil
	}
	got := a.admitProjectHealth(context.Background(), "make this ready to launch", TaskAgent, "", state, reg.ContextSnapshot(), perm.New(config.ModeFast, workspace, nil))
	if !got.attempted {
		t.Fatal("registry-replacement admission reported no attempt")
	}
	if got.focus != outcomeFocusForTask(task) {
		t.Fatalf("registry replacement focus = %q, want task-only focus %q", got.focus, outcomeFocusForTask(task))
	}
}

func TestProjectHealthAdmissionRejectsRegistrySessionAndTaskChanges(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*Agent, *tools.Registry, string, *taskstate.Task) error
	}{
		{name: "registry context", mutate: func(_ *Agent, reg *tools.Registry, workspace string, _ *taskstate.Task) error {
			reg.UpdateContext(func(c *tools.Context) { c.Workspace = filepath.Join(workspace, "replacement") })
			return nil
		}},
		{name: "session", mutate: func(a *Agent, _ *tools.Registry, _ string, _ *taskstate.Task) error {
			return a.SetTaskSession("replacement-session")
		}},
		{name: "task", mutate: func(a *Agent, _ *tools.Registry, _ string, _ *taskstate.Task) error {
			a.taskMu.Lock()
			if a.task != nil {
				a.task.Attempts++
			}
			a.taskMu.Unlock()
			return nil
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			a, reg, workspace := newAdmissionAgent(t, &admissionModel{}, config.ModeFast)
			setAdmissionTask(t, a, "make this ready to launch")
			state := a.RuntimeSnapshot()
			a.runTool = func(context.Context, llm.ToolCall, tools.Tool, tools.Context) (string, error) {
				if err := test.mutate(a, reg, workspace, a.TaskSnapshot()); err != nil {
					t.Fatalf("mutate runtime state: %v", err)
				}
				return admissionHealthOutput(t), nil
			}
			got := a.admitProjectHealth(context.Background(), "make this ready to launch", TaskAgent, "", state, reg.ContextSnapshot(), perm.New(config.ModeFast, workspace, nil))
			if !got.attempted {
				t.Fatal("stale mutation admission reported no attempt")
			}
			if got.focus != outcomeFocusForTask(a.TaskSnapshot()) {
				t.Fatalf("stale mutation focus = %q, want current task-only focus %q", got.focus, outcomeFocusForTask(a.TaskSnapshot()))
			}
			if strings.Contains(got.focus, "status=ATTENTION") {
				t.Fatalf("stale health observation remained actionable: %q", got.focus)
			}
		})
	}
}

func TestProjectHealthAdmissionFallsBackWhenContextIsCanceled(t *testing.T) {
	a, reg, workspace := newAdmissionAgent(t, &admissionModel{}, config.ModeFast)
	task := setAdmissionTask(t, a, "make this ready to launch")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a.runTool = func(ctx context.Context, _ llm.ToolCall, _ tools.Tool, _ tools.Context) (string, error) {
		return "", ctx.Err()
	}
	got := a.admitProjectHealth(ctx, "make this ready to launch", TaskAgent, "", a.RuntimeSnapshot(), reg.ContextSnapshot(), perm.New(config.ModeFast, workspace, nil))
	if !got.attempted {
		t.Fatal("canceled admission reported no attempt")
	}
	if got.focus != outcomeFocusForTask(task) {
		t.Fatalf("canceled admission focus = %q, want %q", got.focus, outcomeFocusForTask(task))
	}
}
