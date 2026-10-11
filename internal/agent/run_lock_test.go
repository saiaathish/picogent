package agent_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/agent"
	"github.com/saiaathish/picogent/internal/config"
	"github.com/saiaathish/picogent/internal/llm"
	"github.com/saiaathish/picogent/internal/perm"
	"github.com/saiaathish/picogent/internal/taskstate"
	"github.com/saiaathish/picogent/internal/tools"
)

type blockingChatClient struct {
	entered    chan struct{}
	continueCh chan struct{}
	once       sync.Once
}

func (c *blockingChatClient) Chat(ctx context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	close(c.entered)
	select {
	case <-c.continueCh:
		return llm.ChatResponse{Message: llm.Message{Role: "assistant", Content: "done"}}, nil
	case <-ctx.Done():
		return llm.ChatResponse{}, ctx.Err()
	}
}

func (c *blockingChatClient) unblock() {
	c.once.Do(func() { close(c.continueCh) })
}

func TestRunWithOptionsHoldsProjectLockForEntireTurn(t *testing.T) {
	workspace := t.TempDir()
	storeDir := t.TempDir()
	store := taskstate.NewStore(storeDir)
	client := &blockingChatClient{entered: make(chan struct{}), continueCh: make(chan struct{})}
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	a := agent.New(cfg, client, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	a.SetTaskStore(store)

	runDone := make(chan error, 1)
	go func() {
		_, _, err := a.Run(context.Background(), nil, llm.Message{Role: "user", Content: "do the work"}, allowAll{})
		runDone <- err
	}()
	select {
	case <-client.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("agent run did not reach the model")
	}

	secondStore := taskstate.NewStore(storeDir)
	lockAcquired := make(chan func() error, 1)
	lockFailed := make(chan error, 1)
	go func() {
		release, err := secondStore.AcquireRunLock()
		if err != nil {
			lockFailed <- err
			return
		}
		lockAcquired <- release
	}()
	select {
	case err := <-lockFailed:
		t.Fatalf("second project lock failed: %v", err)
	case release := <-lockAcquired:
		_ = release()
		t.Fatal("project lock was released while the model call was still active")
	case <-time.After(100 * time.Millisecond):
	}

	client.unblock()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("agent run did not finish after model release")
	}
	select {
	case err := <-lockFailed:
		t.Fatalf("second project lock failed after run: %v", err)
	case release := <-lockAcquired:
		if err := release(); err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("project lock was not released after the turn")
	}
}

func TestRunsWithDifferentTaskStoresSerializeByWorkspace(t *testing.T) {
	workspace := t.TempDir()
	firstClient := &blockingChatClient{entered: make(chan struct{}), continueCh: make(chan struct{})}
	secondClient := &blockingChatClient{entered: make(chan struct{}), continueCh: make(chan struct{})}
	defer firstClient.unblock()
	defer secondClient.unblock()
	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Provider = config.ProviderOllama
	first := agent.New(cfg, firstClient, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	first.SetTaskStore(taskstate.NewStore(t.TempDir()))
	second := agent.New(cfg, secondClient, tools.NewRegistry(tools.Context{Workspace: workspace}), perm.New(config.ModeFast, workspace, nil))
	second.SetTaskStore(taskstate.NewStore(t.TempDir()))

	firstDone := make(chan error, 1)
	go func() {
		_, _, err := first.Run(context.Background(), nil, llm.Message{Role: "user", Content: "first project turn"}, allowAll{})
		firstDone <- err
	}()
	select {
	case <-firstClient.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first agent run did not reach the model")
	}

	secondDone := make(chan error, 1)
	go func() {
		_, _, err := second.Run(context.Background(), nil, llm.Message{Role: "user", Content: "second project turn"}, allowAll{})
		secondDone <- err
	}()
	select {
	case <-secondClient.entered:
		t.Fatal("agent with a different task store entered the same workspace concurrently")
	case <-time.After(100 * time.Millisecond):
	}

	firstClient.unblock()
	select {
	case err := <-firstDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first agent run did not finish after release")
	}
	select {
	case <-secondClient.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("second agent did not enter after the first workspace run completed")
	}
	secondClient.unblock()
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second agent run did not finish after release")
	}
}
