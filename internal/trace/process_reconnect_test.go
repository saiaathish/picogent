package trace

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	traceReconnectModeEnv    = "PICOGENT_TRACE_RECONNECT_MODE"
	traceReconnectLockEnv    = "PICOGENT_TRACE_RECONNECT_LOCK"
	traceReconnectWorkspace  = "PICOGENT_TRACE_RECONNECT_WORKSPACE"
	traceReconnectReadyEnv   = "PICOGENT_TRACE_RECONNECT_READY"
	traceReconnectResultEnv  = "PICOGENT_TRACE_RECONNECT_RESULT"
	traceReconnectWaitPeriod = 15 * time.Second
)

type traceReconnectChild struct {
	cmd    *exec.Cmd
	output bytes.Buffer
	done   chan struct{}
	err    error
}

// TestWriterRecoversAfterKilledLockHolder proves that a fresh Picogent
// process can append after the previous process dies while holding the trace
// lock. The test uses the real kernel-backed lock and a marker handshake; it
// does not claim safety against an uncooperative filesystem writer.
func TestWriterRecoversAfterKilledLockHolder(t *testing.T) {
	if runtime.GOOS == "plan9" || runtime.GOOS == "wasip1" {
		t.Skip("trace locking has no supported process primitive on this platform")
	}
	home := t.TempDir()
	workspace := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PICOGENT_HOME", home)

	log, err := Open(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if err := log.Append("before", "test", "before", nil, 0); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	ready := filepath.Join(root, "holder-ready")
	holder := startTraceReconnectChild(t, []string{
		traceReconnectModeEnv + "=holder",
		traceReconnectLockEnv + "=" + log.Path(),
		traceReconnectReadyEnv + "=" + ready,
	})
	defer stopTraceReconnectChild(holder)
	waitForTraceReconnectReady(t, holder, ready)

	if err := holder.cmd.Process.Kill(); err != nil {
		t.Fatalf("kill trace lock holder: %v (child exit: %v)\n%s", err, holder.err, holder.output.String())
	}
	if err := waitForTraceReconnectExit(holder); err == nil {
		t.Fatal("lock holder exited cleanly after it was killed")
	}

	result := filepath.Join(root, "writer-result")
	writer := startTraceReconnectChild(t, []string{
		traceReconnectModeEnv + "=writer",
		traceReconnectWorkspace + "=" + workspace,
		traceReconnectResultEnv + "=" + result,
	})
	defer stopTraceReconnectChild(writer)
	if err := waitForTraceReconnectExit(writer); err != nil {
		t.Fatalf("fresh writer failed after lock-holder death: %v\n%s", err, writer.output.String())
	}
	if data, err := os.ReadFile(result); err != nil || strings.TrimSpace(string(data)) != "success" {
		t.Fatalf("fresh writer result = %q, err=%v", data, err)
	}

	events := log.Tail(10)
	if len(events) != 2 {
		t.Fatalf("trace events = %d, want 2: %#v", len(events), events)
	}
	if events[0].Seq != 1 || events[0].Detail != "before" || events[1].Seq != 2 || events[1].Detail != "after" {
		t.Fatalf("trace events = %#v, want contiguous before/after sequence", events)
	}
}

func TestTraceReconnectHelper(t *testing.T) {
	switch os.Getenv(traceReconnectModeEnv) {
	case "holder":
		unlock, err := acquireTraceLock(os.Getenv(traceReconnectLockEnv))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(os.Getenv(traceReconnectReadyEnv), []byte("ready\n"), 0o600); err != nil {
			_ = unlock()
			t.Fatal(err)
		}
		for {
			time.Sleep(time.Hour)
		}
	case "writer":
		log, err := Open(os.Getenv(traceReconnectWorkspace))
		if err != nil {
			t.Fatal(err)
		}
		if err := log.Append("after", "test", "after", nil, 0); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(os.Getenv(traceReconnectResultEnv), []byte("success\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func startTraceReconnectChild(t *testing.T, values []string) *traceReconnectChild {
	t.Helper()
	child := &traceReconnectChild{
		cmd:  exec.Command(os.Args[0], "-test.run", "^TestTraceReconnectHelper$", "-test.count=1"),
		done: make(chan struct{}),
	}
	child.cmd.Env = append(os.Environ(), values...)
	child.cmd.Stdout = &child.output
	child.cmd.Stderr = &child.output
	if err := child.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		child.err = child.cmd.Wait()
		close(child.done)
	}()
	return child
}

func stopTraceReconnectChild(child *traceReconnectChild) {
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

func waitForTraceReconnectReady(t *testing.T, child *traceReconnectChild, path string) {
	t.Helper()
	deadline := time.Now().Add(traceReconnectWaitPeriod)
	for {
		if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
			return
		}
		select {
		case <-child.done:
			t.Fatalf("trace lock holder exited before ready: %v\n%s", child.err, child.output.String())
		default:
		}
		if time.Now().After(deadline) {
			stopTraceReconnectChild(child)
			t.Fatalf("timed out waiting for trace lock holder readiness\n%s", child.output.String())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitForTraceReconnectExit(child *traceReconnectChild) error {
	if child == nil || child.done == nil {
		return fmt.Errorf("child was not started")
	}
	select {
	case <-child.done:
		return child.err
	case <-time.After(traceReconnectWaitPeriod):
		if child.cmd != nil && child.cmd.Process != nil {
			_ = child.cmd.Process.Kill()
		}
		<-child.done
		return fmt.Errorf("timed out waiting for child exit")
	}
}
