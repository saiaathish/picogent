//go:build linux

package workspace_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/saiaathish/picogent/internal/workspace"
)

const (
	hostileWorkspaceSwapHelperEnv = "PICOGENT_HOSTILE_WORKSPACE_SWAP_HELPER"
	hostileWorkspaceSwapAttempts  = 200
	hostileWorkspaceEvidenceEnv   = "PICOGENT_HOSTILE_PARENT_SWAP_EVIDENCE_OUT"
	hostileWorkspaceSourceEnv     = "PICOGENT_HOSTILE_PARENT_SWAP_SOURCE_SHA"
)

type hostileWorkspaceSwapEvidence struct {
	Schema                string `json:"schema"`
	CandidateSHA          string `json:"candidate_sha"`
	OS                    string `json:"os"`
	Architecture          string `json:"architecture"`
	Environment           string `json:"environment"`
	Package               string `json:"package"`
	AttackerConfirmed     bool   `json:"attacker_confirmed"`
	Attempts              int    `json:"attempts"`
	AttackerSwaps         int    `json:"attacker_swaps"`
	WriteSuccesses        int    `json:"write_successes"`
	WriteErrors           int    `json:"write_errors"`
	ReadSuccesses         int    `json:"read_successes"`
	ReadErrors            int    `json:"read_errors"`
	RemoveSuccesses       int    `json:"remove_successes"`
	RemoveErrors          int    `json:"remove_errors"`
	OutsideSentinelBefore string `json:"outside_sentinel_before_sha256"`
	OutsideSentinelAfter  string `json:"outside_sentinel_after_sha256"`
	OutsideTreeBefore     string `json:"outside_tree_before_sha256"`
	OutsideTreeAfter      string `json:"outside_tree_after_sha256"`
	OutsideEscapeObserved bool   `json:"outside_escape_observed"`
	Verdict               string `json:"verdict"`
	ObservedAt            string `json:"observed_at"`
	BroadTOCTOUClaim      string `json:"broad_toctou_claim"`
	SourceTreeModified    bool   `json:"source_tree_modified"`
}

func TestLinuxSameUIDWorkspaceParentSwapConfinement(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only same-UID workspace parent-swap harness")
	}
	candidateSHA, sourceTreeModified := currentSourceState(t)
	if sourceTreeModified {
		t.Fatalf("source tree must be clean before retaining hostile evidence")
	}
	if expected := strings.TrimSpace(os.Getenv(hostileWorkspaceSourceEnv)); expected != "" && expected != candidateSHA {
		t.Fatalf("source SHA mismatch: expected %s, current HEAD is %s", expected, candidateSHA)
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
	if err := os.WriteFile(filepath.Join(outside, "seed.txt"), []byte("workspace-outside-marker\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "write-target.txt"), []byte("outside-write\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Nested directory under the workspace that the attacker renames.
	nested := filepath.Join(workspaceRoot, "nested")
	backup := nested + "-real"
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := workspace.WriteAtomic(workspaceRoot, "nested/seed.txt", []byte("seed\n")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < hostileWorkspaceSwapAttempts; i++ {
		if err := os.WriteFile(filepath.Join(nested, fmt.Sprintf("remove-%d.txt", i)), []byte("inside-remove\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(outside, fmt.Sprintf("remove-%d.txt", i)), []byte("outside-remove\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	beforeTree, err := outsideTreeSHA256(outside)
	if err != nil {
		t.Fatal(err)
	}

	ready := filepath.Join(root, "ready")
	release := filepath.Join(root, "release")
	stop := filepath.Join(root, "stop")
	swapsPath := filepath.Join(root, "swaps")

	cmd := exec.Command(os.Args[0], "-test.run", "^TestLinuxSameUIDWorkspaceParentSwapAttackerHelper$", "-test.count=1")
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
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()
	waited := false
	var waitErr error
	waitForAttackerExit := func() error {
		if waited {
			return waitErr
		}
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		select {
		case waitErr = <-waitCh:
			waited = true
		case <-timer.C:
			_ = cmd.Process.Kill()
			waitErr = <-waitCh
			waitErr = fmt.Errorf("attacker did not stop within 5s: %w", waitErr)
			waited = true
		}
		return waitErr
	}
	restoreNested := func() {
		if info, err := os.Lstat(nested); err == nil && info.Mode()&os.ModeSymlink != 0 {
			_ = os.Remove(nested)
		}
		if _, err := os.Lstat(nested); errors.Is(err, os.ErrNotExist) {
			_ = os.Rename(backup, nested)
		}
	}
	cleanup := func() error {
		_ = os.WriteFile(release, []byte("go\n"), 0o600)
		_ = os.WriteFile(stop, []byte("stop\n"), 0o600)
		err := waitForAttackerExit()
		restoreNested()
		return err
	}
	cleaned := false
	defer func() {
		if !cleaned {
			_ = cleanup()
		}
	}()
	waitForWorkspaceFile(t, ready, 15*time.Second)
	if err := os.WriteFile(release, []byte("go\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitForWorkspaceSwaps(t, swapsPath, 1, 15*time.Second)

	writeOK, writeErr, writeEscape := 0, 0, false
	readOK, readErr, readEscape := 0, 0, false
	removeOK, removeErr, removeEscape := 0, 0, false
	removedPaths := make([]string, 0, hostileWorkspaceSwapAttempts)

	for i := 0; i < hostileWorkspaceSwapAttempts; i++ {
		if err := workspace.WriteAtomic(workspaceRoot, "nested/write-target.txt", []byte("inside-write\n")); err == nil {
			writeOK++
		} else {
			writeErr++
		}
		if mutated, err := outsideTreeMutated(outside, beforeTree); err != nil {
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

		path := fmt.Sprintf("nested/remove-%d.txt", i)
		if err := workspace.Remove(workspaceRoot, path); err == nil {
			removeOK++
			removedPaths = append(removedPaths, path)
		} else {
			removeErr++
		}
		if mutated, err := outsideTreeMutated(outside, beforeTree); err != nil {
			t.Fatal(err)
		} else if mutated {
			removeEscape = true
			break
		}
	}

	if err := cleanup(); err != nil {
		t.Fatalf("attacker failed: %v\nstdout=%s\nstderr=%s", err, stdout.String(), stderr.String())
	}
	cleaned = true
	swaps := readWorkspaceSwapCount(t, swapsPath)
	if swaps < 1 {
		t.Fatalf("no confirmed attacker swaps (%d)", swaps)
	}
	after, err := os.ReadFile(sentinelPath)
	if err != nil {
		t.Fatal(err)
	}
	afterSentinelDigest := sha256Hex(after)
	anyEscape := writeEscape || readEscape || removeEscape
	if afterSentinelDigest != before {
		anyEscape = true
	}
	afterTree, err := outsideTreeSHA256(outside)
	if err != nil {
		t.Fatal(err)
	}
	if afterTree != beforeTree {
		anyEscape = true
	}

	if writeOK == 0 || readOK == 0 || removeOK == 0 {
		t.Fatalf("completed with no successful in-tree operations: write=%d read=%d remove=%d", writeOK, readOK, removeOK)
	}
	data, err := os.ReadFile(filepath.Join(nested, "write-target.txt"))
	if err != nil {
		t.Fatalf("read trusted workspace write result: %v", err)
	}
	if string(data) != "inside-write\n" {
		t.Fatalf("trusted workspace write result has unexpected content %q", data)
	}
	for _, path := range removedPaths {
		if _, err := os.Lstat(filepath.Join(workspaceRoot, path)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("successful remove left trusted path %s in place: %v", path, err)
		}
	}

	verdict := "PASS"
	if anyEscape {
		verdict = "FAIL"
	}
	evidence := hostileWorkspaceSwapEvidence{
		Schema:                "picogent.v4.hostile-parent-swap-workspace-evidence.v1",
		CandidateSHA:          candidateSHA,
		OS:                    runtime.GOOS,
		Architecture:          runtime.GOARCH,
		Environment:           "task-owned-disposable",
		Package:               "workspace",
		AttackerConfirmed:     swaps > 0,
		Attempts:              hostileWorkspaceSwapAttempts,
		AttackerSwaps:         swaps,
		WriteSuccesses:        writeOK,
		WriteErrors:           writeErr,
		ReadSuccesses:         readOK,
		ReadErrors:            readErr,
		RemoveSuccesses:       removeOK,
		RemoveErrors:          removeErr,
		OutsideSentinelBefore: before,
		OutsideSentinelAfter:  afterSentinelDigest,
		OutsideTreeBefore:     beforeTree,
		OutsideTreeAfter:      afterTree,
		OutsideEscapeObserved: anyEscape,
		Verdict:               verdict,
		ObservedAt:            time.Now().UTC().Format(time.RFC3339),
		BroadTOCTOUClaim:      "UNVERIFIED",
		SourceTreeModified:    sourceTreeModified,
	}
	payload, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	payload = append(payload, '\n')
	evidencePath := filepath.Join(root, "hostile-parent-swap-workspace-evidence.json")
	if err := os.WriteFile(evidencePath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if dest := strings.TrimSpace(os.Getenv(hostileWorkspaceEvidenceEnv)); dest != "" {
		if err := os.WriteFile(dest, payload, 0o600); err != nil {
			t.Fatalf("retain evidence: %v", err)
		}
	}
	t.Logf("workspace parent-swap evidence verdict=%s path=%s swaps=%d write=%d/%d read=%d/%d remove=%d/%d",
		verdict, evidencePath, swaps, writeOK, writeErr, readOK, readErr, removeOK, removeErr)
	if anyEscape {
		t.Fatalf("escape observed write=%v read=%v remove=%v sentinel=%v tree=%v",
			writeEscape, readEscape, removeEscape, afterSentinelDigest != before, afterTree != beforeTree)
	}
}

func TestLinuxSameUIDWorkspaceParentSwapAttackerHelper(t *testing.T) {
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

func outsideTreeMutated(path, before string) (bool, error) {
	after, err := outsideTreeSHA256(path)
	if err != nil {
		return true, err
	}
	return after != before, nil
}

func outsideTreeSHA256(path string) (string, error) {
	rootInfo, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("outside tree root is not a directory")
	}
	hash := sha256.New()
	var walk func(relative, directory string) error
	walk = func(relative, directory string) error {
		entries, err := os.ReadDir(directory)
		if err != nil {
			return err
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			entryPath := filepath.Join(directory, entry.Name())
			entryInfo, err := os.Lstat(entryPath)
			if err != nil {
				return err
			}
			entryRelative := filepath.ToSlash(filepath.Join(relative, entry.Name()))
			_, _ = fmt.Fprintf(hash, "%s\x00%s\x00", entryRelative, entryInfo.Mode().String())
			switch {
			case entryInfo.IsDir():
				if err := walk(entryRelative, entryPath); err != nil {
					return err
				}
			case entryInfo.Mode().IsRegular():
				data, err := os.ReadFile(entryPath)
				if err != nil {
					return err
				}
				_, _ = hash.Write(data)
			case entryInfo.Mode()&os.ModeSymlink != 0:
				target, err := os.Readlink(entryPath)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprint(hash, target)
			}
			_, _ = hash.Write([]byte{0})
		}
		return nil
	}
	if err := walk("", path); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func currentSourceState(t *testing.T) (string, bool) {
	t.Helper()
	shaOutput, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("resolve candidate HEAD: %v", err)
	}
	sha := strings.TrimSpace(string(shaOutput))
	if sha == "" {
		t.Fatal("git rev-parse HEAD returned an empty SHA")
	}
	statusOutput, err := exec.Command("git", "status", "--porcelain=v1", "--untracked-files=all").Output()
	if err != nil {
		t.Fatalf("check candidate tree: %v", err)
	}
	return sha, strings.TrimSpace(string(statusOutput)) != ""
}

func TestOutsideTreeSHA256RecursesWithoutFollowingSymlinks(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	nestedMarker := filepath.Join(nested, "marker.txt")
	if err := os.WriteFile(nestedMarker, []byte("nested-v1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlinkTarget := t.TempDir()
	if err := os.WriteFile(filepath.Join(symlinkTarget, "target.txt"), []byte("target-v1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(symlinkTarget, filepath.Join(root, "linked-target")); err != nil {
		t.Fatal(err)
	}

	before, err := outsideTreeSHA256(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nestedMarker, []byte("nested-v2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	afterNested, err := outsideTreeSHA256(root)
	if err != nil {
		t.Fatal(err)
	}
	if before == afterNested {
		t.Fatal("recursive outside-tree digest missed nested mutation")
	}
	if err := os.WriteFile(filepath.Join(symlinkTarget, "target.txt"), []byte("target-v2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	afterSymlinkTarget, err := outsideTreeSHA256(root)
	if err != nil {
		t.Fatal(err)
	}
	if afterNested != afterSymlinkTarget {
		t.Fatal("outside-tree digest followed a symlink target")
	}
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
