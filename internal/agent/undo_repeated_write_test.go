package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/saiaathish/picogent/internal/checkpoint"
	"github.com/saiaathish/picogent/internal/workspace"
)

const (
	repeatedWriteCrashEnv       = "PICOGENT_REPEATED_WRITE_CRASH_HELPER"
	repeatedWriteCrashWorkspace = "PICOGENT_REPEATED_WRITE_CRASH_WORKSPACE"
)

func TestRepeatedWriteCrashUsesLastPublishedState(t *testing.T) {
	if os.Getenv(repeatedWriteCrashEnv) == "1" {
		t.Skip("helper process")
	}

	root := t.TempDir()
	path := filepath.Join(root, "note.txt")
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run", "^TestRepeatedWriteCrashChild$", "-test.count=1")
	cmd.Env = append(os.Environ(),
		repeatedWriteCrashEnv+"=1",
		repeatedWriteCrashWorkspace+"="+root,
	)
	if output, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("child completed instead of crashing before the second rename:\n%s", output)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "first\n" {
		t.Fatalf("workspace after simulated crash = %q, err=%v", got, err)
	}

	undo, err := loadLatestDurableUndo(root, "repeated-write", 1)
	if err != nil || undo == nil {
		t.Fatalf("fresh durable undo = %#v, err=%v", undo, err)
	}
	message, complete, restoreErr := undo.restore()
	if restoreErr != nil || !complete || !strings.Contains(message, "restored note.txt") {
		t.Fatalf("fresh repeated-write recovery = (%q, %v, %v)", message, complete, restoreErr)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "before\n" {
		t.Fatalf("recovered workspace = %q, err=%v", got, err)
	}
	if err := undo.finalizeJournal(); err != nil {
		t.Fatal(err)
	}
}

func TestRepeatedWriteCrashChild(t *testing.T) {
	if os.Getenv(repeatedWriteCrashEnv) != "1" {
		t.Skip("helper process")
	}

	root := os.Getenv(repeatedWriteCrashWorkspace)
	path := filepath.Join(root, "note.txt")
	cp, err := checkpoint.Capture(root, []string{"note.txt"})
	if err != nil {
		t.Fatal(err)
	}
	u := &turnUndo{workspace: root, checkpoint: cp, sessionID: "repeated-write", turnSequence: 1}
	write := func(data []byte) error {
		return workspace.WriteAtomicWithPublishHook(root, path, data, func(mode os.FileMode) error {
			if err := u.preparePublish(path, data, mode); err != nil {
				return err
			}
			if string(data) == "before\n" {
				os.Exit(1)
			}
			return nil
		})
	}
	if err := write([]byte("first\n")); err != nil {
		t.Fatal(err)
	}
	if err := write([]byte("before\n")); err == nil {
		t.Fatal("second write unexpectedly returned")
	}
}
