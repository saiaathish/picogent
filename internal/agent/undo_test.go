package agent_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/tools"
)

func TestUndoRestoresExistingFileAndNormalizesFooter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("before"), 0o640); err != nil {
		t.Fatal(err)
	}
	a := undoTestAgent(t, dir, []llm.ChatResponse{
		toolResponse("1", "edit_file", map[string]string{"path": "note.txt", "old_string": "before", "new_string": "after"}),
		{Message: llm.Message{Role: "assistant", Content: "done\n\nChanged: note.txt\nRun: cat note.txt\nUndo: git checkout -- note.txt"}},
	}, nil)

	_, res, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update note.txt"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.UndoAvailable || res.UndoError != "" {
		t.Fatalf("undo state = available:%v error:%q", res.UndoAvailable, res.UndoError)
	}
	if !strings.Contains(res.Text, "Undo: /undo") || strings.Contains(res.Text, "git checkout") {
		t.Fatalf("non-canonical undo footer: %q", res.Text)
	}
	msg, err := a.UndoLastTurn()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "restored note.txt") {
		t.Fatalf("undo message = %q", msg)
	}
	assertFileContent(t, path, "before")
	msg, err = a.UndoLastTurn()
	if err != nil || msg != "nothing to undo" {
		t.Fatalf("second undo = (%q, %v)", msg, err)
	}
}

func TestUndoRemovesNewFile(t *testing.T) {
	dir := t.TempDir()
	a := undoTestAgent(t, dir, []llm.ChatResponse{
		toolResponse("1", "write_file", map[string]string{"path": "new.txt", "content": "new"}),
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}, nil)

	_, _, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "create new.txt"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	msg, err := a.UndoLastTurn()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(msg, "removed new.txt") {
		t.Fatalf("undo message = %q", msg)
	}
	if _, err := os.Stat(filepath.Join(dir, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("new file still exists: %v", err)
	}
}

func TestUndoRestoresMultiplePathsInReverseCaptureOrder(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "existing.txt")
	if err := os.WriteFile(existing, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	firstArgs, _ := json.Marshal(map[string]string{"path": "existing.txt", "content": "after"})
	secondArgs, _ := json.Marshal(map[string]string{"path": "created.txt", "content": "created"})
	a := undoTestAgent(t, dir, []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "1", Name: "write_file", Arguments: string(firstArgs)},
			{ID: "2", Name: "write_file", Arguments: string(secondArgs)},
		}}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}, nil)

	_, res, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "update both files"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.FilesChanged) != 2 {
		t.Fatalf("files changed = %#v", res.FilesChanged)
	}
	if _, err := a.UndoLastTurn(); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, existing, "before")
	if _, err := os.Stat(filepath.Join(dir, "created.txt")); !os.IsNotExist(err) {
		t.Fatalf("created file still exists: %v", err)
	}
}

func TestUndoConflictLeavesEveryTurnPathUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	other := filepath.Join(dir, "other.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("other before"), 0o644); err != nil {
		t.Fatal(err)
	}
	noteArgs, _ := json.Marshal(map[string]string{"path": "note.txt", "content": "agent edit"})
	otherArgs, _ := json.Marshal(map[string]string{"path": "other.txt", "content": "other agent edit"})
	a := undoTestAgent(t, dir, []llm.ChatResponse{
		{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "1", Name: "write_file", Arguments: string(noteArgs)},
			{ID: "2", Name: "write_file", Arguments: string(otherArgs)},
		}}},
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}, nil)
	if _, _, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "edit note"}, allowAll{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("newer user edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.UndoLastTurn(); err == nil || !strings.Contains(err.Error(), "newer changes") {
		t.Fatalf("expected conflict, got %v", err)
	}
	assertFileContent(t, path, "newer user edit")
	assertFileContent(t, other, "other agent edit")
}

func TestFailedWriteDoesNotReplacePriorUndo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := undoTestAgent(t, dir, []llm.ChatResponse{
		toolResponse("1", "write_file", map[string]string{"path": "note.txt", "content": "after"}),
		{Message: llm.Message{Role: "assistant", Content: "done"}},
		toolResponse("2", "edit_file", map[string]string{"path": "note.txt", "old_string": "missing", "new_string": "unused"}),
		{Message: llm.Message{Role: "assistant", Content: "could not write"}},
	}, nil)
	if _, _, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "edit note"}, allowAll{}); err != nil {
		t.Fatal(err)
	}
	_, failed, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "make a missing replacement"}, allowAll{})
	if err != nil {
		t.Fatal(err)
	}
	if failed.UndoAvailable || len(failed.FilesChanged) != 0 || strings.Contains(failed.Text, "Changed:") {
		t.Fatalf("failed write created false undo state: %+v", failed)
	}
	if _, err := a.UndoLastTurn(); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, path, "before")
}

func TestModelCancellationAfterWriteStillSealsUndo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]string{"path": "note.txt", "content": "after"})
	client := &cancelAfterWriteClient{args: string(args)}
	cfg := config.Default()
	cfg.Workspace = dir
	cfg.Mode = config.ModeFast
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, client, tools.NewRegistry(tools.Context{Workspace: dir}), perm.New(config.ModeFast, dir, nil))
	_, res, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "edit note"}, allowAll{})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "context canceled") {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if !res.UndoAvailable {
		t.Fatalf("canceled turn did not seal undo: %+v", res)
	}
	if _, err := a.UndoLastTurn(); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, path, "before")
}

func TestStreamedStaleFooterIsReplacedWithCanonicalUndo(t *testing.T) {
	dir := t.TempDir()
	args, _ := json.Marshal(map[string]string{"path": "note.txt", "content": "after"})
	stale := "done\n\nChanged: note.txt\nRun: cat note.txt\nUndo: git checkout -- note.txt"
	client := &streamingFooterClient{args: string(args), final: stale}
	cfg := config.Default()
	cfg.Workspace = dir
	cfg.Mode = config.ModeFast
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, client, tools.NewRegistry(tools.Context{Workspace: dir}), perm.New(config.ModeFast, dir, nil))
	h := &finalTextCapture{}

	_, res, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "write note"}, h)
	if err != nil {
		t.Fatal(err)
	}
	if h.visible != res.Text {
		t.Fatalf("visible streamed text = %q, final result = %q", h.visible, res.Text)
	}
	if !strings.Contains(h.visible, "Undo: /undo") || strings.Contains(h.visible, "git checkout") {
		t.Fatalf("streamed footer was not reconciled: %q", h.visible)
	}
}

func TestSuppressUndoDoesNotAdvertiseProcessLocalCommand(t *testing.T) {
	dir := t.TempDir()
	args, _ := json.Marshal(map[string]string{"path": "note.txt", "content": "after"})
	a := undoTestAgent(t, dir, []llm.ChatResponse{
		toolResponse("1", "write_file", json.RawMessage(args)),
		{Message: llm.Message{Role: "assistant", Content: "done"}},
	}, nil)
	h := &textCapture{}
	_, res, err := a.RunWithOptions(context.Background(), nil, llm.Message{Role: "user", Content: "write note"}, h, agent.RunOptions{SuppressUndo: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.UndoAvailable {
		t.Fatalf("process-local checkpoint was lost: %+v", res)
	}
	if strings.Contains(res.Text, "Undo: /undo") || strings.Contains(h.visible, "Undo: /undo") {
		t.Fatalf("headless-style output advertised process-local undo: %q", res.Text)
	}
	if !strings.Contains(res.Text, "Undo: unavailable") {
		t.Fatalf("output did not explain unavailable headless undo: %q", res.Text)
	}
}

type cancelAfterWriteClient struct {
	args  string
	calls int
}

type streamingFooterClient struct {
	args  string
	final string
	calls int
}

func (c *streamingFooterClient) Chat(_ context.Context, req llm.ChatRequest) (llm.ChatResponse, error) {
	c.calls++
	if c.calls == 1 {
		return toolResponse("1", "write_file", json.RawMessage(c.args)), nil
	}
	if req.OnDelta != nil {
		req.OnDelta(c.final)
	}
	return llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: c.final}}, nil
}

type finalTextCapture struct {
	agent.NopHandler
	visible string
}

func (h *finalTextCapture) OnTextDelta(delta string) { h.visible += delta }
func (h *finalTextCapture) OnTextFinal(text string)  { h.visible = text }

type textCapture struct {
	agent.NopHandler
	visible string
}

func (h *textCapture) OnText(text string) { h.visible = text }

func (c *cancelAfterWriteClient) Chat(context.Context, llm.ChatRequest) (llm.ChatResponse, error) {
	c.calls++
	if c.calls == 1 {
		return toolResponse("1", "write_file", json.RawMessage(c.args)), nil
	}
	return llm.ChatResponse{}, context.Canceled
}

func undoTestAgent(t *testing.T, dir string, responses []llm.ChatResponse, verify func(context.Context) (string, error)) *agent.Agent {
	t.Helper()
	cfg := config.Default()
	cfg.Workspace = dir
	cfg.Mode = config.ModeFast
	cfg.Provider = config.ProviderOllama
	reg := tools.NewRegistry(tools.Context{Workspace: dir, Verify: verify})
	return agent.New(cfg, &llm.Scripted{Responses: responses}, reg, perm.New(config.ModeFast, dir, nil))
}

func toolResponse(id, name string, args any) llm.ChatResponse {
	var raw []byte
	switch value := args.(type) {
	case json.RawMessage:
		raw = value
	default:
		raw, _ = json.Marshal(value)
	}
	return llm.ChatResponse{Message: llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: id, Name: name, Arguments: string(raw)}}}}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}
