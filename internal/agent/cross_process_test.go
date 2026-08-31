package agent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	agentCrossProcessHelperEnv    = "PICOGENT_AGENT_CROSS_PROCESS_HELPER"
	agentCrossProcessStoreEnv     = "PICOGENT_AGENT_CROSS_PROCESS_STORE"
	agentCrossProcessWorkspaceEnv = "PICOGENT_AGENT_CROSS_PROCESS_WORKSPACE"
	agentCrossProcessSessionEnv   = "PICOGENT_AGENT_CROSS_PROCESS_SESSION"
	agentCrossProcessReadyEnv     = "PICOGENT_AGENT_CROSS_PROCESS_READY"
	agentCrossProcessReleaseEnv   = "PICOGENT_AGENT_CROSS_PROCESS_RELEASE"
	agentCrossProcessResultEnv    = "PICOGENT_AGENT_CROSS_PROCESS_RESULT"
	agentCrossProcessWaitTimeout  = 90 * time.Second
)

type agentCrossProcessChild struct {
	cmd    *exec.Cmd
	ready  string
	result string
	output bytes.Buffer
}

type agentCrossProcessResult struct {
	ID       string           `json:"id"`
	Status   taskstate.Status `json:"status"`
	Revision uint64           `json:"revision"`
	Attempts int              `json:"attempts"`
	Turns    int              `json:"turns"`
	Error    string           `json:"error,omitempty"`
}

// TestAgentCrossProcessMutationsPreserveProgress releases several fresh
// processes after they have all loaded the same revision. The project run
// lock serializes admitted turns, while the Agent CAS retry must rebase each
// stale in-memory snapshot instead of dropping earlier attempts and turns.
func TestAgentCrossProcessMutationsPreserveProgress(t *testing.T) {
	const writers = 4
	root := t.TempDir()
	storeDir := filepath.Join(root, "store")
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}

	store := taskstate.NewStore(storeDir)
	const sessionID = "agent-cross-process"
	initial, err := taskstate.New(sessionID, "record concurrent progress", []string{"work"})
	if err != nil {
		t.Fatal(err)
	}
	if err := initial.SetStatus(taskstate.StatusWorking); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(initial); err != nil {
		t.Fatal(err)
	}

	releasePath := filepath.Join(root, "release")
	children := make([]agentCrossProcessChild, writers)
	defer func() {
		for i := range children {
			if children[i].cmd == nil || children[i].cmd.Process == nil || children[i].cmd.ProcessState != nil {
				continue
			}
			_ = children[i].cmd.Process.Kill()
			_ = children[i].cmd.Wait()
		}
	}()

	for i := range children {
		children[i].ready = filepath.Join(root, fmt.Sprintf("ready-%02d", i))
		children[i].result = filepath.Join(root, fmt.Sprintf("result-%02d", i))
		cmd := exec.Command(os.Args[0], "-test.run", "^TestAgentCrossProcessMutationHelper$", "-test.count=1")
		cmd.Env = append(os.Environ(),
			agentCrossProcessHelperEnv+"=1",
			agentCrossProcessStoreEnv+"="+storeDir,
			agentCrossProcessWorkspaceEnv+"="+workspace,
			agentCrossProcessSessionEnv+"="+sessionID,
			agentCrossProcessReadyEnv+"="+children[i].ready,
			agentCrossProcessReleaseEnv+"="+releasePath,
			agentCrossProcessResultEnv+"="+children[i].result,
		)
		cmd.Stdout = &children[i].output
		cmd.Stderr = &children[i].output
		children[i].cmd = cmd
		if err := cmd.Start(); err != nil {
			t.Fatalf("start child %d: %v", i, err)
		}
	}

	for i := range children {
		waitForAgentCrossProcessFile(t, children[i].ready)
	}
	if err := os.WriteFile(releasePath, []byte("go\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for i := range children {
		if err := children[i].cmd.Wait(); err != nil {
			t.Fatalf("child %d failed: %v\n%s", i, err, children[i].output.String())
		}
	}

	for i := range children {
		data, err := os.ReadFile(children[i].result)
		if err != nil {
			t.Fatal(err)
		}
		var result agentCrossProcessResult
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatalf("child %d result %q: %v", i, data, err)
		}
		if result.Error != "" {
			t.Fatalf("child %d run error: %s", i, result.Error)
		}
		if result.ID != initial.ID || result.Status != taskstate.StatusBlocked || result.Attempts < 1 || result.Turns < 1 || result.Revision <= initial.Revision {
			t.Fatalf("child %d result = %#v, want rebased blocked progress", i, result)
		}
	}

	final, err := store.Load(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if final.ID != initial.ID || final.Status != taskstate.StatusBlocked || final.Attempts != writers || len(final.Turns) != writers || final.Revision <= initial.Revision {
		t.Fatalf("final task = %#v, want one preserved blocked turn per writer", final)
	}
}

func TestAgentCrossProcessMutationHelper(t *testing.T) {
	if os.Getenv(agentCrossProcessHelperEnv) != "1" {
		return
	}

	store := taskstate.NewStore(os.Getenv(agentCrossProcessStoreEnv))
	workspace := os.Getenv(agentCrossProcessWorkspaceEnv)
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, &llm.Scripted{Responses: []llm.ChatResponse{{Message: llm.Message{
		Role:    "assistant",
		Content: "blocked: cooperative cross-process writer checkpoint",
	}}}}, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	a.SetTaskStore(store)
	if err := a.SetTaskSession(os.Getenv(agentCrossProcessSessionEnv)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv(agentCrossProcessReadyEnv), []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitForAgentCrossProcessFile(t, os.Getenv(agentCrossProcessReleaseEnv))

	_, result, runErr := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "record concurrent progress"}, allowAll{})
	got := agentCrossProcessResult{}
	if task := a.TaskSnapshot(); task != nil {
		got = agentCrossProcessResult{
			ID:       task.ID,
			Status:   task.Status,
			Revision: task.Revision,
			Attempts: task.Attempts,
			Turns:    len(task.Turns),
		}
	}
	if runErr != nil {
		got.Error = runErr.Error()
	}
	if result.Task == nil && got.Error == "" {
		got.Error = "run returned no task snapshot"
	}
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv(agentCrossProcessResultEnv), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if got.Error != "" {
		t.Fatal(got.Error)
	}
}

func waitForAgentCrossProcessFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(agentCrossProcessWaitTimeout)
	for {
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			return
		}
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", path)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
