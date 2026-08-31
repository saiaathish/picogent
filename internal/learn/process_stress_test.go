package learn_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/learn"
)

// TestSaveCrossProcessRejectsStaleRevision exercises the cooperative
// process-level lock and persisted revision check. It is not evidence against
// an uncooperative same-UID filesystem writer.
func TestSaveCrossProcessRejectsStaleRevision(t *testing.T) {
	const writers = 8
	root := t.TempDir()
	home := filepath.Join(root, "home")
	t.Setenv("PICOGENT_HOME", home)
	workspace := filepath.Join(root, "project")

	seed := learn.Store{Workspace: workspace}
	seed.RecordRead("seed.go")
	if err := learn.Save(&seed); err != nil {
		t.Fatal(err)
	}
	if seed.Revision != 1 {
		t.Fatalf("initial revision = %d, want 1", seed.Revision)
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
		cmd := exec.Command(os.Args[0], "-test.run", "^TestSaveCrossProcessHelper$", "-test.count=1")
		cmd.Env = append(os.Environ(),
			"PICOGENT_LEARN_HELPER=1",
			"PICOGENT_LEARN_WORKSPACE="+workspace,
			"PICOGENT_LEARN_READY="+children[i].ready,
			"PICOGENT_LEARN_RELEASE="+release,
			"PICOGENT_LEARN_RESULT="+children[i].result,
		)
		cmd.Stdout = &children[i].output
		cmd.Stderr = &children[i].output
		children[i].cmd = cmd
		if err := cmd.Start(); err != nil {
			t.Fatalf("start child %d: %v", i, err)
		}
	}
	for i := range children {
		waitForLearnFile(t, children[i].ready)
	}
	if err := os.WriteFile(release, []byte("release\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for i := range children {
		if err := children[i].cmd.Wait(); err != nil {
			t.Fatalf("child %d failed: %v\n%s", i, err, children[i].output.String())
		}
	}

	successes, conflicts := 0, 0
	for i := range children {
		data, err := os.ReadFile(children[i].result)
		if err != nil {
			t.Fatal(err)
		}
		switch strings.TrimSpace(string(data)) {
		case "success":
			successes++
		case "conflict":
			conflicts++
		default:
			t.Fatalf("child %d result = %q", i, data)
		}
	}
	if successes != 1 || conflicts != writers-1 {
		t.Fatalf("cross-process results = successes %d conflicts %d, want 1/%d", successes, conflicts, writers-1)
	}

	final, err := learn.Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if final.Revision != 2 || final.Turns != 1 || final.FilesRead["seed.go"] != 1 {
		t.Fatalf("final state = revision %d turns %d files %v, want revision 2 turns 1 seed=1", final.Revision, final.Turns, final.FilesRead)
	}
	if _, err := os.Stat(filepath.Join(home, "learn")); err != nil {
		t.Fatalf("learning directory missing: %v", err)
	}
}

func TestSaveCrossProcessHelper(t *testing.T) {
	if os.Getenv("PICOGENT_LEARN_HELPER") != "1" {
		return
	}
	workspace := os.Getenv("PICOGENT_LEARN_WORKSPACE")
	ready := os.Getenv("PICOGENT_LEARN_READY")
	release := os.Getenv("PICOGENT_LEARN_RELEASE")
	result := os.Getenv("PICOGENT_LEARN_RESULT")

	store, err := learn.Load(workspace)
	if err != nil {
		t.Fatal(err)
	}
	store.RecordTurn()
	if err := os.WriteFile(ready, []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitForLearnFile(t, release)

	err = learn.Save(&store)
	status := "success"
	if errors.Is(err, learn.ErrRevisionConflict) {
		status = "conflict"
	} else if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(result, []byte(status+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func waitForLearnFile(t *testing.T, path string) {
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
