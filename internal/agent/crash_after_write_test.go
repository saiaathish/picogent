package agent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
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

const (
	crashAfterWriteHelperEnv    = "PICOGENT_CRASH_AFTER_WRITE_HELPER"
	crashAfterWriteStoreEnv     = "PICOGENT_CRASH_AFTER_WRITE_STORE"
	crashAfterWriteWorkspaceEnv = "PICOGENT_CRASH_AFTER_WRITE_WORKSPACE"
	crashAfterWriteSessionEnv   = "PICOGENT_CRASH_AFTER_WRITE_SESSION"
	crashAfterWriteReadyEnv     = "PICOGENT_CRASH_AFTER_WRITE_READY"
)

// crashAfterWriteClient releases one real write and then blocks at the next
// provider boundary. The parent can therefore kill the process after the
// active task snapshot has been persisted but before the turn is closed.
type crashAfterWriteClient struct {
	arguments string
	readyPath string
	calls     int
}

func (c *crashAfterWriteClient) Chat(ctx context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	c.calls++
	if c.calls == 1 {
		return llm.ChatResponse{Message: llm.Message{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:        "write-note",
				Name:      "write_file",
				Arguments: c.arguments,
			}},
		}}, nil
	}
	if err := os.WriteFile(c.readyPath, []byte("active turn persisted\n"), 0o600); err != nil {
		return llm.ChatResponse{}, err
	}
	<-ctx.Done()
	return llm.ChatResponse{}, ctx.Err()
}

// TestAgentCrashAfterWriteFreshProcessRecovery is the issue #263 acceptance
// harness. The parent never loads the task until after the child has been
// killed, so all recovery and undo observations come from a fresh process.
func TestAgentCrashAfterWriteFreshProcessRecovery(t *testing.T) {
	if os.Getenv(crashAfterWriteHelperEnv) == "1" {
		t.Skip("helper process")
	}

	root := t.TempDir()
	storeDir := filepath.Join(root, "store")
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	readyPath := filepath.Join(root, "child-ready")
	const sessionID = "crash-after-write"

	child := startCrashAfterWriteChild(t, storeDir, workspace, sessionID, readyPath)
	waitForAgentCrossProcessReady(t, child)

	if err := child.cmd.Process.Kill(); err != nil {
		t.Fatalf("kill child: %v", err)
	}
	select {
	case <-child.done:
	case <-time.After(agentCrossProcessWaitTimeout):
		t.Fatal("killed child did not exit")
	}
	if child.err == nil {
		t.Fatalf("child exited cleanly after the crash boundary\n%s", child.output.String())
	}

	store := taskstate.NewStore(storeDir)
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	fresh := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{{
		Message: llm.Message{Role: "assistant", Content: "Review is pending; more evidence is required."},
	}}}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	fresh.SetTaskStore(store)
	if err := fresh.SetTaskSession(sessionID); err != nil {
		t.Fatalf("attach fresh process to recovered task: %v", err)
	}

	recovered := fresh.TaskSnapshot()
	if recovered == nil {
		t.Fatal("fresh process did not load a task")
	}
	originalGoal := recovered.Goal
	originalDefinition := slices.Clone(recovered.DefinitionOfDone)
	originalIntentRevision := recovered.IntentRevision
	originalTurn := recovered.LastTurn()
	if originalTurn == nil {
		t.Fatal("fresh process did not load the recovered turn")
	}
	if originalTurn.State != taskstate.TurnInterrupted || originalTurn.Route != string(taskstate.TurnRouteRecover) || originalTurn.StopReason != taskstate.StopProcessRestart || originalTurn.EvidenceState != "UNVERIFIED" || strings.TrimSpace(originalTurn.Hypothesis) == "" {
		t.Fatalf("recovered turn metadata = %#v", originalTurn)
	}
	if !slices.Equal(originalTurn.ChangedFiles, []string{"note.txt"}) || originalTurn.ChangedFilesCapped || originalTurn.MutationCount != 1 {
		t.Fatalf("recovered turn side effects = %#v", originalTurn)
	}
	if recovered.Status == taskstate.StatusDone || originalGoal == "" || len(originalDefinition) == 0 {
		t.Fatalf("recovered task completion state = %#v", recovered)
	}
	if recovered.Intent == nil || recovered.Intent.Outcome != originalGoal {
		t.Fatalf("recovered intent = %#v", recovered.Intent)
	}
	assertCrashAfterWriteFile(t, filepath.Join(workspace, "note.txt"), "after\n")

	if fresh.UndoAvailable() {
		t.Fatal("fresh process unexpectedly has an in-memory undo checkpoint")
	}
	beforeUndo, err := os.ReadFile(filepath.Join(workspace, "note.txt"))
	if err != nil {
		t.Fatal(err)
	}
	undoMessage, err := fresh.UndoLastTurn()
	if err != nil || undoMessage != "nothing to undo" {
		t.Fatalf("fresh-process undo = (%q, %v)", undoMessage, err)
	}
	afterUndo, err := os.ReadFile(filepath.Join(workspace, "note.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterUndo, beforeUndo) {
		t.Fatalf("fresh-process undo changed modified file from %q to %q", beforeUndo, afterUndo)
	}

	_, result, err := fresh.Run(context.Background(), nil, llm.Message{Role: "user", Content: "review the note workflow"}, allowAll{})
	if err != nil {
		t.Fatalf("steered follow-up: %v", err)
	}
	afterSteering := fresh.TaskSnapshot()
	if afterSteering == nil {
		t.Fatal("steered follow-up lost durable task")
	}
	if afterSteering.IntentRevision <= originalIntentRevision || afterSteering.Goal != originalGoal || !slices.Equal(afterSteering.DefinitionOfDone, originalDefinition) {
		t.Fatalf("steering changed durable outcome contract unexpectedly = %#v", afterSteering)
	}
	if result.GoalDone || afterSteering.Status == taskstate.StatusDone || strings.Contains(strings.ToLower(result.Text), "goal complete") {
		t.Fatalf("steered follow-up claimed completion: result=%#v task=%#v", result, afterSteering)
	}
	if len(afterSteering.Turns) != len(recovered.Turns)+1 {
		t.Fatalf("steered turn history length = %d, want %d", len(afterSteering.Turns), len(recovered.Turns)+1)
	}
	recoveredAfterSteering := afterSteering.Turns[len(recovered.Turns)-1]
	if !reflect.DeepEqual(&recoveredAfterSteering, originalTurn) {
		t.Fatalf("recovered turn was mutated by steering: before=%#v after=%#v", originalTurn, recoveredAfterSteering)
	}
	newTurn := afterSteering.LastTurn()
	if newTurn == nil || newTurn.Sequence <= originalTurn.Sequence || newTurn.IntentRevision <= originalIntentRevision {
		t.Fatalf("steered turn was not bound to the new intent revision: %#v", newTurn)
	}

	persisted, err := store.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.IntentRevision != afterSteering.IntentRevision || persisted.Status == taskstate.StatusDone || persisted.LastTurn() == nil || persisted.LastTurn().IntentRevision != newTurn.IntentRevision {
		t.Fatalf("persisted steered state = %#v", persisted)
	}
}

func TestAgentCrashAfterWriteChild(t *testing.T) {
	if os.Getenv(crashAfterWriteHelperEnv) != "1" {
		t.Skip("helper process")
	}

	arguments, err := json.Marshal(map[string]string{"path": "note.txt", "content": "after\n"})
	if err != nil {
		t.Fatal(err)
	}
	workspace := os.Getenv(crashAfterWriteWorkspaceEnv)
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, &crashAfterWriteClient{
		arguments: string(arguments),
		readyPath: os.Getenv(crashAfterWriteReadyEnv),
	}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	a.SetTaskStore(taskstate.NewStore(os.Getenv(crashAfterWriteStoreEnv)))
	if err := a.SetTaskSession(os.Getenv(crashAfterWriteSessionEnv)); err != nil {
		t.Fatal(err)
	}
	_, _, err = a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "fix the broken note workflow"}, allowAll{})
	if err == nil {
		t.Fatal("child completed instead of remaining at the crash boundary")
	}
}

func startCrashAfterWriteChild(t *testing.T, storeDir, workspace, sessionID, readyPath string) *agentCrossProcessChild {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run", "^TestAgentCrashAfterWriteChild$", "-test.count=1")
	cmd.Env = append(os.Environ(),
		crashAfterWriteHelperEnv+"=1",
		crashAfterWriteStoreEnv+"="+storeDir,
		crashAfterWriteWorkspaceEnv+"="+workspace,
		crashAfterWriteSessionEnv+"="+sessionID,
		crashAfterWriteReadyEnv+"="+readyPath,
	)
	child := &agentCrossProcessChild{
		cmd:    cmd,
		ready:  readyPath,
		output: bytes.Buffer{},
		done:   make(chan struct{}),
	}
	cmd.Stdout = &child.output
	cmd.Stderr = &child.output
	if err := cmd.Start(); err != nil {
		t.Fatalf("start crash-after-write child: %v", err)
	}
	go func() {
		child.err = cmd.Wait()
		close(child.done)
	}()
	t.Cleanup(func() {
		select {
		case <-child.done:
		default:
			_ = cmd.Process.Kill()
			<-child.done
		}
	})
	return child
}

func assertCrashAfterWriteFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}
