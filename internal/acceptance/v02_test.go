// Package acceptance exercises the release-defining v0.2 loop against a real
// temporary Go repository: inspect, edit, verify, persist task state, and undo.
package acceptance

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
	"github.com/saiaathish/picogent/internal/verify"
)

const buggyTodo = `package todo

type List struct {
	items []string
}

func New(items ...string) *List {
	return &List{items: append([]string(nil), items...)}
}

func (l *List) DeleteLast() {
	l.items = l.items[:len(l.items)-2]
}

func (l *List) Items() []string {
	return append([]string(nil), l.items...)
}
`

const fixedTodo = `package todo

type List struct {
	items []string
}

func New(items ...string) *List {
	return &List{items: append([]string(nil), items...)}
}

func (l *List) DeleteLast() {
	if l == nil || len(l.items) == 0 {
		return
	}
	l.items = l.items[:len(l.items)-1]
}

func (l *List) Items() []string {
	return append([]string(nil), l.items...)
}
`

const todoTest = `package todo

import "testing"

func TestDeleteLastSingle(t *testing.T) {
	list := New("one")
	list.DeleteLast()
	if got := len(list.Items()); got != 0 {
		t.Fatalf("remaining items = %d, want 0", got)
	}
}
`

func TestV02ReleaseLoop(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, "go.mod"), "module example.com/todo\n\ngo 1.25\n")
	writeFile(t, filepath.Join(workspace, "todo", "todo.go"), buggyTodo)
	writeFile(t, filepath.Join(workspace, "todo", "todo_test.go"), todoTest)
	if err := exec.Command("git", "init", workspace).Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}

	baseline := verify.RunPipeline(t.Context(), workspace, verify.Options{Targets: []string{"todo/todo.go"}})
	if baseline.Status != verify.StatusFail {
		t.Fatalf("bug fixture unexpectedly passed: %+v", baseline)
	}

	writeArgs, _ := json.Marshal(map[string]string{"path": "todo/todo.go", "content": fixedTodo})
	readArgs, _ := json.Marshal(map[string]string{"path": "todo/todo.go"})
	responses := []llm.ChatResponse{
		toolResponse("map", "repo_map", `{}`),
		toolResponse("read", "read_file", string(readArgs)),
		toolResponse("write", "write_file", string(writeArgs)),
		toolResponse("verify", "verify", `{"targets":["todo/todo.go"]}`),
		{Message: llm.Message{Role: "assistant", Content: "Goal complete: deleting the last todo is safe and covered by a regression test."}},
	}
	fake := &llm.Scripted{Responses: responses}
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Mode = config.ModeFast
	cfg.Provider = config.ProviderOllama
	registry := tools.NewRegistry(tools.Context{
		Workspace: workspace,
		VerifyTargets: func(ctx context.Context, targets []string) (string, error) {
			result := verify.RunPipeline(ctx, workspace, verify.Options{Targets: targets})
			return verify.FormatPipeline(result), nil
		},
	})
	a := agent.New(cfg, fake, registry, perm.New(config.ModeFast, workspace, nil))
	a.TaskStore = taskstate.WorkspaceStore(workspace)
	a.SetTaskSession("acceptance-session")

	_, result, err := a.Run(t.Context(), nil, llm.Message{
		Role:    "user",
		Content: "there is a bug where deleting the last todo crashes the app; fix it and make sure it does not happen again",
	}, permissive{})
	if err != nil {
		t.Fatalf("agent loop: %v", err)
	}
	if len(fake.Calls) != len(responses) {
		t.Fatalf("model calls = %d, want %d", len(fake.Calls), len(responses))
	}
	for i, want := range []string{"repo_map", "read_file", "write_file", "verify"} {
		messages := fake.Calls[i+1].Messages
		got := ""
		for j := len(messages) - 1; j >= 0; j-- {
			if messages[j].Name != "" {
				got = messages[j].Name
				break
			}
		}
		if got != want {
			t.Fatalf("call %d last tool = %+v, want %s", i, messages, want)
		}
		if want == "write_file" && (len(messages) == 0 || !strings.Contains(messages[len(messages)-1].Content, "Outcome state: VERIFY")) {
			t.Fatalf("call %d missing post-write outcome guidance: %+v", i, messages)
		}
	}
	if !result.GoalDone || !strings.Contains(result.Text, "Changed: todo/todo.go") || !strings.Contains(result.Text, "Undo: /undo") {
		t.Fatalf("result did not include completion contract: %+v", result)
	}
	if result.Task == nil || result.Task.Status != taskstate.StatusDone || len(result.Task.Verification) == 0 {
		t.Fatalf("durable task not complete: %+v", result.Task)
	}
	verification := result.Task.Verification[len(result.Task.Verification)-1].Summary
	if !strings.Contains(verification, "targeted PASS") || !strings.Contains(verification, "broader PASS") {
		t.Fatalf("missing staged verification evidence: %q", verification)
	}
	loaded, err := a.TaskStore.Load("acceptance-session")
	if err != nil || loaded.Status != taskstate.StatusWorking || !loaded.NeedsVerification() {
		t.Fatalf("directly loaded task should require live proof rebinding = %+v, err=%v", loaded, err)
	}
	assertFile(t, filepath.Join(workspace, "todo", "todo.go"), fixedTodo)

	if _, err := a.UndoLastTurn(); err != nil {
		t.Fatalf("undo: %v", err)
	}
	assertFile(t, filepath.Join(workspace, "todo", "todo.go"), buggyTodo)
	afterUndo := verify.RunPipeline(t.Context(), workspace, verify.Options{Targets: []string{"todo/todo.go"}})
	if afterUndo.Status != verify.StatusFail {
		t.Fatalf("undo did not restore the failing pre-turn state: %+v", afterUndo)
	}
}

type permissive struct{ agent.NopHandler }

func (permissive) OnNeedPermission(context.Context, perm.Request) (perm.Decision, error) {
	return perm.Allow, nil
}

func toolResponse(id, name, args string) llm.ChatResponse {
	return llm.ChatResponse{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: id, Name: name, Arguments: args}}}}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}
