//go:build darwin

package workspace_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/workspace"
)

const (
	hostileWorkspaceSwapHelperEnv = "PICOGENT_HOSTILE_WORKSPACE_SWAP_HELPER"
	hostileWorkspaceSwapAttempts  = 200
)

func TestDarwinSameUIDWorkspaceParentSwapConfinement(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only same-UID workspace parent-swap harness")
	}

	root := t.TempDir()
	outside := filepath.Join(root, "outside")
	workspaceRoot := filepath.Join(root, "workspace")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(workspaceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinelPath := filepath.Join(outside, "probe.txt")
	sentinel := []byte("workspace-outside-sentinel\n")
	if err := os.WriteFile(sentinelPath, sentinel, 0o600); err != nil {
		t.Fatal(err)
	}
	before := sha256Hex(sentinel)

	// Nested directory under the workspace that the attacker renames.
	nested := filepath.Join(workspaceRoot, "nested")
	backup := nested + "-real"
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := workspace.WriteAtomic(workspaceRoot, "nested/seed.txt", []byte("seed\n")); err != nil {
		t.Fatal(err)
	}

	ready := filepath.Join(root, "ready")
	release := filepath.Join(root, "release")
	stop := filepath.Join(root, "stop")
	swapsPath := filepath.Join(root, "swaps")

	cmd := exec.Command(os.Args[0], "-test.run", "^TestDarwinSameUIDWorkspaceParentSwapAttackerHelper$", "-test.count=1")
	cmd.Env = append(os.Environ(),
		hostileWorkspaceSwapHelperEnv+"=1",
		"PICOGENT_HOSTILE_PARENT="+nested,
		"PICOGENT_HOSTILE_BACKUP="+backup,
		"PICOGENT_HOSTILE_OUTSIDE="+outside,
		"PICOGENT_HOSTILE_READY="+ready,
		"PICOGENT_HOSTILE_RELEASE="+release,
		"PICOGENT_HOSTILE_STOP="+stop,
		"PICOGENT_HOSTILE_SWAPS="+swapsPath,
	)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waitForWorkspaceFile(t, ready, 15*time.Second)
	if err := os.WriteFile(release, []byte("go\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitForWorkspaceSwaps(t, swapsPath, 1, 15*time.Second)

	writeOK, writeErr, writeEscape := 0, 0, false
	readOK, readErr, readEscape := 0, 0, false
	removeOK, removeErr, removeEscape := 0, 0, false

	for i := 0; i < hostileWorkspaceSwapAttempts; i++ {
		path := fmt.Sprintf("nested/item-%d.txt", i)
		if err := workspace.WriteAtomic(workspaceRoot, path, []byte("inside\n")); err == nil {
			writeOK++
		} else {
			writeErr++
		}
		if mutated, err := sentinelMutated(sentinelPath, before); err != nil {
			t.Fatal(err)
		} else if mutated {
			writeEscape = true
			break
		}

		f, err := workspace.OpenRead(workspaceRoot, "nested/seed.txt")
		if err == nil {
			data, readErr2 := io.ReadAll(f)
			_ = f.Close()
			if readErr2 != nil {
				readErr++
			} else if string(data) == "seed\n" {
				readOK++
			} else if string(data) == string(sentinel) || strings.Contains(string(data), "workspace-outside") {
				readEscape = true
				break
			} else {
				readErr++
			}
		} else {
			readErr++
		}

		_ = workspace.WriteAtomic(workspaceRoot, path, []byte("inside\n"))
		if err := workspace.Remove(workspaceRoot, path); err == nil {
			removeOK++
		} else {
			removeErr++
		}
		if mutated, err := sentinelMutated(sentinelPath, before); err != nil {
			t.Fatal(err)
		} else if mutated {
			removeEscape = true
			break
		}
	}

	if err := os.WriteFile(stop, []byte("stop\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("attacker failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	swaps := readWorkspaceSwapCount(t, swapsPath)
	if swaps < 1 {
		t.Fatalf("no confirmed attacker swaps (%d)", swaps)
	}
	if writeEscape || readEscape || removeEscape {
		t.Fatalf("escape observed write=%v read=%v remove=%v", writeEscape, readEscape, removeEscape)
	}
	after, err := os.ReadFile(sentinelPath)
	if err != nil {
		t.Fatal(err)
	}
	if sha256Hex(after) != before {
		t.Fatalf("outside sentinel mutated")
	}

	// Restore nested parent if needed.
	if info, err := os.Lstat(nested); err == nil && info.Mode()&os.ModeSymlink != 0 {
		_ = os.Remove(nested)
	}
	if _, err := os.Lstat(nested); errors.Is(err, os.ErrNotExist) {
		_ = os.Rename(backup, nested)
	}

	t.Logf("workspace parent-swap swaps=%d write=%d/%d read=%d/%d remove=%d/%d",
		swaps, writeOK, writeErr, readOK, readErr, removeOK, removeErr)
}

func TestDarwinSameUIDWorkspaceParentSwapAttackerHelper(t *testing.T) {
	if os.Getenv(hostileWorkspaceSwapHelperEnv) != "1" {
		return
	}
	parent := os.Getenv("PICOGENT_HOSTILE_PARENT")
	backup := os.Getenv("PICOGENT_HOSTILE_BACKUP")
	outside := os.Getenv("PICOGENT_HOSTILE_OUTSIDE")
	ready := os.Getenv("PICOGENT_HOSTILE_READY")
	release := os.Getenv("PICOGENT_HOSTILE_RELEASE")
	stop := os.Getenv("PICOGENT_HOSTILE_STOP")
	swapsPath := os.Getenv("PICOGENT_HOSTILE_SWAPS")
	if parent == "" || backup == "" || outside == "" || ready == "" || release == "" || stop == "" || swapsPath == "" {
		t.Fatal("workspace attacker helper missing required environment")
	}
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
		time.Sleep(5 * time.Millisecond)
	}
	swaps := 0
	stopDeadline := time.Now().Add(60 * time.Second)
	for {
		if _, err := os.Stat(stop); err == nil {
			break
		}
		if time.Now().After(stopDeadline) {
			break
		}
		if err := os.Rename(parent, backup); err != nil {
			continue
		}
		if err := os.Symlink(outside, parent); err != nil {
			_ = os.Rename(backup, parent)
			continue
		}
		swaps++
		_ = os.WriteFile(swapsPath, []byte(strconv.Itoa(swaps)+"\n"), 0o600)
		_ = os.Remove(parent)
		_ = os.Rename(backup, parent)
	}
	if err := os.WriteFile(swapsPath, []byte(strconv.Itoa(swaps)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func waitForWorkspaceFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", path)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitForWorkspaceSwaps(t *testing.T, path string, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if data, err := os.ReadFile(path); err == nil {
			if n, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && n >= want {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for >=%d attacker swaps in %s", want, path)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func readWorkspaceSwapCount(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func sentinelMutated(path, before string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return true, err
	}
	return sha256Hex(data) != before, nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
