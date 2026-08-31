package trace_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/trace"
)

const (
	traceSustainedHelperEnv    = "PICOGENT_TRACE_SUSTAINED_HELPER"
	traceSustainedWorkspaceEnv = "PICOGENT_TRACE_SUSTAINED_WORKSPACE"
	traceSustainedWorkerEnv    = "PICOGENT_TRACE_SUSTAINED_WORKER"
	traceSustainedCountEnv     = "PICOGENT_TRACE_SUSTAINED_COUNT"
	traceSustainedReadyEnv     = "PICOGENT_TRACE_SUSTAINED_READY"
	traceSustainedReleaseEnv   = "PICOGENT_TRACE_SUSTAINED_RELEASE"
	traceSustainedResultEnv    = "PICOGENT_TRACE_SUSTAINED_RESULT"
	traceSustainedWorkers      = 4
	traceSustainedEvents       = 256
	traceSustainedWait         = 45 * time.Second
)

// TestTraceSustainedCrossProcessRetention drives enough real fresh-process
// appends to force JSONL retention while writers contend on the kernel-backed
// trace lock. It proves the bounded tail keeps allocating contiguous sequence
// numbers; it is not a filesystem-hostility or performance-budget claim.
func TestTraceSustainedCrossProcessRetention(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PICOGENT_HOME", home)

	parent, err := trace.Open(workspace)
	if err != nil {
		t.Fatal(err)
	}
	release := filepath.Join(root, "release")
	children := make([]*traceSustainedChild, traceSustainedWorkers)
	for worker := range children {
		children[worker] = startTraceSustainedChild(t, workspace, worker, release, root)
	}
	defer func() {
		for _, child := range children {
			stopTraceSustainedChild(child)
		}
	}()

	for _, child := range children {
		waitForTraceSustainedReady(t, child)
	}
	if err := os.WriteFile(release, []byte("release\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for worker, child := range children {
		if err := waitForTraceSustainedChild(child); err != nil {
			t.Fatalf("sustained trace worker %d failed: %v\n%s", worker, err, child.output.String())
		}
		data, err := os.ReadFile(child.result)
		if err != nil {
			t.Fatalf("read sustained trace worker %d result: %v", worker, err)
		}
		if strings.TrimSpace(string(data)) != "success" {
			t.Fatalf("sustained trace worker %d result = %q", worker, data)
		}
	}

	const total = traceSustainedWorkers * traceSustainedEvents
	events := parent.Tail(64)
	if len(events) != 64 {
		t.Fatalf("sustained trace tail length = %d, want 64", len(events))
	}
	for i, event := range events {
		wantSeq := total - len(events) + i + 1
		if event.Seq != wantSeq {
			t.Fatalf("sustained trace event %d sequence = %d, want %d", i, event.Seq, wantSeq)
		}
		if !strings.HasPrefix(event.Detail, "worker=") {
			t.Fatalf("sustained trace event %d detail = %q, want worker marker", i, event.Detail)
		}
	}
	info, err := os.Stat(parent.Path())
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > 256<<10 {
		t.Fatalf("sustained trace log grew beyond bound: %d", info.Size())
	}
}

func TestTraceSustainedHelper(t *testing.T) {
	if os.Getenv(traceSustainedHelperEnv) != "1" {
		return
	}
	workspace := os.Getenv(traceSustainedWorkspaceEnv)
	worker := os.Getenv(traceSustainedWorkerEnv)
	count, err := strconv.Atoi(os.Getenv(traceSustainedCountEnv))
	if err != nil || count <= 0 {
		t.Fatalf("invalid sustained trace count: %q", os.Getenv(traceSustainedCountEnv))
	}
	log, err := trace.Open(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv(traceSustainedReadyEnv), []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitForTraceSustainedFile(t, os.Getenv(traceSustainedReleaseEnv))
	for event := 0; event < count; event++ {
		detail := fmt.Sprintf("worker=%s event=%03d %s", worker, event, strings.Repeat("x", 220))
		if err := log.Append("sustained", "helper", detail, nil, 0); err != nil {
			t.Fatalf("append event %d: %v", event, err)
		}
	}
	if err := os.WriteFile(os.Getenv(traceSustainedResultEnv), []byte("success\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

type traceSustainedChild struct {
	cmd    *exec.Cmd
	ready  string
	result string
	output bytes.Buffer
	err    error
	done   chan struct{}
}

func startTraceSustainedChild(t *testing.T, workspace string, worker int, release, root string) *traceSustainedChild {
	t.Helper()
	child := &traceSustainedChild{
		cmd:    exec.Command(os.Args[0], "-test.run", "^TestTraceSustainedHelper$", "-test.count=1"),
		ready:  filepath.Join(root, fmt.Sprintf("ready-%02d", worker)),
		result: filepath.Join(root, fmt.Sprintf("result-%02d", worker)),
		done:   make(chan struct{}),
	}
	child.cmd.Env = append(os.Environ(),
		traceSustainedHelperEnv+"=1",
		traceSustainedWorkspaceEnv+"="+workspace,
		traceSustainedWorkerEnv+"="+strconv.Itoa(worker),
		traceSustainedCountEnv+"="+strconv.Itoa(traceSustainedEvents),
		traceSustainedReadyEnv+"="+child.ready,
		traceSustainedReleaseEnv+"="+release,
		traceSustainedResultEnv+"="+child.result,
	)
	child.cmd.Stdout = &child.output
	child.cmd.Stderr = &child.output
	if err := child.cmd.Start(); err != nil {
		t.Fatalf("start sustained trace worker %d: %v", worker, err)
	}
	go func() {
		child.err = child.cmd.Wait()
		close(child.done)
	}()
	return child
}

func stopTraceSustainedChild(child *traceSustainedChild) {
	if child == nil || child.done == nil {
		return
	}
	select {
	case <-child.done:
		return
	default:
		if child.cmd != nil && child.cmd.Process != nil {
			_ = child.cmd.Process.Kill()
		}
		<-child.done
	}
}

func waitForTraceSustainedChild(child *traceSustainedChild) error {
	if child == nil || child.done == nil {
		return fmt.Errorf("sustained trace child was not started")
	}
	select {
	case <-child.done:
		return child.err
	case <-time.After(traceSustainedWait):
		if child.cmd != nil && child.cmd.Process != nil {
			_ = child.cmd.Process.Kill()
		}
		<-child.done
		return fmt.Errorf("timed out waiting for sustained trace child")
	}
}

func waitForTraceSustainedReady(t *testing.T, child *traceSustainedChild) {
	t.Helper()
	if child == nil {
		t.Fatal("sustained trace child was not started")
	}
	deadline := time.Now().Add(traceSustainedWait)
	for {
		if _, err := os.Stat(child.ready); err == nil {
			return
		}
		select {
		case <-child.done:
			t.Fatalf("sustained trace child exited before ready: %v\n%s", child.err, child.output.String())
		default:
		}
		if time.Now().After(deadline) {
			stopTraceSustainedChild(child)
			t.Fatalf("timed out waiting for sustained trace readiness\n%s", child.output.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForTraceSustainedFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(traceSustainedWait)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for sustained trace marker %s", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
