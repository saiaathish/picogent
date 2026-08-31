package session

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/llm"
)

func TestSessionConcurrentCrossProcessUpdates(t *testing.T) {
	// Cooperative process-level coverage for the shared session lock. This
	// proves serialized updates from Picogent writers, not behavior against an
	// uncooperative same-UID filesystem writer.
	const writers = 8
	root := t.TempDir()
	home := filepath.Join(root, "home")
	workspace := filepath.Join(root, "workspace")
	t.Setenv("PICOGENT_HOME", home)

	const id = "cross-process-session"
	initial := &Session{
		ID:        id,
		Workspace: workspace,
		Messages:  []llm.Message{{Role: "user", Content: "initial"}},
	}
	if err := initial.Save(); err != nil {
		t.Fatal(err)
	}

	release := filepath.Join(root, "release")
	type child struct {
		cmd    *exec.Cmd
		ready  string
		stdout bytes.Buffer
		stderr bytes.Buffer
	}
	children := make([]child, writers)
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
		children[i].ready = filepath.Join(root, fmt.Sprintf("ready-%d", i))
		cmd := exec.Command(os.Args[0], "-test.run", "^TestSessionConcurrentCrossProcessHelper$", "-test.count=1")
		cmd.Env = append(os.Environ(),
			"PICOGENT_HOME="+home,
			"PICOGENT_SESSION_HELPER=1",
			"PICOGENT_SESSION_ID="+id,
			"PICOGENT_SESSION_WORKSPACE="+workspace,
			"PICOGENT_SESSION_READY="+children[i].ready,
			"PICOGENT_SESSION_RELEASE="+release,
			"PICOGENT_SESSION_INDEX="+fmt.Sprintf("%d", i),
		)
		cmd.Stdout = &children[i].stdout
		cmd.Stderr = &children[i].stderr
		children[i].cmd = cmd
		if err := cmd.Start(); err != nil {
			t.Fatalf("start child %d: %v", i, err)
		}
	}

	readyPaths := make([]string, 0, writers)
	for i := range children {
		readyPaths = append(readyPaths, children[i].ready)
	}
	waitForSessionFiles(t, readyPaths)
	if err := os.WriteFile(release, []byte("go\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for i := range children {
		if err := children[i].cmd.Wait(); err != nil {
			t.Fatalf("child %d failed: %v\nstdout=%s\nstderr=%s", i, err, children[i].stdout.String(), children[i].stderr.String())
		}
	}

	final, err := Load(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(final.Messages) != writers+1 {
		t.Fatalf("cross-process message count = %d, want %d", len(final.Messages), writers+1)
	}
	seen := make(map[string]bool, len(final.Messages))
	for _, message := range final.Messages {
		seen[message.Content] = true
	}
	if !seen["initial"] {
		t.Fatal("cross-process update lost initial message")
	}
	for i := 0; i < writers; i++ {
		if !seen[fmt.Sprintf("child-%d", i)] {
			t.Fatalf("cross-process update lost child-%d: %#v", i, final.Messages)
		}
	}
}

func TestSessionConcurrentCrossProcessHelper(t *testing.T) {
	if os.Getenv("PICOGENT_SESSION_HELPER") != "1" {
		return
	}
	ready := os.Getenv("PICOGENT_SESSION_READY")
	release := os.Getenv("PICOGENT_SESSION_RELEASE")
	if err := os.WriteFile(ready, []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(release); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for parent release")
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, err := updateSession(os.Getenv("PICOGENT_SESSION_ID"), os.Getenv("PICOGENT_SESSION_WORKSPACE"), false, func(s *Session) error {
		s.Messages = append(s.Messages, llm.Message{
			Role:    "user",
			Content: "child-" + os.Getenv("PICOGENT_SESSION_INDEX"),
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func waitForSessionFiles(t *testing.T, paths []string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		missing := false
		for _, path := range paths {
			if _, err := os.Stat(path); err != nil {
				missing = true
				break
			}
		}
		if !missing {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for session children: %s", strings.Join(paths, ", "))
		}
		time.Sleep(10 * time.Millisecond)
	}
}
