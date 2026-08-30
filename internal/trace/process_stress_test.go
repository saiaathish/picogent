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

// TestAppendCrossProcessPreservesEvents exercises cooperative fresh-process
// writers. It verifies serialized sequence allocation and the bounded log;
// it is not evidence against an uncooperative same-UID filesystem writer.
func TestAppendCrossProcessPreservesEvents(t *testing.T) {
	const writers = 8
	root := t.TempDir()
	home := filepath.Join(root, "home")
	t.Setenv("PICOGENT_HOME", home)
	workspace := filepath.Join(root, "project")

	parent, err := trace.Open(workspace)
	if err != nil {
		t.Fatal(err)
	}

	type child struct {
		cmd    *exec.Cmd
		ready  string
		result string
		output bytes.Buffer
	}
	children := make([]child, writers)
	defer func() {
		for i := range children {
			if children[i].cmd != nil && children[i].cmd.Process != nil {
				_ = children[i].cmd.Process.Kill()
				_ = children[i].cmd.Wait()
			}
		}
	}()

	release := filepath.Join(root, "release")
	for i := range children {
		children[i].ready = filepath.Join(root, fmt.Sprintf("ready-%02d", i))
		children[i].result = filepath.Join(root, fmt.Sprintf("result-%02d", i))
		cmd := exec.Command(os.Args[0], "-test.run", "^TestAppendCrossProcessHelper$", "-test.count=1")
		cmd.Env = append(os.Environ(),
			"PICOGENT_TRACE_HELPER=1",
			"PICOGENT_TRACE_WORKSPACE="+workspace,
			"PICOGENT_TRACE_ID="+strconv.Itoa(i),
			"PICOGENT_TRACE_READY="+children[i].ready,
			"PICOGENT_TRACE_RELEASE="+release,
			"PICOGENT_TRACE_RESULT="+children[i].result,
		)
		cmd.Stdout = &children[i].output
		cmd.Stderr = &children[i].output
		children[i].cmd = cmd
		if err := cmd.Start(); err != nil {
			t.Fatalf("start child %d: %v", i, err)
		}
	}
	for i := range children {
		waitForTraceFile(t, children[i].ready)
	}
	if err := os.WriteFile(release, []byte("release\n"), 0o600); err != nil {
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
		if strings.TrimSpace(string(data)) != "success" {
			t.Fatalf("child %d result = %q", i, data)
		}
	}

	events := parent.Tail(writers)
	if len(events) != writers {
		t.Fatalf("events = %d, want %d", len(events), writers)
	}
	seen := make(map[string]bool, writers)
	for i, event := range events {
		if event.Seq != i+1 {
			t.Fatalf("event %d sequence = %d, want %d", i, event.Seq, i+1)
		}
		if seen[event.Detail] {
			t.Fatalf("duplicate event detail %q", event.Detail)
		}
		seen[event.Detail] = true
	}
	info, err := os.Stat(parent.Path())
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > 256<<10 {
		t.Fatalf("trace log grew beyond bound: %d", info.Size())
	}
}

func TestAppendCrossProcessHelper(t *testing.T) {
	if os.Getenv("PICOGENT_TRACE_HELPER") != "1" {
		return
	}
	workspace := os.Getenv("PICOGENT_TRACE_WORKSPACE")
	id := os.Getenv("PICOGENT_TRACE_ID")
	ready := os.Getenv("PICOGENT_TRACE_READY")
	release := os.Getenv("PICOGENT_TRACE_RELEASE")
	result := os.Getenv("PICOGENT_TRACE_RESULT")

	log, err := trace.Open(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ready, []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitForTraceFile(t, release)
	if err := log.Append("cross_process", "helper", "writer-"+id, nil, 0); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(result, []byte("success\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func waitForTraceFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
